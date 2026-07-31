// Package worker executes durable deployment runs pinned to immutable builds.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neurun-io/neurun/internal/artifact"
	"github.com/neurun-io/neurun/internal/deployment"
)

type ExecuteRequest struct {
	CodeDirectory    string
	InstallDirectory string
	Entrypoint       string
	Input            json.RawMessage
	MaxResultBytes   int64
	MaxLogBytes      int64
}
type ExecuteResult struct {
	Output json.RawMessage
	Logs   string
}

type Runner interface {
	Execute(context.Context, ExecuteRequest) (ExecuteResult, error)
}

type Options struct {
	PollInterval            time.Duration
	RunTimeout              time.Duration
	MaxResultBytes          int64
	MaxLogBytes             int64
	MaxArtifactBytes        int64
	MaxArchiveEntries       int
	MaxArchiveExpandedBytes int64
	Now                     func() time.Time
}

type Worker struct {
	store   deployment.Store
	blobs   artifact.BlobStore
	runner  Runner
	options Options
}

func New(store deployment.Store, blobs artifact.BlobStore, runner Runner, options Options) (*Worker, error) {
	switch {
	case store == nil:
		return nil, errors.New("worker: deployment store is required")
	case blobs == nil:
		return nil, errors.New("worker: artifact store is required")
	case runner == nil:
		return nil, errors.New("worker: runner is required")
	}
	if options.PollInterval < 0 || options.RunTimeout < 0 || options.MaxResultBytes < 0 || options.MaxLogBytes < 0 || options.MaxArtifactBytes < 0 || options.MaxArchiveEntries < 0 || options.MaxArchiveExpandedBytes < 0 {
		return nil, errors.New("worker: options cannot be negative")
	}
	if options.PollInterval == 0 {
		options.PollInterval = 250 * time.Millisecond
	}
	if options.RunTimeout == 0 {
		options.RunTimeout = 5 * time.Minute
	}
	if options.MaxResultBytes == 0 {
		options.MaxResultBytes = 4 << 20
	}
	if options.MaxLogBytes == 0 {
		options.MaxLogBytes = 256 << 10
	}
	if options.MaxArtifactBytes == 0 {
		options.MaxArtifactBytes = 256 << 20
	}
	if options.MaxArchiveEntries == 0 {
		options.MaxArchiveEntries = artifact.DefaultMaxArchiveEntries
	}
	if options.MaxArchiveExpandedBytes == 0 {
		options.MaxArchiveExpandedBytes = artifact.DefaultMaxArchiveExpandedBytes
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Worker{store: store, blobs: blobs, runner: runner, options: options}, nil
}

func (worker *Worker) Recover(ctx context.Context) (int, error) {
	return worker.store.RecoverRunningRuns(ctx, worker.options.Now().UTC(), deployment.Failure{Code: "worker_restarted", Message: "worker stopped before the run completed"})
}

func (worker *Worker) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(worker.options.PollInterval)
	defer ticker.Stop()
	for {
		err := worker.ProcessOne(ctx)
		if err != nil && !errors.Is(err, deployment.ErrNoQueuedRun) && !errors.Is(err, context.Canceled) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (worker *Worker) ProcessOne(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	run, err := worker.store.ClaimQueuedRun(ctx, worker.options.Now().UTC())
	if err != nil {
		return err
	}
	result, logs, failure := worker.execute(ctx, run)
	finished := worker.options.Now().UTC().Round(0)
	run.FinishedAt = &finished
	run.Logs = logs
	if failure != nil {
		run.Status = deployment.RunFailed
		run.Failure = failure
		run.Output = nil
	} else {
		run.Status = deployment.RunSucceeded
		run.Output = result
		run.Failure = nil
	}
	finalizeCtx := ctx
	if finalizeCtx == nil || finalizeCtx.Err() != nil {
		var cancel context.CancelFunc
		finalizeCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	if err := worker.store.FinalizeRun(finalizeCtx, run); err != nil {
		return fmt.Errorf("worker: finalize run %s: %w", run.ID, err)
	}
	return nil
}

func (worker *Worker) execute(ctx context.Context, run deployment.Run) (json.RawMessage, string, *deployment.Failure) {
	record, err := worker.store.GetDeployment(ctx, run.ProjectID, run.DeploymentID)
	if err != nil {
		return nil, "", newFailure("deployment_unavailable", err)
	}
	build, ok := record.BuildByID(run.BuildID)
	if !ok || build.Status != deployment.StatusReady {
		return nil, "", newFailure("build_unavailable", errors.New("pinned build is not ready"))
	}
	if build.Runtime != deployment.RuntimePython {
		return nil, "", newFailure("runtime_unsupported", fmt.Errorf("runtime %q is not supported", build.Runtime))
	}
	work, err := os.MkdirTemp("", "neurun-worker-*")
	if err != nil {
		return nil, "", newFailure("worker_environment", err)
	}
	defer os.RemoveAll(work)
	codeDir := filepath.Join(work, "code")
	installDir := filepath.Join(work, "install")
	seenCode := false
	remaining := worker.options.MaxArtifactBytes
	for _, item := range build.Artifacts {
		if item.Kind != deployment.ArtifactCodeLayer && item.Kind != deployment.ArtifactInstallLayer {
			continue
		}
		if item.SizeBytes > remaining {
			return nil, "", newFailure("artifact_invalid", errors.New("build artifacts exceed configured byte limit"))
		}
		remaining -= item.SizeBytes
		targetArchive := filepath.Join(work, item.ID+".zip")
		if err := worker.materialize(ctx, item, targetArchive); err != nil {
			return nil, "", newFailure("artifact_invalid", err)
		}
		destination := installDir
		if item.Kind == deployment.ArtifactCodeLayer {
			if seenCode {
				return nil, "", newFailure("artifact_invalid", errors.New("build has duplicate code layers"))
			}
			seenCode = true
			destination = codeDir
		}
		if _, err := artifact.ExtractZIPFile(targetArchive, destination, artifact.ArchiveLimits{MaxEntries: worker.options.MaxArchiveEntries, MaxExpandedBytes: worker.options.MaxArchiveExpandedBytes}); err != nil {
			return nil, "", newFailure("artifact_invalid", err)
		}
	}
	if !seenCode {
		return nil, "", newFailure("artifact_invalid", errors.New("build has no code layer"))
	}
	runCtx, cancel := context.WithTimeout(ctx, worker.options.RunTimeout)
	defer cancel()
	result, err := worker.runner.Execute(runCtx, ExecuteRequest{CodeDirectory: codeDir, InstallDirectory: installDir, Entrypoint: build.EntryPoint, Input: run.Input, MaxResultBytes: worker.options.MaxResultBytes, MaxLogBytes: worker.options.MaxLogBytes})
	if err != nil {
		code := "handler_failed"
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			code = "run_timeout"
		} else if errors.Is(err, ErrResultTooLarge) {
			code = "result_too_large"
		}
		return nil, result.Logs, newFailure(code, err)
	}
	return result.Output, result.Logs, nil
}

func (worker *Worker) materialize(ctx context.Context, item deployment.Artifact, targetPath string) error {
	reader, info, err := worker.blobs.Open(ctx, item.StorageKey())
	if err != nil {
		return err
	}
	defer reader.Close()
	if info.SizeBytes() != item.SizeBytes || info.SHA256() != item.SHA256 {
		return errors.New("artifact metadata does not match stored blob")
	}
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	result, copyErr := artifact.CopyAndHashContext(ctx, target, reader, item.SizeBytes)
	closeErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if result.SizeBytes != item.SizeBytes || result.SHA256 != item.SHA256 {
		return errors.New("materialized artifact failed integrity verification")
	}
	return nil
}

func newFailure(code string, err error) *deployment.Failure {
	message := "operation failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = strings.TrimSpace(err.Error())
	}
	if len(message) > 4096 {
		message = message[:4096]
	}
	return &deployment.Failure{Code: code, Message: message}
}
