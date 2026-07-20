package artifact

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

var (
	ErrObjectExists   = errors.New("artifact object already exists")
	ErrObjectNotFound = errors.New("artifact object not found")
)

// BlobInfo is immutable descriptive data returned by a BlobStore.
type BlobInfo struct {
	storageKey string
	sizeBytes  int64
	sha256     string
}

func (info BlobInfo) StorageKey() string { return info.storageKey }
func (info BlobInfo) SizeBytes() int64   { return info.sizeBytes }
func (info BlobInfo) SHA256() string     { return info.sha256 }

// BlobStore is the payload boundary implemented by in-memory, local, and
// S3-compatible adapters. Put is create-only: artifact payloads are immutable.
type BlobStore interface {
	Put(ctx context.Context, storageKey string, source io.Reader, maxBytes int64) (BlobInfo, error)
	Open(ctx context.Context, storageKey string) (io.ReadCloser, BlobInfo, error)
	Delete(ctx context.Context, storageKey string) error
}

// MemoryStore is a deterministic, concurrency-safe BlobStore for development
// and tests. Its zero value is ready for use.
type MemoryStore struct {
	mu      sync.RWMutex
	objects map[string]memoryObject
}

type memoryObject struct {
	data   []byte
	digest string
}

func (store *MemoryStore) Put(
	ctx context.Context,
	storageKey string,
	source io.Reader,
	maxBytes int64,
) (BlobInfo, error) {
	if err := ValidateStorageKey(storageKey); err != nil {
		return BlobInfo{}, err
	}
	if source == nil {
		return BlobInfo{}, errors.New("artifact: source is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return BlobInfo{}, err
	}

	store.mu.RLock()
	_, exists := store.objects[storageKey]
	store.mu.RUnlock()
	if exists {
		return BlobInfo{}, fmt.Errorf("%w: %s", ErrObjectExists, storageKey)
	}

	var payload bytes.Buffer
	result, err := CopyAndHashContext(ctx, &payload, source, maxBytes)
	if err != nil {
		return BlobInfo{}, err
	}
	object := memoryObject{
		data:   append([]byte(nil), payload.Bytes()...),
		digest: result.SHA256,
	}

	store.mu.Lock()
	if store.objects == nil {
		store.objects = make(map[string]memoryObject)
	}
	if _, exists := store.objects[storageKey]; exists {
		store.mu.Unlock()
		return BlobInfo{}, fmt.Errorf("%w: %s", ErrObjectExists, storageKey)
	}
	store.objects[storageKey] = object
	store.mu.Unlock()

	return BlobInfo{
		storageKey: storageKey,
		sizeBytes:  result.SizeBytes,
		sha256:     result.SHA256,
	}, nil
}

func (store *MemoryStore) Open(
	ctx context.Context,
	storageKey string,
) (io.ReadCloser, BlobInfo, error) {
	if err := ValidateStorageKey(storageKey); err != nil {
		return nil, BlobInfo{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, BlobInfo{}, err
	}

	store.mu.RLock()
	object, exists := store.objects[storageKey]
	store.mu.RUnlock()
	if !exists {
		return nil, BlobInfo{}, fmt.Errorf("%w: %s", ErrObjectNotFound, storageKey)
	}

	info := BlobInfo{
		storageKey: storageKey,
		sizeBytes:  int64(len(object.data)),
		sha256:     object.digest,
	}
	return io.NopCloser(bytes.NewReader(object.data)), info, nil
}

func (store *MemoryStore) Delete(ctx context.Context, storageKey string) error {
	if err := ValidateStorageKey(storageKey); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.objects[storageKey]; !exists {
		return fmt.Errorf("%w: %s", ErrObjectNotFound, storageKey)
	}
	delete(store.objects, storageKey)
	return nil
}

// Len returns the number of objects and is primarily useful in health checks
// and deterministic tests.
func (store *MemoryStore) Len() int {
	if store == nil {
		return 0
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	return len(store.objects)
}

var _ BlobStore = (*MemoryStore)(nil)
