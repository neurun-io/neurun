// Package artifact contains immutable artifact metadata, bounded payload
// helpers, and defensive archive extraction primitives.
package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

const copyBufferSize = 32 << 10

var ErrByteLimitExceeded = errors.New("artifact byte limit exceeded")

// CopyResult describes the bytes successfully copied and hashed.
type CopyResult struct {
	SizeBytes int64
	SHA256    string
}

// CopyAndHash copies source to destination while calculating SHA-256. It never
// writes more than maxBytes and probes the source at the boundary so oversized
// input returns ErrByteLimitExceeded rather than silent truncation.
func CopyAndHash(destination io.Writer, source io.Reader, maxBytes int64) (CopyResult, error) {
	return CopyAndHashContext(context.Background(), destination, source, maxBytes)
}

// CopyAndHashContext is CopyAndHash with cancellation checks between reads.
func CopyAndHashContext(
	ctx context.Context,
	destination io.Writer,
	source io.Reader,
	maxBytes int64,
) (CopyResult, error) {
	if destination == nil {
		return CopyResult{}, errors.New("artifact: destination is nil")
	}
	if source == nil {
		return CopyResult{}, errors.New("artifact: source is nil")
	}
	if maxBytes < 0 {
		return CopyResult{}, errors.New("artifact: byte limit cannot be negative")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	digest := sha256.New()
	writer := io.MultiWriter(destination, digest)
	buffer := make([]byte, copyBufferSize)
	var size int64
	var emptyReads int

	result := func() CopyResult {
		return CopyResult{
			SizeBytes: size,
			SHA256:    hex.EncodeToString(digest.Sum(nil)),
		}
	}

	for {
		if err := ctx.Err(); err != nil {
			return result(), err
		}

		remaining := maxBytes - size
		if remaining == 0 {
			var probe [1]byte
			count, readErr := source.Read(probe[:])
			if count > 0 {
				return result(), fmt.Errorf("%w: limit %d", ErrByteLimitExceeded, maxBytes)
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					return result(), nil
				}
				return result(), readErr
			}
			emptyReads++
			if emptyReads >= 100 {
				return result(), io.ErrNoProgress
			}
			continue
		}

		readSize := int64(len(buffer))
		if remaining < readSize {
			readSize = remaining
		}
		count, readErr := source.Read(buffer[:readSize])
		if count > 0 {
			emptyReads = 0
			written, writeErr := writer.Write(buffer[:count])
			size += int64(written)
			if writeErr != nil {
				return result(), writeErr
			}
			if written != count {
				return result(), io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return result(), nil
			}
			return result(), readErr
		}
		if count == 0 {
			emptyReads++
			if emptyReads >= 100 {
				return result(), io.ErrNoProgress
			}
		}
	}
}

// Hash calculates a bounded SHA-256 without retaining the payload.
func Hash(source io.Reader, maxBytes int64) (CopyResult, error) {
	return CopyAndHash(io.Discard, source, maxBytes)
}

// HashFile calculates a bounded SHA-256 for a regular file.
func HashFile(filePath string, maxBytes int64) (CopyResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return CopyResult{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return CopyResult{}, err
	}
	if !info.Mode().IsRegular() {
		return CopyResult{}, fmt.Errorf("artifact: %q is not a regular file", filePath)
	}
	if maxBytes >= 0 && info.Size() > maxBytes {
		return CopyResult{}, fmt.Errorf("%w: file size %d, limit %d",
			ErrByteLimitExceeded, info.Size(), maxBytes)
	}
	return Hash(file, maxBytes)
}
