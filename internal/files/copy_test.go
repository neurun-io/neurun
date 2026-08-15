package files

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func hexDigest(digest [sha256.Size]byte) string {
	const digits = "0123456789abcdef"
	encoded := make([]byte, len(digest)*2)
	for index, value := range digest {
		encoded[index*2] = digits[value>>4]
		encoded[index*2+1] = digits[value&0x0f]
	}
	return string(encoded)
}
