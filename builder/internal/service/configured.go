package service

import (
	"github.com/dagflows/builder/internal/config"
	"github.com/dagflows/builder/internal/storage"
)

func NewConfiguredDeploymentService(cfg config.Config) (*DeploymentService, error) {
	store, err := storage.NewStore(cfg)
	if err != nil {
		return nil, err
	}
	return NewDeploymentService(NewBuildService(store), NewGitHubService(cfg.GitHub)), nil
}
