package storage

import (
	"context"

	"github.com/dagflows/builder/internal/config"
	"github.com/dagflows/builder/internal/domain"
)

type Store interface {
	PutFile(ctx context.Context, key, path, mediaType string) (domain.UploadedArtifact, error)
}

func NewStore(cfg config.Config) (Store, error) {
	return NewR2(cfg.R2)
}
