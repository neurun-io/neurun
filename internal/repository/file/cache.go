package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"

	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/files"
)

type CacheOptions struct {
	Directory string
	MaxBytes  int64
}

// DefaultCacheBytes is a budget large enough to hold the working set of builds a
// busy worker replays, and small enough to sit on an ordinary container disk.
const DefaultCacheBytes = 8_589_934_592

// Cache is a read-through cache in front of a remote Repository.
//
// It is safe because storage keys are content addresses: an object at a key can
// never change, so a hit needs no validation, no expiry and no revalidation
// round trip. Eviction is the only thing that ever removes an entry, and losing
// one costs a download rather than correctness.
//
// The cache is never load-bearing. Every failure to write, evict or index is
// logged and swallowed — a full disk degrades a worker to remote reads instead
// of failing the execution that found it.
type Cache struct {
	remote   Repository
	root     string
	maxBytes int64

	// fills collapses concurrent misses on one key into a single download, which
	// is the difference between one fetch and one per concurrent execution of the
	// same build.
	fills singleflight.Group

	mu    sync.Mutex
	clock uint64
	bytes int64
	// Access order is kept in memory rather than as file timestamps: a read is
	// the hot path and should not write.
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	size int64
	used uint64
}

func NewCache(remote Repository, options CacheOptions) (*Cache, error) {
	if remote == nil {
		return nil, errors.New("artifact: cache requires a backing store")
	}
	root := strings.TrimSpace(options.Directory)
	if root == "" {
		return nil, errors.New("artifact: cache directory is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("artifact: resolve cache directory: %w", err)
	}
	if options.MaxBytes < 0 {
		return nil, errors.New("artifact: cache budget cannot be negative")
	}
	if options.MaxBytes == 0 {
		options.MaxBytes = DefaultCacheBytes
	}
	store := &Cache{
		remote:   remote,
		root:     filepath.Clean(absolute),
		maxBytes: options.MaxBytes,
		entries:  map[string]*cacheEntry{},
	}
	if err := os.MkdirAll(store.stagingDir(), 0o750); err != nil {
		return nil, fmt.Errorf("artifact: create cache directory: %w", err)
	}
	// Anything left staged belongs to a process that died mid-fill.
	if err := os.RemoveAll(store.stagingDir()); err == nil {
		_ = os.MkdirAll(store.stagingDir(), 0o750)
	}
	store.index()
	return store, nil
}

// Put writes through. The object is spooled into the cache first and uploaded
// from there, so one pass over the bytes both stores them remotely and leaves
// the cache warm for the execution that follows the build.
func (store *Cache) Put(
	ctx context.Context,
	storageKey string,
	source io.Reader,
	maxBytes int64,
) (Info, error) {
	if err := build.ValidateStorageKey(storageKey); err != nil {
		return Info{}, err
	}
	ctx = orBackground(ctx)
	staged, result, err := store.stage(ctx, source, maxBytes)
	if err != nil {
		return Info{}, err
	}
	defer os.Remove(staged)

	file, err := os.Open(staged)
	if err != nil {
		return Info{}, fmt.Errorf("artifact: reopen staged object: %w", err)
	}
	info, putErr := store.remote.Put(ctx, storageKey, file, maxBytes)
	closeErr := file.Close()
	if putErr != nil {
		return Info{}, putErr
	}
	if closeErr != nil {
		return Info{}, closeErr
	}
	store.publish(storageKey, staged, result.SizeBytes)
	return info, nil
}

// Open serves a hit from disk and fills on a miss.
func (store *Cache) Open(
	ctx context.Context,
	storageKey string,
) (io.ReadCloser, Info, error) {
	if err := build.ValidateStorageKey(storageKey); err != nil {
		return nil, Info{}, err
	}
	ctx = orBackground(ctx)
	if file, info, ok := store.hit(storageKey); ok {
		return file, info, nil
	}
	if _, err, _ := store.fills.Do(storageKey, func() (any, error) {
		return nil, store.fill(ctx, storageKey)
	}); err != nil {
		// A cache that cannot be filled is still a cache that can be bypassed.
		if errors.Is(err, ErrNotFound) {
			return nil, Info{}, err
		}
		slog.Warn("artifact cache fill failed", "key", storageKey, "error", err)
		return store.remote.Open(ctx, storageKey)
	}
	if file, info, ok := store.hit(storageKey); ok {
		return file, info, nil
	}
	return store.remote.Open(ctx, storageKey)
}

// Delete removes the object remotely and evicts the copy. The remote is the
// authority, so its failure is the caller's; a stale local copy is not, because
// the key can never be reused for different bytes.
func (store *Cache) Delete(ctx context.Context, storageKey string) error {
	if err := build.ValidateStorageKey(storageKey); err != nil {
		return err
	}
	if err := store.remote.Delete(orBackground(ctx), storageKey); err != nil {
		return err
	}
	store.evict(storageKey)
	return nil
}

func (store *Cache) Check(ctx context.Context) error {
	if checker, ok := store.remote.(interface {
		Check(context.Context) error
	}); ok {
		return checker.Check(ctx)
	}
	return nil
}

func (store *Cache) hit(storageKey string) (io.ReadCloser, Info, bool) {
	path, err := store.path(storageKey)
	if err != nil {
		return nil, Info{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, Info{}, false
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, Info{}, false
	}
	store.touch(storageKey, info.Size())
	return file, Info{storageKey: storageKey, sizeBytes: info.Size()}, true
}

func (store *Cache) fill(ctx context.Context, storageKey string) error {
	// Another goroutine may have finished while this one waited for the group.
	if path, err := store.path(storageKey); err == nil {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return nil
		}
	}
	body, _, err := store.remote.Open(ctx, storageKey)
	if err != nil {
		return err
	}
	defer body.Close()

	staged, result, err := store.stage(ctx, body, store.maxBytes)
	if err != nil {
		return err
	}
	store.publish(storageKey, staged, result.SizeBytes)
	return nil
}

// stage writes bytes to a private file inside the cache, hashing as it goes. The
// caller either publishes it under a key or removes it.
func (store *Cache) stage(
	ctx context.Context,
	source io.Reader,
	maxBytes int64,
) (string, files.CopyResult, error) {
	if source == nil {
		return "", files.CopyResult{}, errors.New("artifact: source is nil")
	}
	if err := os.MkdirAll(store.stagingDir(), 0o750); err != nil {
		return "", files.CopyResult{}, fmt.Errorf("artifact: create cache staging: %w", err)
	}
	staged, err := os.CreateTemp(store.stagingDir(), "fill-*")
	if err != nil {
		return "", files.CopyResult{}, fmt.Errorf("artifact: stage cache object: %w", err)
	}
	path := staged.Name()
	result, copyErr := files.CopyAndHashContext(ctx, staged, source, maxBytes)
	if copyErr == nil {
		copyErr = staged.Sync()
	}
	closeErr := staged.Close()
	if copyErr != nil {
		os.Remove(path)
		return "", files.CopyResult{}, copyErr
	}
	if closeErr != nil {
		os.Remove(path)
		return "", files.CopyResult{}, closeErr
	}
	return path, result, nil
}

// publish moves a staged file into place. Rename is atomic within a filesystem,
// so a reader sees the whole object or no object, never a prefix.
func (store *Cache) publish(storageKey, staged string, size int64) {
	target, err := store.path(storageKey)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		slog.Warn("artifact cache directory failed", "key", storageKey, "error", err)
		return
	}
	if err := os.Rename(staged, target); err != nil {
		// Losing the race to another filler is the ordinary case, not a fault:
		// the bytes are identical either way.
		if !errors.Is(err, os.ErrExist) {
			slog.Warn("artifact cache publish failed", "key", storageKey, "error", err)
		}
		return
	}
	store.touch(storageKey, size)
	store.trim()
}

