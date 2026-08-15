package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	appdomain "github.com/neurun-io/neurun/internal/domain/app"
	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/domain/deployment"
	githubdomain "github.com/neurun-io/neurun/internal/domain/github"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/files"
	"github.com/neurun-io/neurun/internal/github"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository/database"
)

// GitHubService turns a repository and a ref into a deployment. It owns no
// build logic: it fetches the source, then hands it to the deployment service
// exactly as an upload would arrive.
type GitHubService struct {
	client        *github.Client
	installations *database.GitHubInstallationRepository
	apps          *AppService
	deployments   *DeploymentService
	now           func() time.Time
	newID         func(string) (string, error)
}

func NewGitHubService(
	client *github.Client,
	installations *database.GitHubInstallationRepository,
	apps *AppService,
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
func (service *GitHubService) Configured() bool {
	return service != nil && service.client != nil
}

// ParsePush verifies a webhook delivery and returns the push it carries.
func (service *GitHubService) ParsePush(
	request *http.Request,
) (github.Push, bool, error) {
	if !service.Configured() {
		return github.Push{}, false, github.ErrNotConfigured
	}
	return service.client.ParsePush(request)
}

// Push builds the pushed commit for every app that follows the ref it names.
//
// The organization comes from the installation GitHub signed for, never from
// the delivery body, so a push can only ever reach the apps of the account the
// app is installed on. One app failing does not stop the rest.
func (service *GitHubService) Push(
	ctx context.Context,
	push github.Push,
) ([]deployment.Deployment, error) {
	if !service.Configured() {
		return nil, github.ErrNotConfigured
	}
	installation, err := service.installations.ByInstallationID(ctx, push.InstallationID)
	if err != nil {
		return nil, err
	}
	connected, err := service.apps.ConnectedTo(
		ctx, installation.OrganizationID, push.Repository,
	)
	if err != nil {
		return nil, err
	}

	var deployed []deployment.Deployment
	var problems []error
	for _, app := range connected {
		if !app.TracksRef(push.Ref, push.DefaultBranch) {
			continue
		}
		// The commit is taken from the delivery rather than resolved again: a
		// later push must not silently take this one's place.
		record, err := service.build(
			ctx, installation.OrganizationID, app, push.InstallationID,
			push.Ref, push.Commit,
		)
		if err != nil {
			problems = append(problems, fmt.Errorf("deploy app %s: %w", app.ID, err))
			continue
		}
		deployed = append(deployed, record)
	}
	return deployed, errors.Join(problems...)
}

// Install records the installation GitHub redirected back with. Whose account
// it is comes from GitHub, not from the caller: the redirect carries only an
// installation id, so the browser has nothing else to go on.
func (service *GitHubService) Install(
	ctx context.Context,
	organizationID string,
	installationID int64,
) (githubdomain.Installation, error) {
	if !service.Configured() {
		return githubdomain.Installation{}, github.ErrNotConfigured
	}
	accountLogin, err := service.client.Account(ctx, installationID)
	if err != nil {
		if errors.Is(err, github.ErrNotFound) {
			return githubdomain.Installation{}, githubdomain.ErrNoInstallation
		}
		return githubdomain.Installation{}, err
	}
	id, err := service.newID("ghi")
	if err != nil {
		return githubdomain.Installation{}, err
	}
	record, err := githubdomain.NewInstallation(
		id, organizationID, installationID, accountLogin, service.now(),
	)
	if err != nil {
		return githubdomain.Installation{}, err
	}
	return service.installations.Save(ctx, record)
}

func (service *GitHubService) Installation(
	ctx context.Context,
	organizationID string,
) (githubdomain.Installation, error) {
	return service.installations.ByOrganization(ctx, organizationID)
}

func (service *GitHubService) Uninstall(
	ctx context.Context,
	organizationID string,
) error {
	return service.installations.Delete(ctx, organizationID, service.now())
}

// Repositories lists what this organization's installation can read.
func (service *GitHubService) Repositories(
	ctx context.Context,
	organizationID string,
) ([]github.Repository, error) {
	if !service.Configured() {
		return nil, github.ErrNotConfigured
	}
	installation, err := service.installations.ByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	return service.client.Repositories(ctx, installation.InstallationID)
}

// Branches lists a repository's branches, for choosing a production ref.
func (service *GitHubService) Branches(
	ctx context.Context,
	organizationID string,
	repository string,
) ([]string, error) {
	if !service.Configured() {
		return nil, github.ErrNotConfigured
	}
	parsed, err := github.ParseRepo(repository)
	if err != nil {
		return nil, err
	}
	installation, err := service.installations.ByOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	return service.client.Branches(ctx, installation.InstallationID, parsed)
}

// CreateApp mints an app against a database. An app has no other source of
// code, so the repository is required and is proved readable before the app
// exists — a name that deploys nothing is worse than a refusal.
func (service *GitHubService) CreateApp(
	ctx context.Context,
	organizationID string,
	request dto.CreateAppRequest,
) (appdomain.App, error) {
	request.Repository = strings.TrimSpace(request.Repository)
	request.ProductionRef = strings.TrimSpace(request.ProductionRef)
	if request.Repository == "" {
		return appdomain.App{}, fmt.Errorf(
			"%w: repository is required", githubdomain.ErrInvalid,
		)
	}
	if !service.Configured() {
		return appdomain.App{}, github.ErrNotConfigured
	}
	installation, err := service.installations.ByOrganization(ctx, organizationID)
	if err != nil {
		return appdomain.App{}, err
	}
	parsed, err := github.ParseRepo(request.Repository)
	if err != nil {
		return appdomain.App{}, err
	}
	ref := request.ProductionRef
	if ref == "" {
		ref = "HEAD"
	}
	if _, err := service.client.ResolveRef(
		ctx, installation.InstallationID, parsed, ref,
	); err != nil {
		return appdomain.App{}, err
	}
	return service.apps.Create(ctx, organizationID, request)
}

// Connect points an app at a repository, verifying the installation can see it
// before storing anything.
func (service *GitHubService) Connect(
	ctx context.Context,
	organizationID string,
	appID string,
	request dto.ConnectRepositoryRequest,
) (appdomain.App, error) {
	app, err := service.apps.Get(ctx, organizationID, appID)
	if err != nil {
		return appdomain.App{}, err
	}
	repository := strings.TrimSpace(request.Repository)
	if repository != "" {
		if !service.Configured() {
			return appdomain.App{}, github.ErrNotConfigured
		}
		installation, err := service.installations.ByOrganization(ctx, organizationID)
		if err != nil {
			return appdomain.App{}, err
		}
		parsed, err := github.ParseRepo(repository)
		if err != nil {
			return appdomain.App{}, err
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
			return appdomain.App{}, err
		}
	}
	if err := app.Connect(
		repository, request.ProductionRef, service.now().UTC().Round(0),
	); err != nil {
		return appdomain.App{}, err
	}
	return service.apps.Save(ctx, organizationID, app)
}

// Deploy resolves a ref, downloads that commit, and builds it. An empty ref
// falls back to the app's production ref.
func (service *GitHubService) Deploy(
	ctx context.Context,
	organizationID string,
	appID string,
	ref string,
) (deployment.Deployment, error) {
	if !service.Configured() {
		return deployment.Deployment{}, github.ErrNotConfigured
	}
	app, err := service.apps.Get(ctx, organizationID, appID)
	if err != nil {
		return deployment.Deployment{}, err
	}
	if app.Repository == "" {
		return deployment.Deployment{}, githubdomain.ErrNotConnected
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
	return service.build(
		ctx, organizationID, app, installation.InstallationID, ref, commit,
	)
}

// build downloads one commit and hands it to the deployment service exactly as
// an upload would arrive, which is also what starts the build.
func (service *GitHubService) build(
	ctx context.Context,
	organizationID string,
	app appdomain.App,
	installationID int64,
	ref string,
	commit string,
) (deployment.Deployment, error) {
	parsed, err := github.ParseRepo(app.Repository)
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
		ctx, installationID, parsed, commit, archive,
	); err != nil {
		return deployment.Deployment{}, err
	}
	file, err := os.Open(archive)
	if err != nil {
		return deployment.Deployment{}, fmt.Errorf("stage github source: %w", err)
	}
	defer file.Close()

	// A push carries no runtime, and the repository is the only thing that knows
	// what it is: Cargo.toml is a Rust crate whoever pushed it.
	names, err := files.ZIPNames(archive)
	if err != nil {
		return deployment.Deployment{}, err
	}
	runtime, err := build.DetectRuntime(names)
	if err != nil {
		return deployment.Deployment{}, err
	}
	return service.deployments.Create(ctx, organizationID, dto.CreateDeploymentRequest{
		AppID:      app.ID,
		Runtime:    runtime,
		Source:     file,
		CommitSHA:  commit,
		GitRef:     ref,
	})
}

// Retry builds the same commit again. It deploys what the original deployment
// built, not what the ref points at now — a ref moves, and a retry that
// silently built something else would not be a retry.
func (service *GitHubService) Retry(
	ctx context.Context,
	organizationID string,
	deploymentID string,
) (deployment.Deployment, error) {
	if !service.Configured() {
		return deployment.Deployment{}, github.ErrNotConfigured
	}
	record, err := service.deployments.Get(ctx, organizationID, deploymentID)
	if err != nil {
		return deployment.Deployment{}, err
	}
	if record.CommitSHA == "" {
		return deployment.Deployment{}, fmt.Errorf(
			"%w: deployment carries no commit to build again", githubdomain.ErrInvalid,
		)
	}
	app, err := service.apps.Get(ctx, organizationID, record.AppID)
	if err != nil {
		return deployment.Deployment{}, err
	}
	if app.Repository == "" {
		return deployment.Deployment{}, githubdomain.ErrNotConnected
	}
	installation, err := service.installations.ByOrganization(ctx, organizationID)
	if err != nil {
		return deployment.Deployment{}, err
	}
	return service.build(
		ctx, organizationID, app, installation.InstallationID,
		record.GitRef, record.CommitSHA,
	)
}
