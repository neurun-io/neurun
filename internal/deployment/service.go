package deployment

import (
	"context"
	"encoding/json"
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
	"github.com/neurun-io/neurun/internal/ids"
)

const (
	DefaultMaxSourceBytes   = int64(32 << 20)
	DefaultMaxArtifactBytes = int64(256 << 20)
	DefaultMaxRunInputBytes = int64(1 << 20)
	DefaultBuildTimeout     = 5 * time.Minute
)

type CreateRequest struct {
	ProjectID  string
	AppID      string
	Runtime    Runtime
	EntryPoint string
	SourceName string
	Source     io.Reader
}

type CreateRunRequest struct {
	ProjectID    string
	DeploymentID string
	Input        json.RawMessage
}

type CreateExecutionRequest = CreateRunRequest

type BuildRequest struct {
	Runtime           Runtime
	EntryPoint        string
	SourceArchivePath string
	WorkDirectory     string
}

type BuiltArtifact struct {
	Kind      string
	Name      string
	MediaType string
	Path      string
}

type BuildResult struct {
	Artifacts []BuiltArtifact
}

// Builder is the runtime-neutral boundary around language toolchains.
type Builder interface {
	Build(context.Context, BuildRequest) (BuildResult, error)
}

type ServiceOptions struct {
	MaxSourceBytes   int64
	MaxArtifactBytes int64
	MaxRunInputBytes int64
	BuildTimeout     time.Duration
	Now              func() time.Time
	NewID            func(string) (string, error)
}

type Service struct {
	store            Store
	blobs            artifact.BlobStore
	builder          Builder
	maxSourceBytes   int64
	maxArtifactBytes int64
	maxRunInputBytes int64
	buildTimeout     time.Duration
	now              func() time.Time
	newID            func(string) (string, error)
	buildMu          sync.Mutex
}

func NewService(
	store Store,
	blobs artifact.BlobStore,
	builder Builder,
	options ServiceOptions,
) (*Service, error) {
	switch {
	case store == nil:
		return nil, errors.New("deployment store is required")
	case blobs == nil:
		return nil, errors.New("deployment artifact store is required")
	case builder == nil:
		return nil, errors.New("deployment builder is required")
	case options.MaxSourceBytes < 0:
		return nil, errors.New("maximum deployment source bytes cannot be negative")
	case options.MaxArtifactBytes < 0:
		return nil, errors.New("maximum deployment artifact bytes cannot be negative")
	case options.MaxRunInputBytes < 0:
		return nil, errors.New("maximum run input bytes cannot be negative")
	case options.BuildTimeout < 0:
		return nil, errors.New("deployment build timeout cannot be negative")
	}
	if options.MaxSourceBytes == 0 {
		options.MaxSourceBytes = DefaultMaxSourceBytes
	}
	if options.MaxArtifactBytes == 0 {
		options.MaxArtifactBytes = DefaultMaxArtifactBytes
	}
	if options.MaxRunInputBytes == 0 {
		options.MaxRunInputBytes = DefaultMaxRunInputBytes
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
	return &Service{
		store: store, blobs: blobs, builder: builder,
		maxSourceBytes:   options.MaxSourceBytes,
		maxArtifactBytes: options.MaxArtifactBytes,
		maxRunInputBytes: options.MaxRunInputBytes,
		buildTimeout:     options.BuildTimeout,
		now:              options.Now,
		newID:            options.NewID,
	}, nil
}

func (service *Service) Create(
	ctx context.Context,
	request CreateRequest,
) (Deployment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateIdentifier("project_id", request.ProjectID); err != nil {
		return Deployment{}, err
	}
	if err := validateIdentifier("app_id", request.AppID); err != nil {
		return Deployment{}, err
	}
	if _, err := service.store.GetApp(ctx, request.ProjectID, request.AppID); err != nil {
		return Deployment{}, err
	}
	if !request.Runtime.Valid() {
		return Deployment{}, fmt.Errorf("%w: runtime must be python", ErrInvalid)
	}
	entryPoint, err := normalizeEntryPoint(request.Runtime, request.EntryPoint)
	if err != nil {
		return Deployment{}, err
	}
	if request.Source == nil {
		return Deployment{}, fmt.Errorf("%w: source ZIP is required", ErrInvalid)
	}
	sourceName, err := normalizeSourceName(request.SourceName)
	if err != nil {
		return Deployment{}, err
	}

	sourcePath, sourceInfo, cleanup, err := spoolSource(
		ctx,
		request.Source,
		service.maxSourceBytes,
	)
	if err != nil {
		if errors.Is(err, artifact.ErrByteLimitExceeded) {
			return Deployment{}, fmt.Errorf(
				"%w: source ZIP exceeds %d bytes",
				ErrSourceTooLarge,
				service.maxSourceBytes,
			)
		}
		return Deployment{}, fmt.Errorf("stage deployment source: %w", err)
	}
	defer cleanup()

	deploymentID, err := service.allocateID("dep")
	if err != nil {
		return Deployment{}, err
	}
	sourceID, err := service.allocateID("art")
	if err != nil {
		return Deployment{}, err
	}
	storageKey, err := service.putContentAddressed(
		ctx,
		sourcePath,
		sourceInfo,
		service.maxSourceBytes,
	)
	if err != nil {
		return Deployment{}, fmt.Errorf("store deployment source: %w", err)
	}

	now := service.now().UTC().Round(0)
	record := Deployment{
		ID: deploymentID, ProjectID: request.ProjectID, AppID: request.AppID,
		Runtime: request.Runtime, EntryPoint: entryPoint,
		Status: StatusUploaded,
		Source: newArtifact(
			sourceID,
			ArtifactSource,
			sourceName,
			"application/zip",
			sourceInfo.SizeBytes,
			sourceInfo.SHA256,
			storageKey,
			now,
		),
		Builds: []Build{}, CreatedAt: now, UpdatedAt: now,
	}
	if err := service.store.SaveDeployment(ctx, record); err != nil {
		return Deployment{}, fmt.Errorf("persist uploaded deployment: %w", err)
	}
	return service.runBuild(ctx, record)
}

func (service *Service) Get(
	ctx context.Context,
	projectID string,
	deploymentID string,
) (Deployment, error) {
	return service.store.GetDeployment(ctx, projectID, deploymentID)
}

func (service *Service) List(
	ctx context.Context,
	projectID string,
	appID string,
	limit int,
) ([]Deployment, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 200", ErrInvalid)
	}
	if appID != "" {
		if err := validateIdentifier("app_id", appID); err != nil {
			return nil, err
		}
		if _, err := service.store.GetApp(ctx, projectID, appID); err != nil {
			return nil, err
		}
	}
	return service.store.ListDeployments(ctx, projectID, appID, limit)
}

