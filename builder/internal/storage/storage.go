package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/dagflows/builder/internal/config"
	"github.com/dagflows/builder/internal/domain"
)

type Store interface {
	PutFile(ctx context.Context, key, path, mediaType string) (domain.UploadedArtifact, error)
}

func NewStore(cfg config.Config) (Store, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Storage.Driver)) {
	case "", config.DefaultStorageDriver, "filesystem", "fs":
		return NewLocal(cfg.LocalStorage)
	case "r2":
		return NewR2(cfg.R2)
	default:
		return nil, fmt.Errorf("unsupported STORAGE_DRIVER %q", cfg.Storage.Driver)
	}
}
