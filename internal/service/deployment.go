// Package service holds the application logic: it drives domain records
// through their transitions and hands them to repositories to persist.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/neurun-io/neurun/internal/builder"
	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/domain/deployment"
	"github.com/neurun-io/neurun/internal/dto"
	"github.com/neurun-io/neurun/internal/files"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository/database"
	"github.com/neurun-io/neurun/internal/repository/file"
)

const (
	DefaultMaxSourceBytes   = int64(33_554_432)
	DefaultMaxArtifactBytes = int64(268_435_456)
	DefaultBuildTimeout     = 5 * time.Minute
	maximumPageSize         = 200
)

type DeploymentOptions struct {
	// BuildCacheDirectory outlives a build and holds the toolchain caches. Empty
	// leaves every build cold.
	BuildCacheDirectory string
	MaxSourceBytes      int64
	MaxArtifactBytes    int64
	BuildTimeout        time.Duration
	Now                 func() time.Time
	NewID               func(string) (string, error)
}

// DeploymentService turns a source archive into a build. It reaches the app
// repository only to learn which project a deployment belongs to; projects and
// apps are owned elsewhere.
type DeploymentService struct {
	apps        *database.AppRepository
	deployments *database.DeploymentRepository
	builds      *database.BuildRepository
	blobs       file.Repository
	builders    map[build.Runtime]builder.Builder

	maxSourceBytes   int64
	maxArtifactBytes int64
	buildTimeout     time.Duration
	buildCache       string
	now              func() time.Time
	newID            func(string) (string, error)
	buildMu          sync.Mutex
}

func NewDeploymentService(
	apps *database.AppRepository,
	deployments *database.DeploymentRepository,
	builds *database.BuildRepository,
	blobs file.Repository,
	toolchains map[build.Runtime]builder.Builder,
	options DeploymentOptions,
) (*DeploymentService, error) {
	switch {
	case apps == nil || deployments == nil || builds == nil:
		return nil, errors.New("deployment service requires its repositories")
	case blobs == nil:
		return nil, errors.New("deployment service requires an artifact store")
	case len(toolchains) == 0:
		return nil, errors.New("deployment service requires at least one builder")
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
		apps: apps, deployments: deployments, builds: builds,
		blobs: blobs, builders: toolchains,
		maxSourceBytes:   options.MaxSourceBytes,
		maxArtifactBytes: options.MaxArtifactBytes,
		buildTimeout:     options.BuildTimeout,
		buildCache:       options.BuildCacheDirectory,
		now:              options.Now,
		newID:            options.NewID,
	}, nil
}

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
			"%w: runtime must be python, rust, go or ruby", deployment.ErrInvalid,
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
		if errors.Is(err, files.ErrByteLimitExceeded) {
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
		build.Artifact{
			ID: sourceID, Kind: build.ArtifactSource, Name: sourceName,
			MediaType: "application/zip", SizeBytes: sourceInfo.SizeBytes,
			SHA256: sourceInfo.SHA256, StorageKey: storageKey, CreatedAt: now,
		},
		now,
	)
	if err != nil {
		return deployment.Deployment{}, err
	}
	record.FromGit(request.CommitSHA, request.GitRef)
	if err := service.deployments.Save(ctx, record); err != nil {
		return deployment.Deployment{}, fmt.Errorf("persist queued deployment: %w", err)
	}
	service.schedule(record)
	return deployment.CloneDeployment(record), nil
}