func (service *Service) GetBuild(
	ctx context.Context,
	projectID string,
	buildID string,
) (Build, error) {
	return service.store.GetBuild(ctx, projectID, buildID)
}

func (service *Service) ListBuilds(
	ctx context.Context,
	projectID string,
	deploymentID string,
	limit int,
) ([]Build, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 200", ErrInvalid)
	}
	if deploymentID != "" {
		if _, err := service.store.GetDeployment(ctx, projectID, deploymentID); err != nil {
			return nil, err
		}
	}
	return service.store.ListBuilds(ctx, projectID, deploymentID, limit)
}

// RecoverInterruptedBuilds marks builds left in the building state by a prior
// process crash as failed. It never retries build side effects implicitly.
func (service *Service) RecoverInterruptedBuilds(ctx context.Context) (int, error) {
	return service.store.RecoverBuildingDeployments(
		ctx,
		service.now().UTC().Round(0),
		Failure{
			Code:    "build_interrupted",
			Message: "build was interrupted by a service restart",
		},
	)
}

func (service *Service) CreateRun(
	ctx context.Context,
	request CreateRunRequest,
) (Run, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateIdentifier("project_id", request.ProjectID); err != nil {
		return Run{}, err
	}
	if err := validateIdentifier("deployment_id", request.DeploymentID); err != nil {
		return Run{}, err
	}
	input, err := NormalizeJSON(request.Input, service.maxRunInputBytes)
	if err != nil {
		return Run{}, err
	}
	record, err := service.store.GetDeployment(
		ctx,
		request.ProjectID,
		request.DeploymentID,
	)
	if err != nil {
		return Run{}, err
	}
	build, ok := record.ReadyBuild()
	if !ok {
		return Run{}, fmt.Errorf("%w: %s", ErrNoReadyBuild, record.ID)
	}
	runID, err := service.allocateID("exe")
	if err != nil {
		return Run{}, err
	}
	run := Run{
		ID: runID, ProjectID: request.ProjectID,
		DeploymentID: record.ID, BuildID: build.ID,
		Status: RunQueued, Input: input,
		CreatedAt: service.now().UTC().Round(0),
	}
	if err := service.store.CreateRun(ctx, run); err != nil {
		return Run{}, fmt.Errorf("persist queued deployment execution: %w", err)
	}
	return cloneRun(run), nil
}

func (service *Service) GetRun(
	ctx context.Context,
	projectID string,
	runID string,
) (Run, error) {
	return service.store.GetRun(ctx, projectID, runID)
}

func (service *Service) ListRuns(
	ctx context.Context,
	projectID string,
	deploymentID string,
	limit int,
) ([]Run, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("%w: limit must be between 1 and 200", ErrInvalid)
	}
	// Confirm project ownership when filtering by deployment. An empty filter
	// returns executions across the principal-owned project.
	if deploymentID != "" {
		if _, err := service.store.GetDeployment(ctx, projectID, deploymentID); err != nil {
			return nil, err
		}
	}
	return service.store.ListRuns(ctx, projectID, deploymentID, limit)
}

