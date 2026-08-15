package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/files"
)

// Local persists immutable artifact payloads beneath one server-owned
// directory. Storage keys remain slash-separated and portable at the boundary;
// they are converted to native paths only after validation.
type Local struct {
	root string
}

// NewLocal prepares a filesystem-backed artifact store.
func NewLocal(root string) (*Local, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("artifact: local storage directory is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("artifact: resolve local storage directory: %w", err)
	}
	absolute = filepath.Clean(absolute)
	if err := os.MkdirAll(absolute, 0o750); err != nil {
		return nil, fmt.Errorf("artifact: create local storage directory: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("artifact: inspect local storage directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("artifact: local storage path %q is not a directory", absolute)
	}
	store := &Local{root: absolute}
	if err := store.Check(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

// Root returns the absolute directory owned by the store.
func (store *Local) Root() string {
	if store == nil {
		return ""
	}
	return store.root
}

func (store *Local) Put(
	ctx context.Context,
	storageKey string,
	source io.Reader,
	maxBytes int64,
) (Info, error) {
	target, err := store.path(storageKey)
	if err != nil {
		return Info{}, err
	}
	if source == nil {
		return Info{}, errors.New("artifact: source is nil")
	}
	if maxBytes < 0 {
		return Info{}, errors.New("artifact: byte limit cannot be negative")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}

	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return Info{}, fmt.Errorf("artifact: create object directory: %w", err)
	}
	temporary, err := os.CreateTemp(parent, ".neurun-upload-*")
	if err != nil {
		return Info{}, fmt.Errorf("artifact: create temporary object: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return Info{}, fmt.Errorf("artifact: protect temporary object: %w", err)
	}
	result, copyErr := files.CopyAndHashContext(ctx, temporary, source, maxBytes)
	if copyErr == nil {
		copyErr = temporary.Sync()
	}
	closeErr := temporary.Close()
	if copyErr != nil {
		return Info{}, copyErr
	}
	if closeErr != nil {
		return Info{}, closeErr
	}

	// Linking a completed temporary file publishes it atomically and fails if
	// the immutable key already exists. The deferred remove drops only the
	// temporary link after publication.
	if err := os.Link(temporaryPath, target); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Info{}, fmt.Errorf("%w: %s", ErrExists, storageKey)
		}
		return Info{}, fmt.Errorf("artifact: publish local object: %w", err)
	}

	return Info{
		storageKey: storageKey,
		sizeBytes:  result.SizeBytes,
		sha256:     result.SHA256,
	}, nil
}

func (store *Local) Open(
	ctx context.Context,
	storageKey string,
) (io.ReadCloser, Info, error) {
	target, err := store.path(storageKey)
	if err != nil {
		return nil, Info{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, Info{}, err
	}

	file, err := os.Open(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, Info{}, fmt.Errorf("%w: %s", ErrNotFound, storageKey)
		}
		return nil, Info{}, fmt.Errorf("artifact: open local object: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, Info{}, fmt.Errorf("artifact: inspect local object: %w", err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, Info{}, fmt.Errorf("artifact: object %q is not a regular file", storageKey)
	}

	result, err := files.CopyAndHashContext(ctx, io.Discard, file, info.Size())
	if err != nil {
		file.Close()
		return nil, Info{}, fmt.Errorf("artifact: hash local object: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return nil, Info{}, fmt.Errorf("artifact: rewind local object: %w", err)
	}
	return file, Info{
		storageKey: storageKey,
		sizeBytes:  result.SizeBytes,
		sha256:     result.SHA256,
	}, nil
}

func (store *Local) Delete(ctx context.Context, storageKey string) error {
	target, err := store.path(storageKey)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(target); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNotFound, storageKey)
		}
		return fmt.Errorf("artifact: delete local object: %w", err)
	}
	return nil
}

// Check verifies that the backing directory still exists and is writable.
func (store *Local) Check(ctx context.Context) error {
	if store == nil || store.root == "" {
		return errors.New("artifact: local store is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	probe, err := os.CreateTemp(store.root, ".neurun-ready-*")
	if err != nil {
		return fmt.Errorf("artifact: local storage is not writable: %w", err)
	}
	probePath := probe.Name()
	closeErr := probe.Close()
	removeErr := os.Remove(probePath)
	return errors.Join(closeErr, removeErr)
}

func (store *Local) path(storageKey string) (string, error) {
	if store == nil || store.root == "" {
		return "", errors.New("artifact: local store is not configured")
	}
	if err := build.ValidateStorageKey(storageKey); err != nil {
		return "", err
	}
	target := filepath.Join(store.root, filepath.FromSlash(storageKey))
	relative, err := filepath.Rel(store.root, target)
	if err != nil ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: key escapes local storage", build.ErrInvalidStorageKey)
	}
	return target, nil
}

var _ Repository = (*Local)(nil)
