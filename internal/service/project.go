package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neurun-io/neurun/internal/domain/project"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository/database"
)

// ProjectService owns an organization's projects. It knows nothing about what
// is deployed under them: everything below a project cascades in storage.
type ProjectService struct {
	projects *database.ProjectRepository
	now      func() time.Time
	newID    func(string) (string, error)
}

func NewProjectService(
	projects *database.ProjectRepository,
	now func() time.Time,
	newID func(string) (string, error),
) (*ProjectService, error) {
	if projects == nil {
		return nil, errors.New("project service requires its repository")
	}
	if now == nil {
		now = time.Now
	}
	if newID == nil {
		newID = ids.New
	}
	return &ProjectService{projects: projects, now: now, newID: newID}, nil
}

// Create mints a project. Nothing creates one implicitly: a project is only
// ever brought into being by an explicit call.
func (service *ProjectService) Create(
	ctx context.Context,
	organizationID string,
	name string,
) (project.Project, error) {
	id, err := service.newID("prj")
	if err != nil {
		return project.Project{}, err
	}
	record, err := project.New(
		id, organizationID, name, service.now().UTC().Round(0),
	)
	if err != nil {
		return project.Project{}, err
	}
	return service.projects.Create(ctx, record)
}

func (service *ProjectService) Get(
	ctx context.Context,
	organizationID string,
	projectID string,
) (project.Project, error) {
	return service.projects.GetByID(ctx, organizationID, projectID)
}

func (service *ProjectService) List(
	ctx context.Context,
	organizationID string,
	limit int,
) ([]project.Project, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	return service.projects.List(ctx, organizationID, limit)
}

func (service *ProjectService) Update(
	ctx context.Context,
	organizationID string,
	projectID string,
	request dto.UpdateProjectRequest,
) (project.Project, error) {
	if request.Name == nil {
		return project.Project{}, fmt.Errorf(
			"%w: project update is empty", project.ErrInvalid,
		)
	}
	record, err := service.projects.GetByID(ctx, organizationID, projectID)
	if err != nil {
		return project.Project{}, err
	}
	if err := record.Rename(*request.Name, service.now().UTC().Round(0)); err != nil {
		return project.Project{}, err
	}
	return service.projects.Update(ctx, record)
}

// Delete destroys a project and everything beneath it — apps, deployments,
// builds, executions, users and API keys all cascade. Blob payloads in the
// artifact store are left alone; they are content-addressed and shared, so
// removing them belongs to a separate sweep.
func (service *ProjectService) Delete(
	ctx context.Context,
	organizationID string,
	projectID string,
) error {
	return service.projects.Delete(ctx, organizationID, projectID)
}
