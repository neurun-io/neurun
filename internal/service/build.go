package service

import (
	"context"
	"errors"

	"github.com/neurun-io/neurun/internal/repository/database"
)

// BuildService reads what deployments produced. It only ever reads: a build is
// made by the deployment that ran the toolchain, never by a caller.
type BuildService struct {
	builds *database.BuildRepository
}

func NewBuildService(builds *database.BuildRepository) (*BuildService, error) {
	if builds == nil {
		return nil, errors.New("build service requires its repository")
	}
	return &BuildService{builds: builds}, nil
}

func (service *BuildService) List(
	ctx context.Context,
	organizationID string,
	deploymentID string,
	limit int,
) ([]database.Produced, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	return service.builds.List(ctx, organizationID, deploymentID, limit)
}

func (service *BuildService) Get(
	ctx context.Context,
	organizationID string,
	buildID string,
) (database.Produced, error) {
	return service.builds.Get(ctx, organizationID, buildID)
}