// schedule builds on a context of its own, so cancelling the caller cannot
// abandon a deployment mid-toolchain.
func (service *DeploymentService) schedule(record deployment.Deployment) {
	go func() {
		if _, err := service.runBuild(context.Background(), record); err != nil {
			slog.Error(
				"deployment build failed",
				"deployment", record.ID, "app", record.AppID, "error", err,
			)
		}
	}()
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

// runBuild is serialized process-wide: a build spends minutes in a toolchain,
// and two concurrent ones would race on the same deployment row.
func (service *DeploymentService) runBuild(
	ctx context.Context,
	record deployment.Deployment,
) (deployment.Deployment, error) {
	service.buildMu.Lock()
	defer service.buildMu.Unlock()

	// Unscoped on purpose: this re-reads a row the caller already reached
	// through an organization-scoped path, to pick up a concurrent write.
	current, err := service.deployments.GetByIDUnscoped(ctx, record.ID)
	if err == nil {
		record = current
	} else if !errors.Is(err, deployment.ErrNotFound) {
		return deployment.Deployment{}, err
	}
	buildCtx, cancel := context.WithTimeout(ctx, service.buildTimeout)
	defer cancel()
	workDirectory, err := os.MkdirTemp("", "neurun-build-*")
	if err != nil {
		return service.fail(ctx, record, "build_environment", err)
	}
	defer os.RemoveAll(workDirectory)

	sourcePath := filepath.Join(workDirectory, "source.zip")
	if err := service.materialize(buildCtx, record.Source, sourcePath); err != nil {
		return service.fail(ctx, record, "source_unavailable", err)
	}
	toolchain, ok := service.builders[record.Runtime]
	if !ok {
		return service.fail(ctx, record, "runtime_unsupported",
			fmt.Errorf("no builder for runtime %q", record.Runtime))
	}
	// Cached per runtime: a Go build cache and a cargo target directory have
	// nothing to say to each other, and keeping them apart makes one runtime's
	// cache safe to delete on its own.
	cacheDirectory := ""
	if service.buildCache != "" {
		cacheDirectory = filepath.Join(service.buildCache, string(record.Runtime))
		if err := os.MkdirAll(cacheDirectory, 0o700); err != nil {
			return service.fail(ctx, record, "build_environment", err)
		}
	}

	// Nothing above this point has run a toolchain, so nothing above it can
	// leave a build behind: those failures are the deployment's alone.
	if err := service.advance(ctx, &record); err != nil {
		return deployment.Deployment{}, err
	}
	stream := service.follow(ctx, &record)
	result, buildErr := toolchain.Build(buildCtx, builder.Request{
		Runtime: record.Runtime, EntryPoint: record.EntryPoint,
		SourceArchivePath: sourcePath, WorkDirectory: workDirectory,
		CacheDirectory: cacheDirectory, Logs: stream,
	})
	stream.Close()
	if buildErr != nil {
		code := "build_failed"
		if errors.Is(buildErr, context.DeadlineExceeded) ||
			errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			code = "build_timeout"
		}
		return service.fail(ctx, record, code, buildErr)
	}
	if len(result.Artifacts) == 0 {
		return service.fail(ctx, record, "build_failed",
			errors.New("builder produced no artifacts"))
	}
	if err := service.advance(ctx, &record); err != nil {
		return deployment.Deployment{}, err
	}
	stored, err := service.storeBuildArtifacts(buildCtx, result.Artifacts)
	if err != nil {
		return service.fail(ctx, record, "artifact_store_failed", err)
	}
	buildID, err := service.allocateID("bld")
	if err != nil {
		return deployment.Deployment{}, err
	}
	produced, err := build.New(
		buildID, record.Runtime, record.EntryPoint,
		record.Source.SHA256, stored, service.now().UTC().Round(0),
	)
	if err != nil {
		return deployment.Deployment{}, err
	}
	// The build is written first: the deployment points at it, so the row it
	// references has to exist before the reference does.
	if err := service.builds.Save(ctx, produced); err != nil {
		return deployment.Deployment{}, fmt.Errorf("persist build: %w", err)
	}
	if err := record.MarkReady(produced, service.now().UTC().Round(0)); err != nil {
		return deployment.Deployment{}, err
	}
	if err := service.deployments.Save(ctx, record); err != nil {
		return deployment.Deployment{}, fmt.Errorf("persist completed deployment: %w", err)
	}
	return deployment.CloneDeployment(record), nil
}

// logFlush is how far behind the toolchain a reader is allowed to be. A build
// prints in bursts, and writing the row on every burst would write it hundreds
// of times for output nobody reads that fast.
const logFlush = 2 * time.Second

// logStream carries toolchain output onto the deployment while the build is
// still running, so following a deployment shows it happening rather than
// showing nothing and then everything.
type logStream struct {
	mutex   sync.Mutex
	record  *deployment.Deployment
	written bool
	persist func(deployment.Deployment)
	stop    chan struct{}
	stopped chan struct{}
}

