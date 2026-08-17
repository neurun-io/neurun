package builder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/domain/deployment"
	"github.com/neurun-io/neurun/internal/files"
	"github.com/neurun-io/neurun/internal/ids"
	"github.com/neurun-io/neurun/internal/repository/database"
	"github.com/neurun-io/neurun/internal/repository/file"
)

// Source hands a deployment the commit it names. It is an interface so the
// deployer never learns where source comes from, the way the worker never
// learns how execution tokens are stored.
type Source interface {
	Fetch(ctx context.Context, appID, commitSHA, targetPath string) error
}

type DeployerOptions struct {
	PollInterval time.Duration
	BuildTimeout time.Duration
	// CacheDirectory outlives a build and holds the toolchain caches. Empty
	// leaves every build cold.
	CacheDirectory          string
	MaxArtifactBytes        int64
	MaxArchiveEntries       int
	MaxArchiveExpandedBytes int64
	Now                     func() time.Time
	NewID                   func(string) (string, error)
}

// Deployer turns queued deployments into builds. It claims them from the
// database rather than being handed them, so a deployment survives the process
// that accepted it and any builder can pick it up.
type Deployer struct {
	deployments *database.DeploymentRepository
	builds      *database.BuildRepository
	blobs       file.Repository
	source      Source
	toolchains  map[build.Runtime]Builder
	options     DeployerOptions
}

