package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dagflows/builder/internal/config"
	"github.com/dagflows/builder/internal/domain"
	"github.com/dagflows/builder/pkg"
)

type LocalStore struct {
	root string
}

func NewLocal(cfg config.LocalStorageConfig) (*LocalStore, error) {
	root := strings.TrimSpace(cfg.Dir)
	if root == "" {
		root = config.DefaultLocalStorageDir
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	return &LocalStore{root: abs}, nil
}

func (s *LocalStore) PutFile(ctx context.Context, key, filePath, _ string) (domain.UploadedArtifact, error) {
	if err := ctx.Err(); err != nil {
		return domain.UploadedArtifact{}, err
	}

	rel := filepath.Clean(filepath.FromSlash(strings.TrimLeft(key, "/\\")))
	if rel == "." || rel == "" {
		return domain.UploadedArtifact{}, fmt.Errorf("storage key is required")
	}

	target, err := filepath.Abs(filepath.Join(s.root, rel))
	if err != nil {
		return domain.UploadedArtifact{}, err
	}
	if !pathInside(s.root, target) {
		return domain.UploadedArtifact{}, fmt.Errorf("storage key escapes local storage root")
	}

	if err := pkg.CopyFile(filePath, target); err != nil {
		return domain.UploadedArtifact{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return domain.UploadedArtifact{}, err
	}

	return domain.UploadedArtifact{
		Bucket:    "local",
		Key:       filepath.ToSlash(rel),
		SizeBytes: info.Size(),
	}, nil
}

func pathInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
