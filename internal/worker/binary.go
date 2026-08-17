package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// BinaryName is the file a compiled build ships in its code layer. It matches
// builder.CompiledBinaryName, extension and all; the two packages agree on a
// name rather than on a manifest, and neither imports the other.
var BinaryName = "app" + executableExtension()

func executableExtension() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// BinaryRunner executes a compiled handler — Rust or Go, which differ only in
// what produced the file.
//
// The contract is the same file protocol the Python bootstrap uses, minus the
// bootstrap: the process is handed an input path and a result path, and writes
// JSON to the second. There is no install directory, because a compiled build
// has no second layer to load from.
type BinaryRunner struct{ browser string }

type BinaryOptions struct {
	// BrowserService is where the image keeps the neurun-browser executable.
	BrowserService string
}

func NewBinaryRunner(options BinaryOptions) (*BinaryRunner, error) {
	return &BinaryRunner{browser: strings.TrimSpace(options.BrowserService)}, nil
}

func (runner *BinaryRunner) Execute(
	ctx context.Context,
	request ExecuteRequest,
) (ExecuteResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	work, err := os.MkdirTemp("", "neurun-binary-run-*")
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("worker: create run directory: %w", err)
	}
	defer os.RemoveAll(work)

	inputPath := filepath.Join(work, "input.json")
	resultPath := filepath.Join(work, "result.json")
	if err := os.WriteFile(inputPath, request.Input, 0o600); err != nil {
		return ExecuteResult{}, err
	}
	binary := filepath.Join(request.CodeDirectory, BinaryName)
	info, err := os.Lstat(binary)
	if err != nil || !info.Mode().IsRegular() {
		return ExecuteResult{}, fmt.Errorf(
			"%w: build contains no %s", ErrHandlerFailed, BinaryName,
		)
	}
	// Extraction masks stored permissions, so the bit is set here rather than
	// trusted to have survived the archive.
	if err := os.Chmod(binary, 0o700); err != nil {
		return ExecuteResult{}, fmt.Errorf("worker: prepare handler: %w", err)
	}

	command := exec.CommandContext(ctx, binary, inputPath, resultPath)
	configureProcessTree(command)
	command.Dir = work
	command.Env = append(childEnvironment(runner.browser), callbackEnvironment(request)...)
	logs := &limitedBuffer{maximum: request.MaxLogBytes, mirror: request.Logs}
	command.Stdout, command.Stderr = logs, logs
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ExecuteResult{Logs: logs.String()}, ctx.Err()
		}
		return ExecuteResult{Logs: logs.String()}, fmt.Errorf(
			"%w: %s", ErrHandlerFailed, failureMessage(err, logs.String()),
		)
	}
	output, err := os.ReadFile(resultPath)
	if errors.Is(err, os.ErrNotExist) {
		// A compiled handler has no bootstrap to write the file for it, so the
		// result is optional and the exit code is the whole contract. A run that
		// finished with nothing to say returns what an interpreted handler
		// returning nothing returns.
		return ExecuteResult{Output: nullResult, Logs: logs.String()}, nil
	}
	if err != nil {
		return ExecuteResult{Logs: logs.String()}, fmt.Errorf(
			"worker: read handler result: %w", err,
		)
	}
	if int64(len(output)) > request.MaxResultBytes {
		return ExecuteResult{Logs: logs.String()}, ErrResultTooLarge
	}
	if !json.Valid(output) {
		return ExecuteResult{Logs: logs.String()}, errors.New(
			"worker: handler returned invalid JSON",
		)
	}
	return ExecuteResult{Output: json.RawMessage(output), Logs: logs.String()}, nil
}

var _ Runner = (*BinaryRunner)(nil)
