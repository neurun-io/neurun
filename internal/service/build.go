package service

import (
	"context"
	"errors"

	"github.com/neurun-io/neurun/internal/domain/build"
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
	appID string,
	deploymentID string,
	limit int,
) ([]build.Build, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	return service.builds.List(ctx, organizationID, appID, deploymentID, limit)
}

func (service *BuildService) Get(
	ctx context.Context,
	organizationID string,
	buildID string,
) (build.Build, error) {
	return service.builds.Get(ctx, organizationID, buildID)
}
