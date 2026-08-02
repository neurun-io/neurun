// Package service holds the application logic: it drives domain records
// through their transitions and hands them to repositories to persist.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/neurun-io/neurun/internal/artifact"
	"github.com/neurun-io/neurun/internal/builder"
	"github.com/neurun-io/neurun/internal/domain/deployment"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository"
)

const (
	DefaultMaxSourceBytes   = int64(32 << 20)
	DefaultMaxArtifactBytes = int64(256 << 20)
	DefaultBuildTimeout     = 5 * time.Minute
	maximumPageSize         = 200
)

type DeploymentOptions struct {
	MaxSourceBytes   int64
	MaxArtifactBytes int64
	BuildTimeout     time.Duration
	Now              func() time.Time
	NewID            func(string) (string, error)
}

type DeploymentService struct {
	projects    *repository.ProjectRepository
	apps        *repository.AppRepository
	deployments *repository.DeploymentRepository
	blobs       *artifact.LocalStore
	builder     builder.Builder

	maxSourceBytes   int64
	maxArtifactBytes int64
	buildTimeout     time.Duration
	now              func() time.Time
	newID            func(string) (string, error)
	buildMu          sync.Mutex
}

func NewDeploymentService(
	projects *repository.ProjectRepository,
	apps *repository.AppRepository,
	deployments *repository.DeploymentRepository,
	blobs *artifact.LocalStore,
	toolchain builder.Builder,
	options DeploymentOptions,
) (*DeploymentService, error) {
	switch {
	case projects == nil || apps == nil || deployments == nil:
		return nil, errors.New("deployment service requires its repositories")
	case blobs == nil:
		return nil, errors.New("deployment service requires an artifact store")
	case toolchain == nil:
		return nil, errors.New("deployment service requires a builder")
	case options.MaxSourceBytes < 0 || options.MaxArtifactBytes < 0 ||
		options.BuildTimeout < 0:
		return nil, errors.New("deployment service limits cannot be negative")
	}
	if options.MaxSourceBytes == 0 {
		options.MaxSourceBytes = DefaultMaxSourceBytes
	}
	if options.MaxArtifactBytes == 0 {
		options.MaxArtifactBytes = DefaultMaxArtifactBytes
	}
	if options.BuildTimeout == 0 {
		options.BuildTimeout = DefaultBuildTimeout
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.NewID == nil {
		options.NewID = ids.New
	}
	return &DeploymentService{
		projects: projects, apps: apps, deployments: deployments,
		blobs: blobs, builder: toolchain,
		maxSourceBytes:   options.MaxSourceBytes,
		maxArtifactBytes: options.MaxArtifactBytes,
		buildTimeout:     options.BuildTimeout,
		now:              options.Now,
		newID:            options.NewID,
	}, nil
}

func (service *DeploymentService) Create(
	ctx context.Context,
	request dto.CreateDeploymentRequest,
) (deployment.Deployment, error) {
	ctx = orBackground(ctx)
	if err := deployment.ValidateIdentifier("app_id", request.AppID); err != nil {
		return deployment.Deployment{}, err
	}
	// The app decides the project. An SDK cannot conjure one by naming it, and an
	// app that does not already exist is refused rather than created.
	app, err := service.apps.GetByID(ctx, request.AppID)
	if err != nil {
		return deployment.Deployment{}, err
	}
	if !request.Runtime.Valid() {
		return deployment.Deployment{}, fmt.Errorf(
			"%w: runtime must be python", deployment.ErrInvalid,
		)
	}
	if request.Source == nil {
		return deployment.Deployment{}, fmt.Errorf(
			"%w: source ZIP is required", deployment.ErrInvalid,
		)
	}
	sourceName, err := normalizeSourceName(request.SourceName)
	if err != nil {
		return deployment.Deployment{}, err
	}

	sourcePath, sourceInfo, cleanup, err := spoolSource(
		ctx, request.Source, service.maxSourceBytes,
	)
	if err != nil {
		if errors.Is(err, artifact.ErrByteLimitExceeded) {
			return deployment.Deployment{}, fmt.Errorf(
				"%w: source ZIP exceeds %d bytes",
				deployment.ErrSourceTooLarge, service.maxSourceBytes,
			)
		}
		return deployment.Deployment{}, fmt.Errorf("stage deployment source: %w", err)
	}
	defer cleanup()

	deploymentID, err := service.allocateID("dep")
	if err != nil {
		return deployment.Deployment{}, err
	}
	sourceID, err := service.allocateID("art")
	if err != nil {
		return deployment.Deployment{}, err
	}
	storageKey, err := service.putContentAddressed(
		ctx, sourcePath, sourceInfo, service.maxSourceBytes,
	)
	if err != nil {
		return deployment.Deployment{}, fmt.Errorf("store deployment source: %w", err)
	}

	now := service.now().UTC().Round(0)
	record, err := deployment.New(
		deploymentID, app.ProjectID, app.ID,
		request.Runtime, request.EntryPoint,
		deployment.Artifact{
			ID: sourceID, Kind: deployment.ArtifactSource, Name: sourceName,
			MediaType: "application/zip", SizeBytes: sourceInfo.SizeBytes,
			SHA256: sourceInfo.SHA256, StorageKey: storageKey, CreatedAt: now,
		},
		now,
	)
	if err != nil {
		return deployment.Deployment{}, err
	}
	if err := service.deployments.Save(ctx, record); err != nil {
		return deployment.Deployment{}, fmt.Errorf("persist uploaded deployment: %w", err)
	}
	return service.runBuild(ctx, record)
}

func (service *DeploymentService) Get(
	ctx context.Context,
	deploymentID string,
) (deployment.Deployment, error) {
	return service.deployments.GetByID(ctx, deploymentID)
}

func (service *DeploymentService) List(
	ctx context.Context,
	projectID string,
	appID string,
	limit int,
) ([]deployment.Deployment, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	if appID != "" {
		if _, err := service.apps.GetByID(ctx, appID); err != nil {
			return nil, err
		}
	}
	return service.deployments.List(ctx, projectID, appID, limit)
}

func (service *DeploymentService) GetBuild(
	ctx context.Context,
	buildID string,
) (deployment.Build, error) {
	return service.deployments.GetBuild(ctx, buildID)
}

func (service *DeploymentService) ListBuilds(
	ctx context.Context,
	deploymentID string,
	limit int,
) ([]deployment.Build, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	if deploymentID != "" {
		if _, err := service.deployments.GetByID(ctx, deploymentID); err != nil {
			return nil, err
		}
	}
	return service.deployments.ListBuilds(ctx, deploymentID, limit)
}

// RecoverInterruptedBuilds marks builds a prior process crash left building as
// failed. It never retries the build's side effects.
func (service *DeploymentService) RecoverInterruptedBuilds(
	ctx context.Context,
) (int, error) {
	return service.deployments.RecoverBuilding(
		ctx,
		service.now().UTC().Round(0),
		deployment.Failure{
			Code:    "build_interrupted",
			Message: "build was interrupted by a service restart",
		},
	)
}

func (service *DeploymentService) EnsureProject(
	ctx context.Context,
	projectID string,
	name string,
) (deployment.Project, error) {
	if err := deployment.ValidateIdentifier("project_id", projectID); err != nil {
		return deployment.Project{}, err
	}
	now := service.now().UTC().Round(0)
	record, err := deployment.NewProject(projectID, name, now)
	if err != nil {
		return deployment.Project{}, err
	}
	return service.projects.Ensure(ctx, record)
}

// CreateProject mints a project. Nothing creates one implicitly: a project is
// only ever brought into being by an explicit call.
func (service *DeploymentService) CreateProject(
	ctx context.Context,
	name string,
) (deployment.Project, error) {
	id, err := service.allocateID("prj")
	if err != nil {
		return deployment.Project{}, err
	}
	record, err := deployment.NewProject(id, name, service.now().UTC().Round(0))
	if err != nil {
		return deployment.Project{}, err
	}
	return service.projects.Create(ctx, record)
}

// DeleteProject destroys a project and everything beneath it — apps,
// deployments, builds, executions, users and API keys all cascade. Blob
// payloads in the artifact store are left alone; they are content-addressed and
// shared, so removing them belongs to a separate sweep.
func (service *DeploymentService) DeleteProject(
	ctx context.Context,
	projectID string,
) error {
	return service.projects.Delete(ctx, projectID)
}

// DeleteApp destroys an app and the deployments, builds and executions under it.
func (service *DeploymentService) DeleteApp(ctx context.Context, appID string) error {
	return service.apps.Delete(ctx, appID)
}

func (service *DeploymentService) GetProject(
	ctx context.Context,
	projectID string,
) (deployment.Project, error) {
	return service.projects.GetByID(ctx, projectID)
}

func (service *DeploymentService) ListProjects(
	ctx context.Context,
	limit int,
) ([]deployment.Project, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	return service.projects.List(ctx, limit)
}

func (service *DeploymentService) UpdateProject(
	ctx context.Context,
	projectID string,
	request dto.UpdateProjectRequest,
) (deployment.Project, error) {
	if request.Name == nil {
		return deployment.Project{}, fmt.Errorf(
			"%w: project update is empty", deployment.ErrInvalid,
		)
	}
	record, err := service.projects.GetByID(ctx, projectID)
	if err != nil {
		return deployment.Project{}, err
	}
	if err := record.Rename(*request.Name, service.now().UTC().Round(0)); err != nil {
		return deployment.Project{}, err
	}
	return service.projects.Update(ctx, record)
}

func (service *DeploymentService) CreateApp(
	ctx context.Context,
	request dto.CreateAppRequest,
) (deployment.App, error) {
	if err := deployment.ValidateIdentifier("project_id", request.ProjectID); err != nil {
		return deployment.App{}, err
	}
	if _, err := service.projects.GetByID(ctx, request.ProjectID); err != nil {
		return deployment.App{}, err
	}
	id, err := service.allocateID("app")
	if err != nil {
		return deployment.App{}, err
	}
	record, err := deployment.NewApp(
		id, request.ProjectID, request.Name, service.now().UTC().Round(0),
	)
	if err != nil {
		return deployment.App{}, err
	}
	return service.apps.Create(ctx, record)
}

func (service *DeploymentService) GetApp(
	ctx context.Context,
	appID string,
) (deployment.App, error) {
	return service.apps.GetByID(ctx, appID)
}

func (service *DeploymentService) ListApps(
	ctx context.Context,
	projectID string,
	name string,
	limit int,
) ([]deployment.App, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	if err := deployment.ValidateAppNameFilter(name); err != nil {
		return nil, err
	}
	return service.apps.List(ctx, projectID, name, limit)
}

func (service *DeploymentService) UpdateApp(
	ctx context.Context,
	appID string,
	request dto.UpdateAppRequest,
) (deployment.App, error) {
	if request.Name == nil {
		return deployment.App{}, fmt.Errorf(
			"%w: app update is empty", deployment.ErrInvalid,
		)
	}
	record, err := service.apps.GetByID(ctx, appID)
	if err != nil {
		return deployment.App{}, err
	}
	if err := record.Rename(*request.Name, service.now().UTC().Round(0)); err != nil {
		return deployment.App{}, err
	}
	return service.apps.Update(ctx, record)
}

// runBuild is serialized process-wide: a build spends minutes in a toolchain,
// and two concurrent ones would race on the same deployment row.
func (service *DeploymentService) runBuild(
	ctx context.Context,
	record deployment.Deployment,
) (deployment.Deployment, error) {
	service.buildMu.Lock()
	defer service.buildMu.Unlock()

	current, err := service.deployments.GetByID(ctx, record.ID)
	if err == nil {
		record = current
	} else if !errors.Is(err, deployment.ErrNotFound) {
		return deployment.Deployment{}, err
	}
	buildID, err := service.allocateID("bld")
	if err != nil {
		return deployment.Deployment{}, err
	}
	if _, err := record.StartBuild(buildID, service.now().UTC().Round(0)); err != nil {
		return deployment.Deployment{}, err
	}
	if err := service.deployments.Save(ctx, record); err != nil {
		return deployment.Deployment{}, fmt.Errorf("persist deployment build start: %w", err)
	}

	buildCtx, cancel := context.WithTimeout(ctx, service.buildTimeout)
	defer cancel()
	workDirectory, err := os.MkdirTemp("", "neurun-build-*")
	if err != nil {
		return service.failBuild(ctx, record, buildID, "build_environment", err)
	}
	defer os.RemoveAll(workDirectory)

	sourcePath := filepath.Join(workDirectory, "source.zip")
	if err := service.materialize(buildCtx, record.Source, sourcePath); err != nil {
		return service.failBuild(ctx, record, buildID, "source_unavailable", err)
	}
	result, buildErr := service.builder.Build(buildCtx, builder.Request{
		Runtime: record.Runtime, EntryPoint: record.EntryPoint,
		SourceArchivePath: sourcePath, WorkDirectory: workDirectory,
	})
	if buildErr != nil {
		code := "build_failed"
		if errors.Is(buildErr, context.DeadlineExceeded) ||
			errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			code = "build_timeout"
		}
		return service.failBuild(ctx, record, buildID, code, buildErr)
	}
	if len(result.Artifacts) == 0 {
		return service.failBuild(ctx, record, buildID, "build_failed",
			errors.New("builder produced no artifacts"))
	}
	stored, err := service.storeBuildArtifacts(buildCtx, result.Artifacts)
	if err != nil {
		return service.failBuild(ctx, record, buildID, "artifact_store_failed", err)
	}
	if err := record.MarkBuildReady(
		buildID, stored, service.now().UTC().Round(0),
	); err != nil {
		return deployment.Deployment{}, err
	}
	if err := service.deployments.Save(ctx, record); err != nil {
		return deployment.Deployment{}, fmt.Errorf("persist completed deployment build: %w", err)
	}
	return deployment.CloneDeployment(record), nil
}

