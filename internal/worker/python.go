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
)

var (
	ErrHandlerFailed  = errors.New("worker: Python handler failed")
	ErrResultTooLarge = errors.New("worker: handler result is too large")
)

type PythonOptions struct{ Executable string }

type PythonRunner struct{ executable string }

func NewPythonRunner(options PythonOptions) (*PythonRunner, error) {
	executable := strings.TrimSpace(options.Executable)
	if executable == "" {
		executable = "python"
	}
	return &PythonRunner{executable: executable}, nil
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
	command := exec.CommandContext(ctx, runner.executable, "-I", bootstrapPath, request.CodeDirectory, request.InstallDirectory, request.Entrypoint, inputPath, resultPath, strconv.FormatInt(request.MaxResultBytes, 10))
	configureProcessTree(command)
	command.Dir = work
	command.Env = pythonEnvironment()
	logs := &limitedBuffer{maximum: request.MaxLogBytes}
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

func pythonEnvironment() []string {
	allowed := []string{"PATH", "PATHEXT", "SYSTEMDRIVE", "SYSTEMROOT", "WINDIR", "TEMP", "TMP", "TMPDIR", "LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR"}
	environment := []string{"PYTHONNOUSERSITE=1", "PYTHONDONTWRITEBYTECODE=1", "PYTHONUNBUFFERED=1", "PIP_DISABLE_PIP_VERSION_CHECK=1"}
	for _, name := range allowed {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

type limitedBuffer struct {
	buffer  bytes.Buffer
	maximum int64
	written int64
}

func (target *limitedBuffer) Write(payload []byte) (int, error) {
	target.written += int64(len(payload))
	remaining := target.maximum - int64(target.buffer.Len())
	if remaining > 0 {
		if int64(len(payload)) > remaining {
			_, _ = target.buffer.Write(payload[:remaining])
		} else {
			_, _ = target.buffer.Write(payload)
		}
	}
	return len(payload), nil
}
func (target *limitedBuffer) String() string {
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
