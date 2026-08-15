package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appdomain "github.com/neurun-io/neurun/internal/domain/app"
	builddomain "github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository/database"
)

// AppService owns the apps under a project. It reaches the project repository
// only to refuse an app whose project does not exist, and the build repository
// only to refuse a build that is not the app's.
type AppService struct {
	projects *database.ProjectRepository
	apps     *database.AppRepository
	builds   *database.BuildRepository
	now      func() time.Time
	newID    func(string) (string, error)
}

func NewAppService(
	projects *database.ProjectRepository,
	apps *database.AppRepository,
	builds *database.BuildRepository,
	now func() time.Time,
	newID func(string) (string, error),
) (*AppService, error) {
	if projects == nil || apps == nil || builds == nil {
		return nil, errors.New("app service requires its repositories")
	}
	if now == nil {
		now = time.Now
	}
	if newID == nil {
		newID = ids.New
	}
	return &AppService{
		projects: projects, apps: apps, builds: builds,
		now: now, newID: newID,
	}, nil
}

func (service *AppService) Create(
	ctx context.Context,
	organizationID string,
	request dto.CreateAppRequest,
) (appdomain.App, error) {
	if err := appdomain.ValidateIdentifier("project_id", request.ProjectID); err != nil {
		return appdomain.App{}, err
	}
	if _, err := service.projects.GetByID(
		ctx, organizationID, request.ProjectID,
	); err != nil {
		return appdomain.App{}, err
	}
	id, err := service.newID("app")
	if err != nil {
		return appdomain.App{}, err
	}
	now := service.now().UTC().Round(0)
	record, err := appdomain.New(id, request.ProjectID, request.Name, now)
	if err != nil {
		return appdomain.App{}, err
	}
	// Connected before the insert rather than after: an app that exists without
	// its repository is an app nothing can ever deploy to.
	if err := record.Connect(request.Repository, request.ProductionRef, now); err != nil {
		return appdomain.App{}, err
	}
	return service.apps.Create(ctx, record)
}

func (service *AppService) Get(
	ctx context.Context,
	organizationID string,
	appID string,
) (appdomain.App, error) {
	return service.apps.GetByID(ctx, organizationID, appID)
}

func (service *AppService) List(
	ctx context.Context,
	organizationID string,
	projectID string,
	name string,
	limit int,
) ([]appdomain.App, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	if err := appdomain.ValidateNameFilter(name); err != nil {
		return nil, err
	}
	return service.apps.List(ctx, organizationID, projectID, name, limit)
}

func (service *AppService) Update(
	ctx context.Context,
	organizationID string,
	appID string,
	request dto.UpdateAppRequest,
) (appdomain.App, error) {
	if request.Name == nil {
		return appdomain.App{}, fmt.Errorf(
			"%w: app update is empty", appdomain.ErrInvalid,
		)
	}
	record, err := service.apps.GetByID(ctx, organizationID, appID)
	if err != nil {
		return appdomain.App{}, err
	}
	if err := record.Rename(*request.Name, service.now().UTC().Round(0)); err != nil {
		return appdomain.App{}, err
	}
	return service.apps.Update(ctx, organizationID, record)
}

// ActivateBuild pins the build the app runs, or releases the pin when buildID
// is empty. The build has to be one of this app's — pinning another app's would
// only fail at the next run.
func (service *AppService) ActivateBuild(
	ctx context.Context,
	organizationID string,
	appID string,
	buildID string,
) (appdomain.App, error) {
	record, err := service.apps.GetByID(ctx, organizationID, appID)
	if err != nil {
		return appdomain.App{}, err
	}
	buildID = strings.TrimSpace(buildID)
	if buildID != "" {
		produced, err := service.builds.Get(ctx, organizationID, buildID)
		if err != nil {
			return appdomain.App{}, err
		}
		if produced.AppID != appID {
			return appdomain.App{}, fmt.Errorf(
				"%w: build %s belongs to another app", builddomain.ErrNotFound, buildID,
			)
		}
	}
	if err := record.ActivateBuild(buildID, service.now().UTC().Round(0)); err != nil {
		return appdomain.App{}, err
	}
	return service.apps.Update(ctx, organizationID, record)
}

// ConnectedTo lists the apps of one repository, which is what a push has to be
// matched against.
func (service *AppService) ConnectedTo(
	ctx context.Context,
	organizationID string,
	repository string,
) ([]appdomain.App, error) {
	return service.apps.ConnectedTo(ctx, organizationID, repository)
}

// Save stores an app a caller has already transitioned, which is how the GitHub
// service writes a connection it verified first.
func (service *AppService) Save(
	ctx context.Context,
	organizationID string,
	record appdomain.App,
) (appdomain.App, error) {
	return service.apps.Update(ctx, organizationID, record)
}

// Delete destroys an app and the deployments, builds and executions under it.
func (service *AppService) Delete(
	ctx context.Context,
	organizationID string,
	appID string,
) error {
	return service.apps.Delete(ctx, organizationID, appID)
}