func (service *DeploymentService) failBuild(
	ctx context.Context,
	record deployment.Deployment,
	buildID string,
	code string,
	cause error,
) (deployment.Deployment, error) {
	if err := record.FailBuild(
		buildID,
		deployment.Failure{Code: code, Message: boundedError(cause)},
		service.now().UTC().Round(0),
	); err != nil {
		return deployment.Deployment{}, err
	}
	// The failure must be recorded even when the request that triggered the
	// build has already gone away.
	saveCtx := ctx
	if saveCtx == nil || saveCtx.Err() != nil {
		saveCtx = context.WithoutCancel(orBackground(ctx))
	}
	if err := service.deployments.Save(saveCtx, record); err != nil {
		return deployment.CloneDeployment(record), fmt.Errorf(
			"persist failed deployment build: %w", err,
		)
	}
	return deployment.CloneDeployment(record), nil
}

func (service *DeploymentService) materialize(
	ctx context.Context,
	stored deployment.Artifact,
	targetPath string,
) error {
	reader, info, err := service.blobs.Open(ctx, stored.StorageKey)
	if err != nil {
		return err
	}
	defer reader.Close()
	if info.SHA256() != stored.SHA256 || info.SizeBytes() != stored.SizeBytes {
		return errors.New("stored artifact bytes do not match deployment metadata")
	}
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	result, copyErr := artifact.CopyAndHashContext(ctx, target, reader, stored.SizeBytes)
	closeErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if result.SHA256 != stored.SHA256 || result.SizeBytes != stored.SizeBytes {
		return errors.New("materialized artifact bytes do not match deployment metadata")
	}
	return nil
}

