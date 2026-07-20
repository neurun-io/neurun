package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCopyAndHashProducesBuilderCompatibleMetadata(t *testing.T) {
	t.Parallel()

	var destination bytes.Buffer
	result, err := CopyAndHash(&destination, strings.NewReader("hello"), 5)
	if err != nil {
		t.Fatalf("CopyAndHash() error = %v", err)
	}
	digest := sha256.Sum256([]byte("hello"))
	if result.SizeBytes != 5 || result.SHA256 != hexDigest(digest) {
		t.Fatalf("result = %+v", result)
	}
	if destination.String() != "hello" {
		t.Fatalf("destination = %q", destination.String())
	}
}

func TestCopyAndHashRejectsOversizeWithoutWritingPastLimit(t *testing.T) {
	t.Parallel()

	var destination bytes.Buffer
	result, err := CopyAndHash(&destination, strings.NewReader("123456"), 5)
	if !errors.Is(err, ErrByteLimitExceeded) {
		t.Fatalf("CopyAndHash() error = %v, want ErrByteLimitExceeded", err)
	}
	if result.SizeBytes != 5 || destination.String() != "12345" {
		t.Fatalf("result = %+v, destination = %q", result, destination.String())
	}
}

func TestCopyAndHashHonorsCancellationAndWriterErrors(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CopyAndHashContext(ctx, io.Discard, strings.NewReader("data"), 4); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled copy error = %v", err)
	}

	writeErr := errors.New("disk full")
	if _, err := CopyAndHash(errorWriter{err: writeErr}, strings.NewReader("data"), 4); !errors.Is(err, writeErr) {
		t.Fatalf("writer error = %v", err)
	}
	if _, err := CopyAndHash(io.Discard, zeroReader{}, 4); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("no-progress reader error = %v", err)
	}
}

func TestHashFileIsBounded(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "artifact.bin")
	if err := os.WriteFile(filePath, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := HashFile(filePath, 7)
	if err != nil {
		t.Fatal(err)
	}
	if result.SizeBytes != 7 {
		t.Fatalf("size = %d", result.SizeBytes)
	}
	if _, err := HashFile(filePath, 6); !errors.Is(err, ErrByteLimitExceeded) {
		t.Fatalf("bounded HashFile error = %v", err)
	}
}

func TestMemoryStoreImmutabilityAndLifecycle(t *testing.T) {
	t.Parallel()

	var store MemoryStore
	info, err := store.Put(context.Background(), "project/artifact.txt", strings.NewReader("payload"), 7)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("payload"))
	if info.StorageKey() != "project/artifact.txt" ||
		info.SizeBytes() != 7 ||
		info.SHA256() != hexDigest(digest) ||
		store.Len() != 1 {
		t.Fatalf("info = %#v, length = %d", info, store.Len())
	}

	if _, err := store.Put(context.Background(), "project/artifact.txt", strings.NewReader("replacement"), 20); !errors.Is(err, ErrObjectExists) {
		t.Fatalf("duplicate Put error = %v", err)
	}
	reader, openedInfo, err := store.Open(context.Background(), "project/artifact.txt")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := io.ReadAll(reader)
	if closeErr := reader.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "payload" || openedInfo.SHA256() != info.SHA256() {
		t.Fatalf("payload = %q, info = %#v", payload, openedInfo)
	}

	if err := store.Delete(context.Background(), "project/artifact.txt"); err != nil {
		t.Fatal(err)
	}
	if store.Len() != 0 {
		t.Fatalf("length after delete = %d", store.Len())
	}
	if _, _, err := store.Open(context.Background(), "project/artifact.txt"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("Open after delete error = %v", err)
	}
	if err := store.Delete(context.Background(), "project/artifact.txt"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("second Delete error = %v", err)
	}
}

func TestMemoryStoreOversizeAndCancellationAreAtomic(t *testing.T) {
	t.Parallel()

	var store MemoryStore
	if _, err := store.Put(context.Background(), "too-large", strings.NewReader("123"), 2); !errors.Is(err, ErrByteLimitExceeded) {
		t.Fatalf("oversize Put error = %v", err)
	}
	if store.Len() != 0 {
		t.Fatal("oversize object was retained")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Put(ctx, "canceled", strings.NewReader("123"), 3); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Put error = %v", err)
	}
	if store.Len() != 0 {
		t.Fatal("canceled object was retained")
	}
}

func TestMemoryStoreConcurrentCreateOnlyPut(t *testing.T) {
	t.Parallel()

	var store MemoryStore
	var succeeded atomic.Int32
	var wait sync.WaitGroup
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.Put(context.Background(), "same/key", strings.NewReader("same"), 4)
			if err == nil {
				succeeded.Add(1)
				return
			}
			if !errors.Is(err, ErrObjectExists) {
				t.Errorf("Put error = %v", err)
			}
		}()
	}
	wait.Wait()
	if succeeded.Load() != 1 || store.Len() != 1 {
		t.Fatalf("successful puts = %d, objects = %d", succeeded.Load(), store.Len())
	}
}

type errorWriter struct {
	err error
}

type zeroReader struct{}

func (zeroReader) Read(_ []byte) (int, error) {
	return 0, nil
}

func (writer errorWriter) Write(_ []byte) (int, error) {
	return 0, writer.err
}
