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

// BundleName is the file a Node build ships in its code layer. It matches
// builder.BundleName; the two packages agree on a name rather than a manifest.
const BundleName = "handler.js"

type NodeOptions struct {
	Executable     string
	BrowserService string
}

// NodeRunner executes a bundled handler. There is no install directory: the
// bundle carries its dependencies, which is the whole point of building one.
type NodeRunner struct {
	executable string
	browser    string
}

func NewNodeRunner(options NodeOptions) (*NodeRunner, error) {
	executable := strings.TrimSpace(options.Executable)
	if executable == "" {
		executable = "node"
	}
	return &NodeRunner{
		executable: executable,
		browser:    strings.TrimSpace(options.BrowserService),
	}, nil
}

func (runner *NodeRunner) Execute(
	ctx context.Context,
	request ExecuteRequest,
) (ExecuteResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	work, err := os.MkdirTemp("", "neurun-node-run-*")
	if err != nil {
		return ExecuteResult{}, fmt.Errorf("worker: create Node run directory: %w", err)
	}
	defer os.RemoveAll(work)

	inputPath := filepath.Join(work, "input.json")
	resultPath := filepath.Join(work, "result.json")
	bootstrapPath := filepath.Join(work, "bootstrap.js")
	if err := os.WriteFile(inputPath, request.Input, 0o600); err != nil {
		return ExecuteResult{}, err
	}
	if err := os.WriteFile(bootstrapPath, []byte(nodeBootstrap), 0o600); err != nil {
		return ExecuteResult{}, err
	}
	bundle := filepath.Join(request.CodeDirectory, BundleName)
	if info, err := os.Lstat(bundle); err != nil || !info.Mode().IsRegular() {
		return ExecuteResult{}, fmt.Errorf(
			"%w: build contains no %s", ErrHandlerFailed, BundleName,
		)
	}
	command := exec.CommandContext(
		ctx, runner.executable, bootstrapPath,
		bundle, nodeHandler, inputPath, resultPath,
		strconv.FormatInt(request.MaxResultBytes, 10),
	)
	configureProcessTree(command)
	command.Dir = work
	command.Env = append(childEnvironment(runner.browser, "NODE_ENV=production"), callbackEnvironment(request)...)
	logs := &limitedBuffer{maximum: request.MaxLogBytes}
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

// nodeBootstrap requires the bundle and calls the named export. A returned
// promise is awaited, so an async handler needs no ceremony — the same
// affordance the Python bootstrap gives a coroutine.
// nodeHandler is the export the bundle is called through. Nothing configures
// it: it stands in until the SDK reports what it registered.
const nodeHandler = "handler"

const nodeBootstrap = `
const fs = require("node:fs");

const [bundlePath, handlerName, inputPath, resultPath, maximum] = process.argv.slice(2);
const limit = Number(maximum);

async function main() {
  const module = require(bundlePath);
  const handler = module?.[handlerName] ?? module?.default?.[handlerName];
  if (typeof handler !== "function") {
    throw new Error("handler " + handlerName + " is not exported by the bundle");
  }
  const input = JSON.parse(fs.readFileSync(inputPath, "utf8"));
  const encoded = JSON.stringify(await handler(input));
  if (encoded === undefined) {
    throw new Error("handler returned a value that is not JSON");
  }
  if (Buffer.byteLength(encoded) > limit) {
    console.error("handler result exceeds configured byte limit");
    process.exit(1);
  }
  fs.writeFileSync(resultPath, encoded);
}

main().catch((error) => {
  console.error(error?.stack ?? String(error));
  process.exit(1);
});
`

var _ Runner = (*NodeRunner)(nil)
