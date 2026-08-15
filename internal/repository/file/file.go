package file

import (
	"context"
	"errors"
	"io"
)

var (
	ErrExists   = errors.New("artifact object already exists")
	ErrNotFound = errors.New("artifact object not found")
)

// Info is immutable descriptive data returned by a Repository.
type Info struct {
	storageKey string
	sizeBytes  int64
	sha256     string
}

func (info Info) StorageKey() string { return info.storageKey }
func (info Info) SizeBytes() int64   { return info.sizeBytes }
func (info Info) SHA256() string     { return info.sha256 }

// Repository is the payload boundary implemented by in-memory, local, and
// S3-compatible adapters. Put is create-only: artifact payloads are immutable.
type Repository interface {
	Put(ctx context.Context, storageKey string, source io.Reader, maxBytes int64) (Info, error)
	Open(ctx context.Context, storageKey string) (io.ReadCloser, Info, error)
	Delete(ctx context.Context, storageKey string) error
}