func (service *DeploymentService) follow(
	ctx context.Context,
	record *deployment.Deployment,
) *logStream {
	stream := &logStream{
		record: record,
		persist: func(snapshot deployment.Deployment) {
			if err := service.deployments.Save(ctx, snapshot); err != nil {
				slog.Warn(
					"persist deployment logs",
					"deployment", snapshot.ID, "error", err,
				)
			}
		},
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	go stream.flushEvery(logFlush)
	return stream
}

func (stream *logStream) Write(output []byte) (int, error) {
	stream.mutex.Lock()
	defer stream.mutex.Unlock()
	stream.record.Log(string(output))
	stream.written = true
	return len(output), nil
}

func (stream *logStream) flushEvery(interval time.Duration) {
	defer close(stream.stopped)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stream.stop:
			stream.flush()
			return
		case <-ticker.C:
			stream.flush()
		}
	}
}

func (stream *logStream) flush() {
	stream.mutex.Lock()
	if !stream.written {
		stream.mutex.Unlock()
		return
	}
	stream.written = false
	snapshot := deployment.CloneDeployment(*stream.record)
	stream.mutex.Unlock()
	stream.persist(snapshot)
}

// Close stops following and leaves the last of the output on the row.
func (stream *logStream) Close() {
	close(stream.stop)
	<-stream.stopped
}

// advance moves the deployment on to its next stage and records it there, so
// somebody watching sees which wait they are in.
func (service *DeploymentService) advance(
	ctx context.Context,
	record *deployment.Deployment,
) error {
	if err := record.Advance(service.now().UTC().Round(0)); err != nil {
		return err
	}
	if err := service.deployments.Save(ctx, *record); err != nil {
		return fmt.Errorf("persist deployment stage: %w", err)
	}
	return nil
}

func (service *DeploymentService) fail(
	ctx context.Context,
	record deployment.Deployment,
	code string,
	cause error,
) (deployment.Deployment, error) {
	if err := record.Fail(
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
			"persist failed deployment: %w", err,
		)
	}
	return deployment.CloneDeployment(record), nil
}

func (service *DeploymentService) materialize(
	ctx context.Context,
	stored build.Artifact,
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
	result, copyErr := files.CopyAndHashContext(ctx, target, reader, stored.SizeBytes)
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
) ([]build.Artifact, error) {
	remaining := service.maxArtifactBytes
	seenKinds := make(map[string]struct{}, len(outputs))
	stored := make([]build.Artifact, 0, len(outputs))
	for _, output := range outputs {
		if err := validateBuiltArtifact(output); err != nil {
			return nil, err
		}
		if _, duplicate := seenKinds[output.Kind]; duplicate {
			return nil, fmt.Errorf("builder returned duplicate %s artifact", output.Kind)
		}
		seenKinds[output.Kind] = struct{}{}
		info, err := files.HashFile(output.Path, remaining)
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
		stored = append(stored, build.Artifact{
			ID: artifactID, Kind: output.Kind, Name: output.Name,
			MediaType: output.MediaType, SizeBytes: info.SizeBytes,
			SHA256: info.SHA256, StorageKey: storageKey,
			CreatedAt: service.now().UTC().Round(0),
		})
	}
	if _, exists := seenKinds[build.ArtifactCodeLayer]; !exists {
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
	expected files.CopyResult,
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
	if !errors.Is(putErr, file.ErrExists) {
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
) (string, files.CopyResult, func(), error) {
	file, err := os.CreateTemp("", "neurun-source-*.zip")
	if err != nil {
		return "", files.CopyResult{}, func() {}, err
	}
	name := file.Name()
	cleanup := func() { _ = os.Remove(name) }
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		cleanup()
		return "", files.CopyResult{}, func() {}, err
	}
	result, copyErr := files.CopyAndHashContext(ctx, file, source, maximumBytes)
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
	if output.Kind != build.ArtifactInstallLayer &&
		output.Kind != build.ArtifactCodeLayer {
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
