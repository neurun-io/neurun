package file

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/files"
)

// Memory is a deterministic, concurrency-safe Repository, and the remote
// the cache is tested against. Its zero value is ready for use.
type Memory struct {
	mu      sync.RWMutex
	objects map[string]memoryObject
}

type memoryObject struct {
	data   []byte
	digest string
}

func (store *Memory) Put(
	ctx context.Context,
	storageKey string,
	source io.Reader,
	maxBytes int64,
) (Info, error) {
	if err := build.ValidateStorageKey(storageKey); err != nil {
		return Info{}, err
	}
	if source == nil {
		return Info{}, errors.New("artifact: source is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}

	store.mu.RLock()
	_, exists := store.objects[storageKey]
	store.mu.RUnlock()
	if exists {
		return Info{}, fmt.Errorf("%w: %s", ErrExists, storageKey)
	}

	var payload bytes.Buffer
	result, err := files.CopyAndHashContext(ctx, &payload, source, maxBytes)
	if err != nil {
		return Info{}, err
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
		return Info{}, fmt.Errorf("%w: %s", ErrExists, storageKey)
	}
	store.objects[storageKey] = object
	store.mu.Unlock()

	return Info{
		storageKey: storageKey,
		sizeBytes:  result.SizeBytes,
		sha256:     result.SHA256,
	}, nil
}

func (store *Memory) Open(
	ctx context.Context,
	storageKey string,
) (io.ReadCloser, Info, error) {
	if err := build.ValidateStorageKey(storageKey); err != nil {
		return nil, Info{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, Info{}, err
	}

	store.mu.RLock()
	object, exists := store.objects[storageKey]
	store.mu.RUnlock()
	if !exists {
		return nil, Info{}, fmt.Errorf("%w: %s", ErrNotFound, storageKey)
	}

	info := Info{
		storageKey: storageKey,
		sizeBytes:  int64(len(object.data)),
		sha256:     object.digest,
	}
	return io.NopCloser(bytes.NewReader(object.data)), info, nil
}

func (store *Memory) Delete(ctx context.Context, storageKey string) error {
	if err := build.ValidateStorageKey(storageKey); err != nil {
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
		return fmt.Errorf("%w: %s", ErrNotFound, storageKey)
	}
	delete(store.objects, storageKey)
	return nil
}

// Len returns the number of objects and is primarily useful in health checks
// and deterministic tests.
func (store *Memory) Len() int {
	if store == nil {
		return 0
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	return len(store.objects)
}

var _ Repository = (*Memory)(nil)
