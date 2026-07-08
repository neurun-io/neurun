package vm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dagflows/worker/internal/artifact"
)

type HostRunner struct {
	pythonBinary string
}

func NewHostRunner(pythonBinary string) *HostRunner {
	if strings.TrimSpace(pythonBinary) == "" {
		pythonBinary = "python"
	}
	return &HostRunner{pythonBinary: pythonBinary}
}

func (r *HostRunner) Run(ctx context.Context, workload *artifact.PreparedWorkload) (Result, error) {
	switch strings.ToLower(strings.TrimSpace(workload.Request.Language)) {
	case "python", "py":
		return r.runPython(ctx, workload)
	default:
		return Result{}, RunError{
			Message:   fmt.Sprintf("host runtime does not support language %q; use firecracker mode or add a runner adapter", workload.Request.Language),
			Category:  "permanent",
			Retryable: false,
		}
	}
}

func (r *HostRunner) runPython(ctx context.Context, workload *artifact.PreparedWorkload) (Result, error) {
	timeout := timeoutFor(workload.Request.TimeoutSeconds)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	input, err := os.ReadFile(workload.InputPath)
	if err != nil {
		return Result{}, RunError{Message: "read runtime input: " + err.Error(), Category: "infrastructure", Retryable: true}
	}

	start := time.Now()
	cmd := exec.CommandContext(runCtx, r.pythonBinary, "-m", "dagflows", "invoke", "--node", workload.Request.NodeKey)
	cmd.Dir = workload.CodeDir
	cmd.Env = append(os.Environ(), "PYTHONPATH="+pythonPath(workload.CodeDir, workload.DepsDir))
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	duration := time.Since(start)
	if runCtx.Err() == context.DeadlineExceeded {
		return Result{Duration: duration}, RunError{Message: "runtime timed out", Category: "infrastructure", Retryable: true}
	}
	if err != nil {
		return Result{Duration: duration}, RunError{Message: fmt.Sprintf("python runtime failed: %v\n%s", err, tail(stderr.String()+stdout.String(), 4000)), Category: "permanent", Retryable: false}
	}
	return Result{Output: stdout.Bytes(), Duration: duration}, nil
}

func pythonPath(codeDir, depsDir string) string {
	sep := ":"
	if runtime.GOOS == "windows" {
		sep = ";"
	}
	return strings.Join([]string{codeDir, depsDir, filepath.Join(depsDir, "src")}, sep)
}
