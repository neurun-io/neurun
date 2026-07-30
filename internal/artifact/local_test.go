package artifact

import (
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

func TestLocalStorePersistsImmutableObjects(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewLocalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	info, err := store.Put(
		context.Background(),
		"projects/prj_local/artifacts/result.txt",
		strings.NewReader("payload"),
		7,
	)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("payload"))
	if info.SizeBytes() != 7 || info.SHA256() != hexDigest(digest) {
		t.Fatalf("Put() info = %#v", info)
	}

	reopened, err := NewLocalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	reader, openedInfo, err := reopened.Open(context.Background(), info.StorageKey())
	if err != nil {
		t.Fatal(err)
	}
	payload, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		t.Fatal(err)
	}
	if string(payload) != "payload" || openedInfo.SHA256() != info.SHA256() {
		t.Fatalf("payload = %q, info = %#v", payload, openedInfo)
	}

	if _, err := reopened.Put(
		context.Background(),
		info.StorageKey(),
		strings.NewReader("replacement"),
		11,
	); !errors.Is(err, ErrObjectExists) {
		t.Fatalf("duplicate Put() error = %v, want ErrObjectExists", err)
	}
	if err := reopened.Delete(context.Background(), info.StorageKey()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reopened.Open(
		context.Background(),
		info.StorageKey(),
	); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("Open() after Delete() error = %v, want ErrObjectNotFound", err)
	}
}

func TestLocalStoreFailedPutsLeaveNoObject(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := NewLocalStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(
		context.Background(),
		"too-large.bin",
		strings.NewReader("123"),
		2,
	); !errors.Is(err, ErrByteLimitExceeded) {
		t.Fatalf("oversize Put() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Put(
		ctx,
		"canceled.bin",
		strings.NewReader("123"),
		3,
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Put() error = %v", err)
	}

	for _, key := range []string{"too-large.bin", "canceled.bin"} {
		if _, _, err := store.Open(
			context.Background(),
			key,
		); !errors.Is(err, ErrObjectNotFound) {
			t.Fatalf("Open(%q) error = %v, want ErrObjectNotFound", key, err)
		}
	}
	temporary, err := filepath.Glob(filepath.Join(root, ".neurun-upload-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporary) != 0 {
		t.Fatalf("temporary files left behind: %v", temporary)
	}
}

func TestLocalStoreConcurrentCreateOnlyPut(t *testing.T) {
	t.Parallel()

	store, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var succeeded atomic.Int32
	var wait sync.WaitGroup
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, putErr := store.Put(
				context.Background(),
				"same/key",
				strings.NewReader("same"),
				4,
			)
			if putErr == nil {
				succeeded.Add(1)
				return
			}
			if !errors.Is(putErr, ErrObjectExists) {
				t.Errorf("Put() error = %v", putErr)
			}
		}()
	}
	wait.Wait()
	if succeeded.Load() != 1 {
		t.Fatalf("successful puts = %d, want 1", succeeded.Load())
	}
}

func TestNewLocalStoreRejectsFileRoot(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "artifact-file")
	if err := os.WriteFile(root, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLocalStore(root); err == nil {
		t.Fatal("NewLocalStore() accepted a regular file")
	}
}
