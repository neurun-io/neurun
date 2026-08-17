// Package worker drains queued executions, running each against the exact
// build it pinned.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/neurun-io/neurun/internal/domain/build"
	"github.com/neurun-io/neurun/internal/domain/execution"
	"github.com/neurun-io/neurun/internal/files"
	"github.com/neurun-io/neurun/internal/repository/database"
	"github.com/neurun-io/neurun/internal/repository/file"
)

type ExecuteRequest struct {
	CodeDirectory    string
	InstallDirectory string
	// CallbackAddress is the control plane's loopback gRPC address, and
	// ExecutionToken is what proves a call on it belongs to this execution.
	CallbackAddress string
	ExecutionToken  string
	Input           json.RawMessage
	MaxResultBytes  int64
	MaxLogBytes     int64
	// Logs takes what the handler prints, as it prints it. A run can last
	// minutes and somebody is watching it; handing the output over at the end
	// would be handing it over after the interesting part.
	Logs io.Writer
}

type ExecuteResult struct {
	Output json.RawMessage
	Logs   string
}

// nullResult is what a handler that returned nothing returns. An execution row
// carries JSON, and absent is not one of the things JSON can be.
var nullResult = json.RawMessage("null")

// Runner is the boundary around a language runtime. It is behaviour, not
// storage: a second runtime plugs in here.
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
	// CallbackAddress is the control plane's loopback gRPC address. Empty leaves
	// handlers with no way to open a browser, which is the correct state when
	// nothing is serving it.
	CallbackAddress string
	// Tokens issues the per-execution credential. Nil has the same effect.
	Tokens ExecutionTokens
}

// ExecutionTokens mints the credential a handler calls back with, and spends it
// when the run ends. It is an interface so the worker never learns how identity
// is stored.
type ExecutionTokens interface {
	Mint(ctx context.Context, executionID, appID string) (string, error)
	Revoke(ctx context.Context, token string) error
}

type Worker struct {
	executions *database.ExecutionRepository
	builds     *database.BuildRepository
	blobs      file.Repository
	runners    map[build.Runtime]Runner
	options    Options
}

