package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var (
	ErrHandlerFailed  = errors.New("worker: handler failed")
	ErrResultTooLarge = errors.New("worker: handler result is too large")
)

type PythonOptions struct {
	Executable string
	// BrowserService is where the image keeps the neurun-browser executable. A
	// handler that drives a browser is told the path rather than searching for
	// it, because the child environment is an allowlist and PATH alone would not
	// settle which build it got.
	BrowserService string
}

type PythonRunner struct {
	executable string
	browser    string
}

func NewPythonRunner(options PythonOptions) (*PythonRunner, error) {
	executable := strings.TrimSpace(options.Executable)
	if executable == "" {
		executable = "python"
	}
	return &PythonRunner{
		executable: executable,
		browser:    strings.TrimSpace(options.BrowserService),
	}, nil
}

func (runner *PythonRunner) Execute(ctx context.Context, request ExecuteRequest) (ExecuteResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	work, err := os.MkdirTemp("", "neurun-python-run-*")
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("worker: create Python run directory: %w", err)
	}
	defer os.RemoveAll(work)
	inputPath := filepath.Join(work, "input.json")
	resultPath := filepath.Join(work, "result.json")
	bootstrapPath := filepath.Join(work, "bootstrap.py")
	if err := os.WriteFile(inputPath, request.Input, 0o600); err != nil {
		return ExecuteResult{}, err
	}
	if err := os.WriteFile(bootstrapPath, []byte(pythonBootstrap), 0o600); err != nil {
		return ExecuteResult{}, err
	}
	command := exec.CommandContext(ctx, runner.executable, "-I", bootstrapPath, request.CodeDirectory, request.InstallDirectory, pythonEntry, inputPath, resultPath, strconv.FormatInt(request.MaxResultBytes, 10))
	configureProcessTree(command)
	command.Dir = work
	command.Env = append(pythonEnvironment(runner.browser), callbackEnvironment(request)...)
	logs := &limitedBuffer{maximum: request.MaxLogBytes, mirror: request.Logs}
	command.Stdout, command.Stderr = logs, logs
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ExecuteResult{Logs: logs.String()}, ctx.Err()
		}
		if strings.Contains(logs.String(), "handler result exceeds configured byte limit") {
			return ExecuteResult{Logs: logs.String()}, ErrResultTooLarge
		}
		return ExecuteResult{Logs: logs.String()}, fmt.Errorf("%w: %s", ErrHandlerFailed, failureMessage(err, logs.String()))
	}
	output, err := os.ReadFile(resultPath)
	if errors.Is(err, os.ErrNotExist) {
		// The process exited cleanly and wrote nothing. That is the handler's
		// side of the contract unmet, not a fault of this host, so it reads as
		// what it is rather than as a missing file.
		return ExecuteResult{Logs: logs.String()}, fmt.Errorf(
			"%w: the handler returned no result", ErrHandlerFailed,
		)
	}
	if err != nil {
		return ExecuteResult{Logs: logs.String()}, fmt.Errorf("worker: read handler result: %w", err)
	}
	if int64(len(output)) > request.MaxResultBytes {
		return ExecuteResult{Logs: logs.String()}, ErrResultTooLarge
	}
	if !json.Valid(output) {
		return ExecuteResult{Logs: logs.String()}, errors.New("worker: handler returned invalid JSON")
	}
	return ExecuteResult{Output: json.RawMessage(output), Logs: logs.String()}, nil
}

