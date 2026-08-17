// Package service holds the application logic: it drives domain records
// through their transitions and hands them to repositories to persist.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neurun-io/neurun/internal/domain/deployment"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository/database"
)

const maximumPageSize = 200

type DeploymentOptions struct {
	Now   func() time.Time
	NewID func(string) (string, error)
}

// DeploymentService queues deployments and reads them back. Building one is the
// deployer's work: it claims what is queued here, so a deployment outlives the
// request that asked for it.
type DeploymentService struct {
	apps        *database.AppRepository
	deployments *database.DeploymentRepository
	now         func() time.Time
	newID       func(string) (string, error)
}

func NewDeploymentService(
	apps *database.AppRepository,
	deployments *database.DeploymentRepository,
	options DeploymentOptions,
) (*DeploymentService, error) {
	if apps == nil || deployments == nil {
		return nil, errors.New("deployment service requires its repositories")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.NewID == nil {
		options.NewID = ids.New
	}
	return &DeploymentService{
		apps: apps, deployments: deployments,
		now: options.Now, newID: options.NewID,
	}, nil
}

// Create queues a commit to build. It returns as soon as the row is written:
// what happens next is a deployer's, and the caller follows the deployment.
func (service *DeploymentService) Create(
	ctx context.Context,
	organizationID string,
	request dto.CreateDeploymentRequest,
) (deployment.Deployment, error) {
	ctx = orBackground(ctx)
	if err := deployment.ValidateIdentifier("app_id", request.AppID); err != nil {
		return deployment.Deployment{}, err
	}
	// The app decides the project. An SDK cannot conjure one by naming it, and an
	// app that does not already exist is refused rather than created.
	app, err := service.apps.GetByID(ctx, organizationID, request.AppID)
	if err != nil {
		return deployment.Deployment{}, err
	}
	if !request.Runtime.Valid() {
		return deployment.Deployment{}, fmt.Errorf(
			"%w: runtime must be python, rust, go, ruby or node", deployment.ErrInvalid,
		)
	}
	deploymentID, err := service.allocateID("dep")
	if err != nil {
		return deployment.Deployment{}, err
	}
	record, err := deployment.New(
		deploymentID, app.ProjectID, app.ID, request.Runtime,
		service.now().UTC().Round(0),
	)
	if err != nil {
		return deployment.Deployment{}, err
	}
	record.FromGit(request.CommitSHA, request.GitRef)
	if err := service.deployments.Save(ctx, record); err != nil {
		return deployment.Deployment{}, fmt.Errorf("persist queued deployment: %w", err)
	}
	return deployment.CloneDeployment(record), nil
}

func (service *DeploymentService) Get(
	ctx context.Context,
	organizationID string,
	deploymentID string,
) (deployment.Deployment, error) {
	return service.deployments.GetByID(ctx, organizationID, deploymentID)
}

func (service *DeploymentService) List(
	ctx context.Context,
	organizationID string,
	projectID string,
	appID string,
	limit int,
) ([]deployment.Deployment, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	if appID != "" {
		if _, err := service.apps.GetByID(ctx, organizationID, appID); err != nil {
			return nil, err
		}
	}
	return service.deployments.List(ctx, organizationID, projectID, appID, limit)
}

func (service *DeploymentService) allocateID(prefix string) (string, error) {
	value, err := service.newID(prefix)
	if err != nil {
		return "", fmt.Errorf("allocate %s ID: %w", prefix, err)
	}
	if err := deployment.ValidateIdentifier(prefix+"_id", value); err != nil {
		return "", err
	}
	return value, nil
}

func validateLimit(limit int) error {
	if limit < 1 || limit > maximumPageSize {
		return fmt.Errorf(
			"%w: limit must be between 1 and %d", deployment.ErrInvalid, maximumPageSize,
		)
	}
	return nil
}

func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
