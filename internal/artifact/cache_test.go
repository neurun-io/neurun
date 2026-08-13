package artifact

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// countingStore is a MemoryStore that records how often it was read, which is
// the only way to see whether the cache in front of it did its job.
type countingStore struct {
	inner MemoryStore
	opens atomic.Int64
	block chan struct{}
}

func (store *countingStore) Put(
	ctx context.Context, key string, source io.Reader, maxBytes int64,
) (BlobInfo, error) {
	return store.inner.Put(ctx, key, source, maxBytes)
}

func (store *countingStore) Open(
	ctx context.Context, key string,
) (io.ReadCloser, BlobInfo, error) {
	store.opens.Add(1)
	if store.block != nil {
		<-store.block
	}
	return store.inner.Open(ctx, key)
}

func (store *countingStore) Delete(ctx context.Context, key string) error {
	return store.inner.Delete(ctx, key)
}

func newCache(t *testing.T, remote BlobStore, maxBytes int64) *CacheStore {
	t.Helper()
	cache, err := NewCacheStore(remote, CacheOptions{
		Directory: t.TempDir(), MaxBytes: maxBytes,
	})
	if err != nil {
		t.Fatalf("construct cache: %v", err)
	}
	return cache
}

func read(t *testing.T, store BlobStore, key string) string {
	t.Helper()
	body, _, err := store.Open(context.Background(), key)
	if err != nil {
		t.Fatalf("open %s: %v", key, err)
	}
	defer body.Close()
	payload, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	return string(payload)
}

const objectKey = "objects/sha256/ab/abcdef"

func TestCacheServesRepeatedReadsWithoutTouchingTheRemote(t *testing.T) {
	t.Parallel()
	remote := &countingStore{}
	cache := newCache(t, remote, 0)

	if _, err := cache.Put(
		context.Background(), objectKey, strings.NewReader("payload"), 1<<20,
	); err != nil {
		t.Fatalf("put: %v", err)
	}
	// A write-through leaves the cache warm, so the first read is already a hit.
	for range 3 {
		if body := read(t, cache, objectKey); body != "payload" {
			t.Fatalf("body = %q", body)
		}
	}
	if opens := remote.opens.Load(); opens != 0 {
		t.Fatalf("remote reads = %d, want 0", opens)
	}
}

func TestCacheCollapsesConcurrentMissesIntoOneFetch(t *testing.T) {
	t.Parallel()
	remote := &countingStore{block: make(chan struct{})}
	if _, err := remote.inner.Put(
		context.Background(), objectKey, strings.NewReader("payload"), 1<<20,
	); err != nil {
		t.Fatalf("seed remote: %v", err)
	}
	cache := newCache(t, remote, 0)

	var group sync.WaitGroup
	bodies := make([]string, 8)
	for index := range bodies {
		group.Add(1)
		go func() {
			defer group.Done()
			body, _, err := cache.Open(context.Background(), objectKey)
			if err != nil {
				return
			}
			defer body.Close()
			payload, _ := io.ReadAll(body)
			bodies[index] = string(payload)
		}()
	}
	// Every reader is now either waiting on the group or on the remote itself.
	close(remote.block)
	group.Wait()

	for index, body := range bodies {
		if body != "payload" {
			t.Fatalf("reader %d got %q", index, body)
		}
	}
	// Eight concurrent misses on one key must not become eight downloads.
	if opens := remote.opens.Load(); opens > 2 {
		t.Fatalf("remote reads = %d, want the misses collapsed", opens)
	}
}

func TestCacheEvictsLeastRecentlyUsedToStayInBudget(t *testing.T) {
	t.Parallel()
	remote := &countingStore{}
	// Room for two objects of ten bytes, so writing a third must drop one.
	cache := newCache(t, remote, 20)

	keys := []string{
		"objects/sha256/aa/" + strings.Repeat("a", 8),
		"objects/sha256/bb/" + strings.Repeat("b", 8),
		"objects/sha256/cc/" + strings.Repeat("c", 8),
	}
	for _, key := range keys[:2] {
		if _, err := cache.Put(
			context.Background(), key, bytes.NewReader(make([]byte, 10)), 1<<20,
		); err != nil {
			t.Fatalf("put %s: %v", key, err)
		}
	}
	// Reading the first makes the second the least recently used.
	read(t, cache, keys[0])
	if _, err := cache.Put(
		context.Background(), keys[2], bytes.NewReader(make([]byte, 10)), 1<<20,
	); err != nil {
		t.Fatalf("put %s: %v", keys[2], err)
	}

	cache.mu.Lock()
	_, keptFirst := cache.entries[keys[0]]
	_, keptSecond := cache.entries[keys[1]]
	bytesHeld := cache.bytes
	cache.mu.Unlock()

	if !keptFirst || keptSecond {
		t.Fatalf("evicted the wrong entry: first=%v second=%v", keptFirst, keptSecond)
	}
	if bytesHeld > 20 {
		t.Fatalf("cache holds %d bytes, over its budget", bytesHeld)
	}
	// Evicted, not lost: the remote still has it.
	if body := read(t, cache, keys[1]); len(body) != 10 {
		t.Fatalf("refetched body = %d bytes", len(body))
	}
}

func TestCacheReportsAMissingObjectRatherThanMasking(t *testing.T) {
	t.Parallel()
	cache := newCache(t, &countingStore{}, 0)
	if _, _, err := cache.Open(
		context.Background(), objectKey,
	); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("err = %v, want ErrObjectNotFound", err)
	}
}
