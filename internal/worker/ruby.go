package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type RubyOptions struct {
	Executable     string
	BrowserService string
}

// RubyRunner executes an interpreted handler out of two directories, the same
// shape as Python: the code layer is what it requires, the install layer is what
// its gems resolve from.
type RubyRunner struct {
	executable string
	browser    string
}

func NewRubyRunner(options RubyOptions) (*RubyRunner, error) {
	executable := strings.TrimSpace(options.Executable)
	if executable == "" {
		executable = "ruby"
	}
	return &RubyRunner{
		executable: executable,
		browser:    strings.TrimSpace(options.BrowserService),
	}, nil
}

func (runner *RubyRunner) Execute(
	ctx context.Context,
	request ExecuteRequest,
) (ExecuteResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	work, err := os.MkdirTemp("", "neurun-ruby-run-*")
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("worker: create Ruby run directory: %w", err)
	}
	defer os.RemoveAll(work)

	inputPath := filepath.Join(work, "input.json")
	resultPath := filepath.Join(work, "result.json")
	bootstrapPath := filepath.Join(work, "bootstrap.rb")
	if err := os.WriteFile(inputPath, request.Input, 0o600); err != nil {
		return ExecuteResult{}, err
	}
	if err := os.WriteFile(bootstrapPath, []byte(rubyBootstrap), 0o600); err != nil {
		return ExecuteResult{}, err
	}
	command := exec.CommandContext(
		ctx, runner.executable, "--disable-gems", bootstrapPath,
		request.CodeDirectory, request.InstallDirectory, rubyEntry,
		inputPath, resultPath, strconv.FormatInt(request.MaxResultBytes, 10),
	)
	configureProcessTree(command)
	command.Dir = work
	command.Env = append(childEnvironment(runner.browser, "GEM_PATH="+request.InstallDirectory), callbackEnvironment(request)...)
	logs := &limitedBuffer{maximum: request.MaxLogBytes, mirror: request.Logs}
	command.Stdout, command.Stderr = logs, logs
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ExecuteResult{Logs: logs.String()}, ctx.Err()
		}
		if strings.Contains(logs.String(), "handler result exceeds configured byte limit") {
			return ExecuteResult{Logs: logs.String()}, ErrResultTooLarge
		}
		return ExecuteResult{Logs: logs.String()}, fmt.Errorf(
			"%w: %s", ErrHandlerFailed, failureMessage(err, logs.String()),
		)
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

// rubyBootstrap loads the handler file and calls the named method. A top-level
// def becomes a private method on Object, so the call goes through send.
// rubyEntry is what the bootstrap loads and calls. Nothing configures it: it
// stands in until the SDK reports what it registered.
const rubyEntry = "main.rb:handler"

const rubyBootstrap = `
require "json"

code_dir, install_dir, entrypoint, input_path, result_path, maximum = ARGV
maximum = Integer(maximum)
$LOAD_PATH.unshift(code_dir) unless code_dir.empty?
$LOAD_PATH.unshift(install_dir) unless install_dir.empty?

file, handler = entrypoint.split(":", 2)
require File.expand_path(file, code_dir)

input = JSON.parse(File.read(input_path))
result = send(handler.to_sym, input)
encoded = JSON.generate(result)
if encoded.bytesize > maximum
  warn "handler result exceeds configured byte limit"
  exit 1
end
File.write(result_path, encoded)
`

var _ Runner = (*RubyRunner)(nil)