// childEnvironment is the allowlist every runtime's handler runs under. It is a
// list rather than a filter so a variable the host happens to carry — a cloud
// credential, a proxy setting — cannot reach a tenant's code by accident.
func childEnvironment(browser string, extra ...string) []string {
	allowed := []string{"PATH", "PATHEXT", "SYSTEMDRIVE", "SYSTEMROOT", "WINDIR", "TEMP", "TMP", "TMPDIR", "LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR"}
	environment := append([]string{}, extra...)
	for _, name := range allowed {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	if browser != "" {
		environment = append(environment, "NEURUN_BROWSER_SERVICE="+browser)
	}
	return environment
}

// callbackEnvironment is how a handler finds the control plane and proves who
// it is. Both are per execution, so they travel with the request rather than
// living on the runner.
func callbackEnvironment(request ExecuteRequest) []string {
	var environment []string
	if request.CallbackAddress != "" {
		environment = append(environment, "NEURUN_GRPC_ADDRESS="+request.CallbackAddress)
	}
	if request.ExecutionToken != "" {
		environment = append(environment, "NEURUN_EXECUTION_TOKEN="+request.ExecutionToken)
	}
	return environment
}

func pythonEnvironment(browser string) []string {
	return childEnvironment(browser,
		"PYTHONNOUSERSITE=1", "PYTHONDONTWRITEBYTECODE=1",
		"PYTHONUNBUFFERED=1", "PIP_DISABLE_PIP_VERSION_CHECK=1",
	)
}

// limitedBuffer keeps the head of what a handler printed, up to a cap, and
// mirrors it to whoever is following the execution as it arrives.
//
// A command's stdout and stderr are two pipes drained by two goroutines, so the
// lock is not optional.
type limitedBuffer struct {
	mutex   sync.Mutex
	buffer  bytes.Buffer
	maximum int64
	written int64
	mirror  io.Writer
}

func (target *limitedBuffer) Write(payload []byte) (int, error) {
	target.mutex.Lock()
	target.written += int64(len(payload))
	remaining := target.maximum - int64(target.buffer.Len())
	if remaining > 0 {
		if int64(len(payload)) > remaining {
			_, _ = target.buffer.Write(payload[:remaining])
		} else {
			_, _ = target.buffer.Write(payload)
		}
	}
	mirror := target.mirror
	target.mutex.Unlock()
	if mirror != nil {
		_, _ = mirror.Write(payload)
	}
	return len(payload), nil
}

func (target *limitedBuffer) String() string {
	target.mutex.Lock()
	defer target.mutex.Unlock()
	value := strings.TrimSpace(target.buffer.String())
	if target.written > int64(target.buffer.Len()) {
		value += "\n[logs truncated]"
	}
	return strings.TrimSpace(value)
}

func failureMessage(err error, logs string) string {
	logs = strings.TrimSpace(logs)
	if logs == "" {
		return err.Error()
	}
	if len(logs) > 4096 {
		logs = logs[len(logs)-4096:]
	}
	return logs
}

// pythonEntry is what the bootstrap imports and calls. Nothing configures it:
// it stands in until the SDK reports what it registered.
const pythonEntry = "main.py:handler"

const pythonBootstrap = `
import asyncio
import importlib
import importlib.util
import inspect
import json
import os
import sys

code_dir, install_dir, entrypoint, input_path, result_path, maximum = sys.argv[1:]
maximum = int(maximum)
sys.path[:0] = [p for p in (code_dir, install_dir) if p]
subject, handler_name = entrypoint.rsplit(":", 1)
if subject.endswith(".py") or "/" in subject:
    path = os.path.join(code_dir, *subject.split("/"))
    spec = importlib.util.spec_from_file_location("neurun_handler", path)
    if spec is None or spec.loader is None:
        raise RuntimeError("unable to load handler module")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
else:
    module = importlib.import_module(subject)
handler = getattr(module, handler_name)
if inspect.isclass(handler):
    handler = handler()
if not callable(handler):
    raise TypeError("entrypoint handler is not callable")
with open(input_path, "r", encoding="utf-8") as stream:
    event = json.load(stream)
result = handler(event)
if inspect.isawaitable(result):
    result = asyncio.run(result)
encoded = json.dumps(result, ensure_ascii=False, separators=(",", ":"), allow_nan=False).encode("utf-8")
if len(encoded) > maximum:
    raise ValueError("handler result exceeds configured byte limit")
with open(result_path, "xb") as stream:
    stream.write(encoded)
`

var _ Runner = (*PythonRunner)(nil)
var _ io.Writer = (*limitedBuffer)(nil)