func (service *Service) Rerun(
	ctx context.Context,
	projectID string,
	runID string,
) (Run, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	original, err := service.store.GetRun(ctx, projectID, runID)
	if err != nil {
		return Run{}, err
	}
	if !original.Status.Terminal() {
		return Run{}, fmt.Errorf("%w: only a finished execution can be rerun", ErrInvalid)
	}
	record, err := service.store.GetDeployment(
		ctx,
		projectID,
		original.DeploymentID,
	)
	if err != nil {
		return Run{}, err
	}
	build, exists := record.BuildByID(original.BuildID)
	if !exists || build.Status != StatusReady {
		return Run{}, fmt.Errorf("%w: pinned build %s", ErrNoReadyBuild, original.BuildID)
	}
	newID, err := service.allocateID("exe")
	if err != nil {
		return Run{}, err
	}
	rerun := Run{
		ID: newID, ProjectID: original.ProjectID,
		DeploymentID: original.DeploymentID, BuildID: original.BuildID,
		Status:       RunQueued,
		Input:        append(json.RawMessage(nil), original.Input...),
		CreatedAt:    service.now().UTC().Round(0),
		RerunOfRunID: original.ID,
	}
	if err := service.store.CreateRun(ctx, rerun); err != nil {
		return Run{}, fmt.Errorf("persist queued rerun: %w", err)
	}
	return cloneRun(rerun), nil
}

func (service *Service) CreateExecution(
	ctx context.Context,
	request CreateExecutionRequest,
) (Execution, error) {
	return service.CreateRun(ctx, request)
}

func (service *Service) GetExecution(
	ctx context.Context,
	projectID string,
	executionID string,
) (Execution, error) {
	return service.GetRun(ctx, projectID, executionID)
}

func (service *Service) ListExecutions(
	ctx context.Context,
	projectID string,
	deploymentID string,
	limit int,
) ([]Execution, error) {
	return service.ListRuns(ctx, projectID, deploymentID, limit)
}

func (service *Service) ListDeploymentExecutions(
	ctx context.Context,
	projectID string,
	deploymentID string,
	limit int,
) ([]Execution, error) {
	if deploymentID == "" {
		return nil, fmt.Errorf("%w: deployment_id is required", ErrInvalid)
	}
	return service.ListExecutions(ctx, projectID, deploymentID, limit)
}

func (service *Service) RerunExecution(
	ctx context.Context,
	projectID string,
	executionID string,
) (Execution, error) {
	return service.Rerun(ctx, projectID, executionID)
}

