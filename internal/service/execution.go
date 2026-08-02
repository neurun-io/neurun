package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neurun-io/neurun/internal/domain/deployment"
	"github.com/neurun-io/neurun/internal/domain/execution"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository"
)

const DefaultMaxExecutionInputBytes = int64(1 << 20)

type ExecutionOptions struct {
	MaxInputBytes int64
	Now           func() time.Time
	NewID         func(string) (string, error)
}

type ExecutionService struct {
	executions    *repository.ExecutionRepository
	deployments   *repository.DeploymentRepository
	maxInputBytes int64
	now           func() time.Time
	newID         func(string) (string, error)
}

func NewExecutionService(
	executions *repository.ExecutionRepository,
	deployments *repository.DeploymentRepository,
	options ExecutionOptions,
) (*ExecutionService, error) {
	switch {
	case executions == nil || deployments == nil:
		return nil, errors.New("execution service requires its repositories")
	case options.MaxInputBytes < 0:
		return nil, errors.New("maximum execution input bytes cannot be negative")
	}
	if options.MaxInputBytes == 0 {
		options.MaxInputBytes = DefaultMaxExecutionInputBytes
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.NewID == nil {
		options.NewID = ids.New
	}
	return &ExecutionService{
		executions: executions, deployments: deployments,
		maxInputBytes: options.MaxInputBytes,
		now:           options.Now, newID: options.NewID,
	}, nil
}

// Create queues an invocation against whichever build is ready now, and pins it
// there — a later rebuild will not move it.
func (service *ExecutionService) Create(
	ctx context.Context,
	request dto.CreateExecutionRequest,
) (execution.Execution, error) {
	ctx = orBackground(ctx)
	if err := ids.Validate("deployment_id", request.DeploymentID); err != nil {
		return execution.Execution{}, fmt.Errorf("%w: %v", execution.ErrInvalid, err)
	}
	input, err := execution.NormalizeInput(request.Input, service.maxInputBytes)
	if err != nil {
		return execution.Execution{}, err
	}
	// The deployment carries the project; the caller does not supply one.
	record, err := service.deployments.GetByID(ctx, request.DeploymentID)
	if err != nil {
		return execution.Execution{}, err
	}
	build, ok := record.ReadyBuild()
	if !ok {
		return execution.Execution{}, fmt.Errorf(
			"%w: %s", deployment.ErrNoReadyBuild, record.ID,
		)
	}
	id, err := service.allocateID()
	if err != nil {
		return execution.Execution{}, err
	}
	queued, err := execution.New(
		id, record.ProjectID, record.ID, build.ID, input,
		service.now().UTC().Round(0),
	)
	if err != nil {
		return execution.Execution{}, err
	}
	if err := service.executions.Create(ctx, queued); err != nil {
		return execution.Execution{}, fmt.Errorf("persist queued execution: %w", err)
	}
	return queued, nil
}

func (service *ExecutionService) Get(
	ctx context.Context,
	executionID string,
) (execution.Execution, error) {
	if err := ids.Validate("execution_id", executionID); err != nil {
		return execution.Execution{}, fmt.Errorf("%w: %v", execution.ErrInvalid, err)
	}
	return service.executions.GetByID(ctx, executionID)
}

// List returns executions across the principal's project, or those of one
// deployment. Filtering by deployment confirms ownership first, so a foreign
// identifier reads as not found rather than as an empty list.
func (service *ExecutionService) List(
	ctx context.Context,
	projectID string,
	deploymentID string,
	limit int,
) ([]execution.Execution, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	if projectID != "" {
		if err := ids.Validate("project_id", projectID); err != nil {
			return nil, fmt.Errorf("%w: %v", execution.ErrInvalid, err)
		}
	}
	if deploymentID != "" {
		if _, err := service.deployments.GetByID(ctx, deploymentID); err != nil {
			return nil, err
		}
	}
	return service.executions.List(ctx, projectID, deploymentID, limit)
}

func (service *ExecutionService) ListForDeployment(
	ctx context.Context,
	deploymentID string,
	limit int,
) ([]execution.Execution, error) {
	if deploymentID == "" {
		return nil, fmt.Errorf("%w: deployment_id is required", execution.ErrInvalid)
	}
	return service.List(ctx, "", deploymentID, limit)
}

// Rerun repeats a finished execution against the build it was pinned to, which
// must still be ready.
func (service *ExecutionService) Rerun(
	ctx context.Context,
	executionID string,
) (execution.Execution, error) {
	ctx = orBackground(ctx)
	original, err := service.Get(ctx, executionID)
	if err != nil {
		return execution.Execution{}, err
	}
	record, err := service.deployments.GetByID(ctx, original.DeploymentID)
	if err != nil {
		return execution.Execution{}, err
	}
	build, exists := record.BuildByID(original.BuildID)
	if !exists || build.Status != deployment.StatusReady {
		return execution.Execution{}, fmt.Errorf(
			"%w: pinned build %s", deployment.ErrNoReadyBuild, original.BuildID,
		)
	}
	id, err := service.allocateID()
	if err != nil {
		return execution.Execution{}, err
	}
	rerun, err := original.Rerun(id, service.now().UTC().Round(0))
	if err != nil {
		return execution.Execution{}, err
	}
	if err := service.executions.Create(ctx, rerun); err != nil {
		return execution.Execution{}, fmt.Errorf("persist queued rerun: %w", err)
	}
	return rerun, nil
}

func (service *ExecutionService) allocateID() (string, error) {
	value, err := service.newID("exe")
	if err != nil {
		return "", fmt.Errorf("allocate execution ID: %w", err)
	}
	if err := ids.Validate("execution_id", value); err != nil {
		return "", fmt.Errorf("%w: %v", execution.ErrInvalid, err)
	}
	return value, nil
}