func (store *Cache) touch(storageKey string, size int64) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.clock++
	entry, known := store.entries[storageKey]
	if !known {
		entry = &cacheEntry{size: size}
		store.entries[storageKey] = entry
		store.bytes += size
	}
	entry.used = store.clock
}

func (store *Cache) evict(storageKey string) {
	if target, err := store.path(storageKey); err == nil {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			// A reader still holds it open on Windows. Leave it indexed and let a
			// later trim retry rather than losing track of the bytes.
			return
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if entry, known := store.entries[storageKey]; known {
		store.bytes -= entry.size
		delete(store.entries, storageKey)
	}
}

// trim drops least-recently-used entries until the cache is inside its budget.
func (store *Cache) trim() {
	store.mu.Lock()
	if store.bytes <= store.maxBytes {
		store.mu.Unlock()
		return
	}
	type candidate struct {
		key  string
		used uint64
		size int64
	}
	candidates := make([]candidate, 0, len(store.entries))
	for key, entry := range store.entries {
		candidates = append(candidates, candidate{key: key, used: entry.used, size: entry.size})
	}
	over := store.bytes - store.maxBytes
	store.mu.Unlock()

	sort.Slice(candidates, func(i, j int) bool { return candidates[i].used < candidates[j].used })
	for _, item := range candidates {
		if over <= 0 {
			return
		}
		store.evict(item.key)
		over -= item.size
	}
}

// index rebuilds the accounting from what is already on disk, so a restart keeps
// the cache it warmed rather than starting cold or growing unbounded.
func (store *Cache) index() {
	staging := store.stagingDir()
	err := filepath.WalkDir(store.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path == staging {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(store.root, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(relative)
		if build.ValidateStorageKey(key) != nil {
			return nil
		}
		store.mu.Lock()
		store.clock++
		store.entries[key] = &cacheEntry{size: info.Size(), used: store.clock}
		store.bytes += info.Size()
		store.mu.Unlock()
		return nil
	})
	if err != nil {
		slog.Warn("artifact cache index failed", "error", err)
	}
	store.trim()
}

func (store *Cache) stagingDir() string {
	return filepath.Join(store.root, ".staging")
}

func (store *Cache) path(storageKey string) (string, error) {
	if err := build.ValidateStorageKey(storageKey); err != nil {
		return "", err
	}
	target := filepath.Join(store.root, filepath.FromSlash(storageKey))
	relative, err := filepath.Rel(store.root, target)
	if err != nil || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: key escapes cache", build.ErrInvalidStorageKey)
	}
	return target, nil
}

var _ Repository = (*Cache)(nil)