func (service *Service) runBuild(
	ctx context.Context,
	record Deployment,
) (Deployment, error) {
	service.buildMu.Lock()
	defer service.buildMu.Unlock()

	current, err := service.store.GetDeployment(ctx, record.ProjectID, record.ID)
	if err == nil {
		record = current
	} else if !errors.Is(err, ErrNotFound) {
		return Deployment{}, err
	}
	buildID, err := service.allocateID("bld")
	if err != nil {
		return Deployment{}, err
	}
	startedAt := service.now().UTC().Round(0)
	record.Builds = append(record.Builds, Build{
		ID: buildID, ProjectID: record.ProjectID, DeploymentID: record.ID,
		Number: len(record.Builds) + 1,
		Status: StatusBuilding, Runtime: record.Runtime,
		EntryPoint: record.EntryPoint, SourceSHA256: record.Source.SHA256,
		Artifacts: []Artifact{}, StartedAt: startedAt,
	})
	record.Status = StatusBuilding
	record.UpdatedAt = startedAt
	if err := service.store.SaveDeployment(ctx, record); err != nil {
		return Deployment{}, fmt.Errorf("persist deployment build start: %w", err)
	}

	buildCtx, cancel := context.WithTimeout(ctx, service.buildTimeout)
	defer cancel()
	workDirectory, err := os.MkdirTemp("", "neurun-build-*")
	if err != nil {
		return service.failBuild(ctx, record, buildID, "build_environment", err)
	}
	defer os.RemoveAll(workDirectory)
	sourcePath := filepath.Join(workDirectory, "source.zip")
	if err := service.materializeArtifact(buildCtx, record.Source, sourcePath); err != nil {
		return service.failBuild(ctx, record, buildID, "source_unavailable", err)
	}
	result, buildErr := service.builder.Build(buildCtx, BuildRequest{
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
		return service.failBuild(
			ctx,
			record,
			buildID,
			"build_failed",
			errors.New("builder produced no artifacts"),
		)
	}
	stored, err := service.storeBuildArtifacts(
		buildCtx,
		record,
		result.Artifacts,
	)
	if err != nil {
		return service.failBuild(ctx, record, buildID, "artifact_store_failed", err)
	}
	finishedAt := service.now().UTC().Round(0)
	index := buildIndex(record.Builds, buildID)
	if index < 0 {
		return Deployment{}, errors.New("active build disappeared from deployment")
	}
	record.Builds[index].Status = StatusReady
	record.Builds[index].Artifacts = stored
	record.Builds[index].FinishedAt = &finishedAt
	record.Status = StatusReady
	record.UpdatedAt = finishedAt
	if err := service.store.SaveDeployment(ctx, record); err != nil {
		return Deployment{}, fmt.Errorf("persist completed deployment build: %w", err)
	}
	return cloneDeployment(record), nil
}

func (service *Service) materializeArtifact(
	ctx context.Context,
	stored Artifact,
	targetPath string,
) error {
	reader, info, err := service.blobs.Open(ctx, stored.storageKey)
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

func (service *Service) storeBuildArtifacts(
	ctx context.Context,
	record Deployment,
	outputs []BuiltArtifact,
) ([]Artifact, error) {
	remaining := service.maxArtifactBytes
	seenKinds := make(map[string]struct{}, len(outputs))
	stored := make([]Artifact, 0, len(outputs))
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
		storageKey, err := service.putContentAddressed(
			ctx,
			output.Path,
			info,
			remaining,
		)
		if err != nil {
			return nil, err
		}
		artifactID, err := service.allocateID("art")
		if err != nil {
			return nil, err
		}
		remaining -= info.SizeBytes
		stored = append(stored, newArtifact(
			artifactID,
			output.Kind,
			output.Name,
			output.MediaType,
			info.SizeBytes,
			info.SHA256,
			storageKey,
			service.now().UTC().Round(0),
		))
	}
	if _, exists := seenKinds[ArtifactCodeLayer]; !exists {
		return nil, errors.New("builder did not produce a code layer")
	}
	return stored, nil
}

func (service *Service) putContentAddressed(
	ctx context.Context,
	filePath string,
	expected artifact.CopyResult,
	maximumBytes int64,
) (string, error) {
	if len(expected.SHA256) != 64 || expected.SizeBytes < 0 {
		return "", errors.New("content digest metadata is invalid")
	}
	storageKey := path.Join(
		"objects",
		"sha256",
		expected.SHA256[:2],
		expected.SHA256,
	)
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
	closeErr = existing.Close()
	if closeErr != nil {
		return "", closeErr
	}
	if existingInfo.SizeBytes() != expected.SizeBytes ||
		existingInfo.SHA256() != expected.SHA256 {
		return "", errors.New("content-addressed blob does not match its digest")
	}
	return storageKey, nil
}

func (service *Service) failBuild(
	ctx context.Context,
	record Deployment,
	buildID string,
	code string,
	cause error,
) (Deployment, error) {
	finishedAt := service.now().UTC().Round(0)
	index := buildIndex(record.Builds, buildID)
	if index < 0 {
		return Deployment{}, errors.New("active build disappeared from deployment")
	}
	record.Builds[index].Status = StatusFailed
	record.Builds[index].Failure = &Failure{
		Code: code, Message: boundedError(cause),
	}
	record.Builds[index].FinishedAt = &finishedAt
	record.Status = StatusFailed
	record.UpdatedAt = finishedAt
	saveCtx := ctx
	if saveCtx == nil || saveCtx.Err() != nil {
		saveCtx = context.Background()
	}
	if err := service.store.SaveDeployment(saveCtx, record); err != nil {
		return cloneDeployment(record), fmt.Errorf(
			"persist failed deployment build: %w",
			err,
		)
	}
	return cloneDeployment(record), nil
}

func (service *Service) allocateID(prefix string) (string, error) {
	value, err := service.newID(prefix)
	if err != nil {
		return "", fmt.Errorf("allocate %s ID: %w", prefix, err)
	}
	if err := validateIdentifier(prefix+"_id", value); err != nil {
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
		return "", fmt.Errorf("%w: source must be a ZIP archive", ErrInvalid)
	}
	return name, nil
}

func spoolSource(
	ctx context.Context,
	source io.Reader,
	maximumBytes int64,
) (string, artifact.CopyResult, func(), error) {
	file, err := os.CreateTemp("", "neurun-source-*.zip")
	if err != nil {
		return "", artifact.CopyResult{}, func() {}, err
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
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
	return path, result, cleanup, nil
}

func validateBuiltArtifact(output BuiltArtifact) error {
	if output.Kind != ArtifactInstallLayer && output.Kind != ArtifactCodeLayer {
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

func buildIndex(builds []Build, buildID string) int {
	for index := range builds {
		if builds[index].ID == buildID {
			return index
		}
	}
	return -1
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
