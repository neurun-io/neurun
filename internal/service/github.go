package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neurun-io/neurun/internal/domain/deployment"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/github"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository"
)

// GitHubService turns a repository and a ref into a deployment. It owns no
// build logic: it fetches the source, then hands it to the deployment service
// exactly as an upload would arrive.
type GitHubService struct {
	client        *github.Client
	installations *repository.GitHubInstallationRepository
	apps          *repository.AppRepository
	deployments   *DeploymentService
	now           func() time.Time
	newID         func(string) (string, error)
}

func NewGitHubService(
	client *github.Client,
	installations *repository.GitHubInstallationRepository,
	apps *repository.AppRepository,
	deployments *DeploymentService,
	now func() time.Time,
	newID func(string) (string, error),
) (*GitHubService, error) {
	if installations == nil || apps == nil || deployments == nil {
		return nil, errors.New("github service requires its repositories")
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if newID == nil {
		newID = ids.New
	}
	return &GitHubService{
		client: client, installations: installations, apps: apps,
		deployments: deployments, now: now, newID: newID,
	}, nil
}

// Configured reports whether the server holds GitHub App credentials. Without
// them every route here refuses rather than failing halfway through a deploy.
func (service *GitHubService) Configured() bool { return service.client != nil }

func (service *GitHubService) Install(
	ctx context.Context,
	organizationID string,
	installationID int64,
	accountLogin string,
) (deployment.Installation, error) {
	if !service.Configured() {
		return deployment.Installation{}, github.ErrNotConfigured
	}
	id, err := service.newID("ghi")
	if err != nil {
		return deployment.Installation{}, err
	}
	record, err := deployment.NewInstallation(
		id, organizationID, installationID, accountLogin, service.now(),
	)
	if err != nil {
		return deployment.Installation{}, err
	}
	return service.installations.Save(ctx, record)
}

func (service *GitHubService) Installation(
	ctx context.Context,
	organizationID string,
) (deployment.Installation, error) {
	return service.installations.ByOrganization(ctx, organizationID)
}

func (service *GitHubService) Uninstall(
	ctx context.Context,
	organizationID string,
) error {
	return service.installations.Delete(ctx, organizationID, service.now())
}

// Connect points an app at a repository, verifying the installation can see it
// before storing anything.
func (service *GitHubService) Connect(
	ctx context.Context,
	organizationID string,
	appID string,
	request dto.ConnectRepositoryRequest,
) (deployment.App, error) {
	app, err := service.apps.GetByID(ctx, organizationID, appID)
	if err != nil {
		return deployment.App{}, err
	}
	repository := strings.TrimSpace(request.Repository)
	if repository != "" {
		if !service.Configured() {
			return deployment.App{}, github.ErrNotConfigured
		}
		installation, err := service.installations.ByOrganization(ctx, organizationID)
		if err != nil {
			return deployment.App{}, err
		}
		parsed, err := github.ParseRepo(repository)
		if err != nil {
			return deployment.App{}, err
		}
		ref := request.ProductionRef
		if strings.TrimSpace(ref) == "" {
			ref = "HEAD"
		}
		// Resolving proves the installation can actually read it, so a
		// misconfigured connection fails now rather than on the first deploy.
		if _, err := service.client.ResolveRef(
			ctx, installation.InstallationID, parsed, ref,
		); err != nil {
			return deployment.App{}, err
		}
	}
	if err := app.Connect(
		repository, request.ProductionRef, service.now().UTC().Round(0),
	); err != nil {
		return deployment.App{}, err
	}
	return service.apps.Update(ctx, organizationID, app)
}

// Deploy resolves a ref, downloads that commit, and builds it. An empty ref
// falls back to the app's production ref.
func (service *GitHubService) Deploy(
	ctx context.Context,
	organizationID string,
	appID string,
	ref string,
	runtime deployment.Runtime,
	entryPoint string,
) (deployment.Deployment, error) {
	if !service.Configured() {
		return deployment.Deployment{}, github.ErrNotConfigured
	}
	app, err := service.apps.GetByID(ctx, organizationID, appID)
	if err != nil {
		return deployment.Deployment{}, err
	}
	if app.Repository == "" {
		return deployment.Deployment{}, deployment.ErrNotConnected
	}
	installation, err := service.installations.ByOrganization(ctx, organizationID)
	if err != nil {
		return deployment.Deployment{}, err
	}
	parsed, err := github.ParseRepo(app.Repository)
	if err != nil {
		return deployment.Deployment{}, err
	}
	if strings.TrimSpace(ref) == "" {
		ref = app.ProductionRef
	}
	if strings.TrimSpace(ref) == "" {
		ref = "HEAD"
	}

	commit, err := service.client.ResolveRef(ctx, installation.InstallationID, parsed, ref)
	if err != nil {
		return deployment.Deployment{}, err
	}

	directory, err := os.MkdirTemp("", "neurun-github-")
	if err != nil {
		return deployment.Deployment{}, fmt.Errorf("stage github source: %w", err)
	}
	defer os.RemoveAll(directory)
	archive := filepath.Join(directory, "source.zip")

	if _, err := service.client.Source(
		ctx, installation.InstallationID, parsed, commit, archive,
	); err != nil {
		return deployment.Deployment{}, err
	}
	file, err := os.Open(archive)
	if err != nil {
		return deployment.Deployment{}, fmt.Errorf("stage github source: %w", err)
	}
	defer file.Close()

	if !runtime.Valid() {
		runtime = deployment.RuntimePython
	}
	return service.deployments.Create(ctx, organizationID, dto.CreateDeploymentRequest{
		AppID:      app.ID,
		Runtime:    runtime,
		EntryPoint: entryPoint,
		SourceName: fmt.Sprintf("%s-%s.zip", parsed.Name, commit[:7]),
		Source:     file,
		CommitSHA:  commit,
		GitRef:     ref,
	})
}