func NewDeployer(
	deployments *database.DeploymentRepository,
	builds *database.BuildRepository,
	blobs file.Repository,
	source Source,
	toolchains map[build.Runtime]Builder,
	options DeployerOptions,
) (*Deployer, error) {
	switch {
	case deployments == nil || builds == nil:
		return nil, errors.New("builder: deployer requires its repositories")
	case blobs == nil:
		return nil, errors.New("builder: artifact store is required")
	case source == nil:
		return nil, errors.New("builder: deployer requires a source")
	case len(toolchains) == 0:
		return nil, errors.New("builder: at least one toolchain is required")
	case options.PollInterval < 0 || options.BuildTimeout < 0 ||
		options.MaxArtifactBytes < 0 || options.MaxArchiveEntries < 0 ||
		options.MaxArchiveExpandedBytes < 0:
		return nil, errors.New("builder: options cannot be negative")
	}
	if options.PollInterval == 0 {
		options.PollInterval = 250 * time.Millisecond
	}
	if options.BuildTimeout == 0 {
		options.BuildTimeout = 5 * time.Minute
	}
	if options.MaxArtifactBytes == 0 {
		options.MaxArtifactBytes = 268_435_456
	}
	if options.MaxArchiveEntries == 0 {
		options.MaxArchiveEntries = files.DefaultMaxArchiveEntries
	}
	if options.MaxArchiveExpandedBytes == 0 {
		options.MaxArchiveExpandedBytes = files.DefaultMaxArchiveExpandedBytes
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.NewID == nil {
		options.NewID = ids.New
	}
	return &Deployer{
		deployments: deployments, builds: builds, blobs: blobs,
		source: source, toolchains: toolchains, options: options,
	}, nil
}

// Recover fails deployments a previous process left building. It never retries
// them: whatever the first attempt did to a toolchain cache already happened.
func (deployer *Deployer) Recover(ctx context.Context) (int, error) {
	return deployer.deployments.RecoverBuilding(
		ctx,
		deployer.options.Now().UTC(),
		deployment.Failure{
			Code:    "build_interrupted",
			Message: "build was interrupted by a service restart",
		},
	)
}

func (deployer *Deployer) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(deployer.options.PollInterval)
	defer ticker.Stop()
	for {
		err := deployer.ProcessOne(ctx)
		if err != nil && !errors.Is(err, deployment.ErrNoQueued) &&
			!errors.Is(err, context.Canceled) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (deployer *Deployer) ProcessOne(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	record, err := deployer.deployments.ClaimQueued(
		ctx, deployer.options.Now().UTC(),
	)
	if err != nil {
		return err
	}
	produced, failure := deployer.build(ctx, &record)
	// A cancelled context must not strand the deployment in building: the row
	// has to reach a terminal state even while the process is shutting down.
	finishCtx := ctx
	if finishCtx.Err() != nil {
		var cancel context.CancelFunc
		finishCtx, cancel = context.WithTimeout(
			context.WithoutCancel(ctx), 5*time.Second,
		)
		defer cancel()
	}
	finished := deployer.options.Now().UTC().Round(0)
	if failure != nil {
		if err := record.Fail(*failure, finished); err != nil {
			return fmt.Errorf("builder: fail deployment %s: %w", record.ID, err)
		}
	} else if err := record.MarkReady(produced, finished); err != nil {
		return fmt.Errorf("builder: finish deployment %s: %w", record.ID, err)
	}
	if err := deployer.deployments.Save(finishCtx, record); err != nil {
		return fmt.Errorf("builder: persist deployment %s: %w", record.ID, err)
	}
	return nil
}

// build runs the toolchain over the commit, and either returns the sealed build
// or the reason there is none. The record is written as it goes, so somebody
// watching sees the stage it is in and the output as it arrives.
func (deployer *Deployer) build(
	ctx context.Context,
	record *deployment.Deployment,
) (build.Build, *deployment.Failure) {
	buildCtx, cancel := context.WithTimeout(ctx, deployer.options.BuildTimeout)
	defer cancel()

	work, err := os.MkdirTemp("", "neurun-build-*")
	if err != nil {
		return build.Build{}, newFailure("build_environment", err)
	}
	defer os.RemoveAll(work)

	toolchain, known := deployer.toolchains[record.Runtime]
	if !known {
		return build.Build{}, newFailure("runtime_unsupported",
			fmt.Errorf("no toolchain for runtime %q", record.Runtime))
	}
	archive := filepath.Join(work, "source.zip")
	if err := deployer.source.Fetch(
		buildCtx, record.AppID, record.CommitSHA, archive,
	); err != nil {
		return build.Build{}, newFailure("source_unavailable", err)
	}
	fetched, err := files.HashFile(archive, deployer.options.MaxArchiveExpandedBytes)
	if err != nil {
		return build.Build{}, newFailure("source_unavailable", err)
	}
	sourceDir := filepath.Join(work, "source")
	if _, err := files.ExtractZIPFile(archive, sourceDir, files.ArchiveLimits{
		MaxEntries:       deployer.options.MaxArchiveEntries,
		MaxExpandedBytes: deployer.options.MaxArchiveExpandedBytes,
	}); err != nil {
		return build.Build{}, newFailure("source_unavailable", err)
	}
	cacheDirectory, err := deployer.cacheFor(*record)
	if err != nil {
		return build.Build{}, newFailure("build_environment", err)
	}

	stream := deployer.follow(ctx, record)
	result, buildErr := toolchain.Build(buildCtx, Request{
		SourceDirectory: sourceDir, WorkDirectory: work,
		CacheDirectory: cacheDirectory, Logs: stream,
	})
	stream.Close()
	if buildErr != nil {
		code := "build_failed"
		if errors.Is(buildErr, context.DeadlineExceeded) ||
			errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			code = "build_timeout"
		}
		return build.Build{}, newFailure(code, buildErr)
	}
	if len(result.Layers) == 0 {
		return build.Build{}, newFailure(
			"build_failed", errors.New("toolchain produced no layers"),
		)
	}
	if err := deployer.advance(ctx, record); err != nil {
		return build.Build{}, newFailure("build_environment", err)
	}

	buildID, err := deployer.allocateID("bld")
	if err != nil {
		return build.Build{}, newFailure("artifact_store_failed", err)
	}
	remaining := deployer.options.MaxArtifactBytes
	artifacts := make([]build.Artifact, 0, len(result.Layers))
	for _, layer := range result.Layers {
		stored, err := deployer.store(buildCtx, buildID, layer, remaining)
		if err != nil {
			return build.Build{}, newFailure("artifact_store_failed", err)
		}
		remaining -= stored.SizeBytes
		artifacts = append(artifacts, stored)
	}
	produced, err := build.New(
		buildID, record.AppID, record.ID, record.Runtime,
		fetched.SHA256, artifacts, deployer.options.Now().UTC().Round(0),
	)
	if err != nil {
		return build.Build{}, newFailure("artifact_store_failed", err)
	}
	// The build is written first: the deployment points at it, so the row it
	// references has to exist before the reference does.
	if err := deployer.builds.Save(ctx, produced); err != nil {
		return build.Build{}, newFailure("artifact_store_failed", err)
	}
	return produced, nil
}

// cacheFor is the build environment, kept so the next deployment of this app
// compiles what changed rather than everything. It is per app and per runtime:
// two apps sharing a target directory would fingerprint against each other's
// intermediates, and one runtime's cache stays safe to delete on its own.
//
// Absolute, because a toolchain runs with the source as its working directory
// and reads this out of the environment: a relative path would put the cache
// inside whatever it happens to be compiling.
func (deployer *Deployer) cacheFor(record deployment.Deployment) (string, error) {
	if deployer.options.CacheDirectory == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(filepath.Join(
		deployer.options.CacheDirectory, string(record.Runtime), record.AppID,
	))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return "", err
	}
	return absolute, nil
}