func New(
	executions *database.ExecutionRepository,
	builds *database.BuildRepository,
	blobs file.Repository,
	runners map[build.Runtime]Runner,
	options Options,
) (*Worker, error) {
	switch {
	case executions == nil:
		return nil, errors.New("worker: execution repository is required")
	case builds == nil:
		return nil, errors.New("worker: build repository is required")
	case blobs == nil:
		return nil, errors.New("worker: artifact store is required")
	case len(runners) == 0:
		return nil, errors.New("worker: at least one runner is required")
	}
	if options.PollInterval < 0 || options.RunTimeout < 0 ||
		options.MaxResultBytes < 0 || options.MaxLogBytes < 0 ||
		options.MaxArtifactBytes < 0 || options.MaxArchiveEntries < 0 ||
		options.MaxArchiveExpandedBytes < 0 {
		return nil, errors.New("worker: options cannot be negative")
	}
	if options.PollInterval == 0 {
		options.PollInterval = 250 * time.Millisecond
	}
	if options.RunTimeout == 0 {
		options.RunTimeout = 5 * time.Minute
	}
	if options.MaxResultBytes == 0 {
		options.MaxResultBytes = 4_194_304
	}
	if options.MaxLogBytes == 0 {
		options.MaxLogBytes = execution.MaxLogBytes
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
	return &Worker{
		executions: executions, builds: builds,
		blobs: blobs, runners: runners, options: options,
	}, nil
}

// Recover fails executions a previous process left running. It never re-runs
// them: whatever the first attempt did already happened.
func (worker *Worker) Recover(ctx context.Context) (int, error) {
	return worker.executions.RecoverRunning(
		ctx,
		worker.options.Now().UTC(),
		execution.Failure{
			Code:    "worker_restarted",
			Message: "worker stopped before the execution completed",
		},
	)
}

func (worker *Worker) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ticker := time.NewTicker(worker.options.PollInterval)
	defer ticker.Stop()
	for {
		err := worker.ProcessOne(ctx)
		if err != nil && !errors.Is(err, execution.ErrNoQueued) &&
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

func (worker *Worker) ProcessOne(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	record, err := worker.executions.ClaimQueued(ctx, worker.options.Now().UTC())
	if err != nil {
		return err
	}
	output, logs, failure := worker.execute(ctx, record)
	finished := worker.options.Now().UTC().Round(0)
	if failure != nil {
		err = record.Fail(*failure, logs, finished)
	} else {
		err = record.Succeed(output, logs, finished)
	}
	if err != nil {
		return fmt.Errorf("worker: finish execution %s: %w", record.ID, err)
	}
	// A cancelled context must not strand the execution in running: the row has
	// to reach a terminal state even while the process is shutting down.
	finalizeCtx := ctx
	if finalizeCtx.Err() != nil {
		var cancel context.CancelFunc
		finalizeCtx, cancel = context.WithTimeout(
			context.WithoutCancel(ctx), 5*time.Second,
		)
		defer cancel()
	}
	if err := worker.executions.Finalize(finalizeCtx, record); err != nil {
		return fmt.Errorf("worker: finalize execution %s: %w", record.ID, err)
	}
	return nil
}

func (worker *Worker) execute(
	ctx context.Context,
	record execution.Execution,
) (json.RawMessage, string, *execution.Failure) {
	produced, err := worker.builds.GetByID(ctx, record.BuildID)
	if err != nil {
		return nil, "", newFailure("build_unavailable", err)
	}
	work, err := os.MkdirTemp("", "neurun-worker-*")
	if err != nil {
		return nil, "", newFailure("worker_environment", err)
	}
	defer os.RemoveAll(work)

	remaining := worker.options.MaxArtifactBytes
	for _, layer := range produced.Artifacts {
		if layer.SizeBytes > remaining {
			return nil, "", newFailure("artifact_invalid", errors.New(
				"build artifacts exceed configured byte limit",
			))
		}
		remaining -= layer.SizeBytes
		targetArchive := filepath.Join(work, layer.ID+".zip")
		if err := worker.materialize(ctx, layer, targetArchive); err != nil {
			return nil, "", newFailure("artifact_invalid", err)
		}
		if _, err := files.ExtractZIPFile(
			targetArchive,
			filepath.Join(work, layer.Name),
			files.ArchiveLimits{
				MaxEntries:       worker.options.MaxArchiveEntries,
				MaxExpandedBytes: worker.options.MaxArchiveExpandedBytes,
			},
		); err != nil {
			return nil, "", newFailure("artifact_invalid", err)
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, worker.options.RunTimeout)
	defer cancel()
	runner, ok := worker.runners[produced.Runtime]
	if !ok {
		return nil, "", newFailure("runtime_unsupported", fmt.Errorf(
			"worker: no runner for runtime %q", produced.Runtime,
		))
	}
	// Minted for this run and spent when it ends, so a token that leaks cannot
	// outlive the execution that held it. A failure to mint is not a failure to
	// run: the handler simply has no browser support.
	var token string
	if worker.options.Tokens != nil && worker.options.CallbackAddress != "" {
		token, err = worker.options.Tokens.Mint(ctx, record.ID, produced.AppID)
		if err != nil {
			slog.Warn("execution token was not issued",
				"execution", record.ID, "error", err)
		} else {
			defer func() {
				if err := worker.options.Tokens.Revoke(
					context.WithoutCancel(ctx), token,
				); err != nil {
					slog.Warn("execution token was not revoked",
						"execution", record.ID, "error", err)
				}
			}()
		}
	}
	stream := worker.follow(ctx, record.ID)
	defer stream.Close()
	result, err := runner.Execute(runCtx, ExecuteRequest{
		Logs:             stream,
		CodeDirectory:    filepath.Join(work, build.LayerCode),
		InstallDirectory: filepath.Join(work, build.LayerInstall),
		Input:            record.Input,
		CallbackAddress:  worker.options.CallbackAddress,
		ExecutionToken:   token,
		MaxResultBytes:   worker.options.MaxResultBytes,
		MaxLogBytes:      worker.options.MaxLogBytes,
	})
	if err != nil {
		code := "failed"
		switch {
		case errors.Is(err, context.DeadlineExceeded),
			errors.Is(runCtx.Err(), context.DeadlineExceeded):
			code = "execution_timeout"
		case errors.Is(err, ErrResultTooLarge):
			// The row carries the output, so one too large for it fails the run.
			// TODO: put it in the artifact store instead and keep the handle on
			// the execution, which is what the size limit is really about.
			code = "result_too_large"
		}
		return nil, result.Logs, newFailure(code, err)
	}
	return result.Output, result.Logs, nil
}

func (worker *Worker) materialize(
	ctx context.Context,
	item build.Artifact,
	targetPath string,
) error {
	reader, info, err := worker.blobs.Open(ctx, item.StorageKey)
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
	result, copyErr := files.CopyAndHashContext(ctx, target, reader, item.SizeBytes)
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

func newFailure(code string, err error) *execution.Failure {
	message := "operation failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = strings.TrimSpace(err.Error())
	}
	if len(message) > 4096 {
		message = message[:4096]
	}
	return &execution.Failure{Code: code, Message: message}
}

// logFlush is how far behind the handler a reader is allowed to be. Output
// arrives in bursts, and writing the row on every burst would write it hundreds
// of times for output nobody reads that fast.
const logFlush = 2 * time.Second

// logStream carries handler output onto the execution while it is still
// running, so following one shows it happening rather than showing nothing and
// then everything.
type logStream struct {
	mutex   sync.Mutex
	buffer  strings.Builder
	written bool
	persist func(string)
	stop    chan struct{}
	stopped chan struct{}
}

func (worker *Worker) follow(ctx context.Context, executionID string) *logStream {
	stream := &logStream{
		persist: func(logs string) {
			if err := worker.executions.SaveLogs(ctx, executionID, logs); err != nil {
				slog.Warn(
					"persist execution logs", "execution", executionID, "error", err,
				)
			}
		},
		stop:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	go stream.flushEvery(logFlush)
	return stream
}

func (stream *logStream) Write(payload []byte) (int, error) {
	stream.mutex.Lock()
	defer stream.mutex.Unlock()
	stream.buffer.Write(payload)
	stream.written = true
	return len(payload), nil
}

func (stream *logStream) flushEvery(interval time.Duration) {
	defer close(stream.stopped)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stream.stop:
			return
		case <-ticker.C:
			stream.flush()
		}
	}
}

// flush writes what has arrived. The last of it is not written here: the
// execution is finalized with its logs a moment later, and that write is the
// one that has to win.
func (stream *logStream) flush() {
	stream.mutex.Lock()
	if !stream.written {
		stream.mutex.Unlock()
		return
	}
	stream.written = false
	logs := strings.TrimSpace(stream.buffer.String())
	stream.mutex.Unlock()
	stream.persist(logs)
}

func (stream *logStream) Close() {
	close(stream.stop)
	<-stream.stopped
}