func (service *DeploymentService) storeBuildArtifacts(
	ctx context.Context,
	outputs []builder.Output,
) ([]deployment.Artifact, error) {
	remaining := service.maxArtifactBytes
	seenKinds := make(map[string]struct{}, len(outputs))
	stored := make([]deployment.Artifact, 0, len(outputs))
	for _, output := range outputs {
		if err := validateBuiltArtifact(output); err != nil {
			return nil, err
		}
		if _, duplicate := seenKinds[output.Kind]; duplicate {
			return nil, fmt.Errorf("builder returned duplicate %s artifact", output.Kind)
		}
		seenKinds[output.Kind] = struct{}{}
		info, err := artifact.HashFile(output.Path, remaining)
		if err != nil {
			return nil, err
		}
		storageKey, err := service.putContentAddressed(ctx, output.Path, info, remaining)
		if err != nil {
			return nil, err
		}
		artifactID, err := service.allocateID("art")
		if err != nil {
			return nil, err
		}
		remaining -= info.SizeBytes
		stored = append(stored, deployment.Artifact{
			ID: artifactID, Kind: output.Kind, Name: output.Name,
			MediaType: output.MediaType, SizeBytes: info.SizeBytes,
			SHA256: info.SHA256, StorageKey: storageKey,
			CreatedAt: service.now().UTC().Round(0),
		})
	}
	if _, exists := seenKinds[deployment.ArtifactCodeLayer]; !exists {
		return nil, errors.New("builder did not produce a code layer")
	}
	return stored, nil
}