// advance moves the deployment on to its next stage and records it there, so
// somebody watching sees which wait they are in.
func (deployer *Deployer) advance(
	ctx context.Context,
	record *deployment.Deployment,
) error {
	if err := record.Advance(deployer.options.Now().UTC().Round(0)); err != nil {
		return err
	}
	if err := deployer.deployments.Save(ctx, *record); err != nil {
		return fmt.Errorf("persist deployment stage: %w", err)
	}
	return nil
}

// store puts one built layer under the build that made it, and returns the
// handle. The store hashes what it writes, so the digest describes what
// actually landed rather than what was staged.
func (deployer *Deployer) store(
	ctx context.Context,
	buildID string,
	layer Layer,
	maximumBytes int64,
) (build.Artifact, error) {
	info, err := os.Lstat(layer.Path)
	if err != nil {
		return build.Artifact{}, fmt.Errorf("inspect built artifact: %w", err)
	}
	if !info.Mode().IsRegular() {
		return build.Artifact{}, errors.New("built artifact must be a regular file")
	}
	artifactID, err := deployer.allocateID("art")
	if err != nil {
		return build.Artifact{}, err
	}
	source, err := os.Open(layer.Path)
	if err != nil {
		return build.Artifact{}, err
	}
	defer source.Close()

	storageKey := build.StorageKeyFor(buildID, artifactID)
	stored, err := deployer.blobs.Put(ctx, storageKey, source, maximumBytes)
	if err != nil {
		return build.Artifact{}, err
	}
	return build.Artifact{
		ID: artifactID, Name: layer.Name,
		SizeBytes: stored.SizeBytes(), SHA256: stored.SHA256(),
		StorageKey: storageKey,
		CreatedAt:  deployer.options.Now().UTC().Round(0),
	}, nil
}

func (deployer *Deployer) allocateID(prefix string) (string, error) {
	value, err := deployer.options.NewID(prefix)
	if err != nil {
		return "", fmt.Errorf("allocate %s ID: %w", prefix, err)
	}
	if err := deployment.ValidateIdentifier(prefix+"_id", value); err != nil {
		return "", err
	}
	return value, nil
}

func newFailure(code string, cause error) *deployment.Failure {
	message := "operation failed"
	if cause != nil && strings.TrimSpace(cause.Error()) != "" {
		message = strings.TrimSpace(cause.Error())
		if len(message) > 4_096 {
			message = message[:4_096]
		}
	}
	return &deployment.Failure{Code: code, Message: message}
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

func (deployer *Deployer) follow(
	ctx context.Context,
	record *deployment.Deployment,
) *logStream {
	stream := &logStream{
		record: record,
		persist: func(snapshot deployment.Deployment) {
			if err := deployer.deployments.Save(ctx, snapshot); err != nil {
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