// putContentAddressed stores bytes under their own digest. An existing object
// is accepted only after its digest is confirmed, so a corrupted blob can never
// be silently reused.
func (service *DeploymentService) putContentAddressed(
	ctx context.Context,
	filePath string,
	expected artifact.CopyResult,
	maximumBytes int64,
) (string, error) {
	if len(expected.SHA256) != 64 || expected.SizeBytes < 0 {
		return "", errors.New("content digest metadata is invalid")
	}
	storageKey := path.Join("objects", "sha256", expected.SHA256[:2], expected.SHA256)
	source, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	info, putErr := service.blobs.Put(ctx, storageKey, source, maximumBytes)
	closeErr := source.Close()
	if putErr == nil && closeErr != nil {
		return "", closeErr
	}
	if putErr == nil {
		if info.SizeBytes() != expected.SizeBytes || info.SHA256() != expected.SHA256 {
			return "", errors.New("stored blob digest differs from staged content")
		}
		return storageKey, nil
	}
	if !errors.Is(putErr, artifact.ErrObjectExists) {
		return "", putErr
	}
	existing, existingInfo, err := service.blobs.Open(ctx, storageKey)
	if err != nil {
		return "", fmt.Errorf("verify existing content-addressed blob: %w", err)
	}
	if err := existing.Close(); err != nil {
		return "", err
	}
	if existingInfo.SizeBytes() != expected.SizeBytes ||
		existingInfo.SHA256() != expected.SHA256 {
		return "", errors.New("content-addressed blob does not match its digest")
	}
	return storageKey, nil
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

func normalizeSourceName(raw string) (string, error) {
	portable := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	name := path.Base(portable)
	if name == "" || name == "." || name == "/" {
		name = "source.zip"
	}
	if len(name) > 255 || strings.ContainsRune(name, '\x00') ||
		!strings.EqualFold(path.Ext(name), ".zip") {
		return "", fmt.Errorf("%w: source must be a ZIP archive", deployment.ErrInvalid)
	}
	return name, nil
}

// spoolSource streams the upload to a private temporary file, hashing as it
// goes, so the archive is never held in memory and never exceeds its limit.
func spoolSource(
	ctx context.Context,
	source io.Reader,
	maximumBytes int64,
) (string, artifact.CopyResult, func(), error) {
	file, err := os.CreateTemp("", "neurun-source-*.zip")
	if err != nil {
		return "", artifact.CopyResult{}, func() {}, err
	}
	name := file.Name()
	cleanup := func() { _ = os.Remove(name) }
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		cleanup()
		return "", artifact.CopyResult{}, func() {}, err
	}
	result, copyErr := artifact.CopyAndHashContext(ctx, file, source, maximumBytes)
	if copyErr == nil {
		copyErr = file.Sync()
	}
	closeErr := file.Close()
	if copyErr != nil {
		cleanup()
		return "", result, func() {}, copyErr
	}
	if closeErr != nil {
		cleanup()
		return "", result, func() {}, closeErr
	}
	return name, result, cleanup, nil
}

func validateBuiltArtifact(output builder.Output) error {
	if output.Kind != deployment.ArtifactInstallLayer &&
		output.Kind != deployment.ArtifactCodeLayer {
		return fmt.Errorf("builder returned unsupported artifact kind %q", output.Kind)
	}
	if output.Name == "" || path.Base(output.Name) != output.Name ||
		strings.ContainsAny(output.Name, "\\\x00") {
		return errors.New("builder returned an unsafe artifact name")
	}
	if output.Path == "" {
		return errors.New("builder returned an empty artifact path")
	}
	info, err := os.Lstat(output.Path)
	if err != nil {
		return fmt.Errorf("inspect built artifact: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("builder artifact must be a regular file")
	}
	mediaType, _, err := mime.ParseMediaType(output.MediaType)
	if err != nil || !strings.Contains(mediaType, "/") {
		return errors.New("builder returned an invalid artifact media type")
	}
	return nil
}

func validateLimit(limit int) error {
	if limit < 1 || limit > maximumPageSize {
		return fmt.Errorf(
			"%w: limit must be between 1 and %d", deployment.ErrInvalid, maximumPageSize,
		)
	}
	return nil
}

func boundedError(err error) string {
	if err == nil {
		return "operation failed"
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "operation failed"
	}
	if len(message) > 4_096 {
		message = message[:4_096]
	}
	return message
}

func orBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
