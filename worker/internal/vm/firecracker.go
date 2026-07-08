package vm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dagflows/worker/internal/artifact"
)

type FirecrackerRunner struct {
	command string
}

func NewFirecrackerRunner(command string) (*FirecrackerRunner, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("FIRECRACKER_RUNNER_COMMAND is required when WORKER_RUNTIME_MODE=firecracker")
	}
	return &FirecrackerRunner{command: command}, nil
}

func (r *FirecrackerRunner) Run(ctx context.Context, workload *artifact.PreparedWorkload) (Result, error) {
	timeout := timeoutFor(workload.Request.TimeoutSeconds)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	outputPath := filepath.Join(workload.WorkDir, "output.json")
	args := []string{
		"--work-dir", workload.WorkDir,
		"--manifest", workload.ManifestPath,
		"--input", workload.InputPath,
		"--code-dir", workload.CodeDir,
		"--deps-dir", workload.DepsDir,
		"--output", outputPath,
	}

	start := time.Now()
	cmd := exec.CommandContext(runCtx, r.command, args...)
	cmd.Dir = workload.WorkDir
	cmd.Env = append(os.Environ(),
		"DAGFLOWS_WORK_DIR="+workload.WorkDir,
		"DAGFLOWS_MANIFEST="+workload.ManifestPath,
		"DAGFLOWS_INPUT="+workload.InputPath,
		"DAGFLOWS_CODE_DIR="+workload.CodeDir,
		"DAGFLOWS_DEPS_DIR="+workload.DepsDir,
		"DAGFLOWS_OUTPUT="+outputPath,
	)

	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	err := cmd.Run()
	duration := time.Since(start)
	if runCtx.Err() == context.DeadlineExceeded {
		return Result{Duration: duration}, RunError{Message: "firecracker execution timed out", Category: "infrastructure", Retryable: true}
	}
	if err != nil {
		return Result{Duration: duration}, RunError{Message: fmt.Sprintf("firecracker runner failed: %v\n%s", err, tail(combined.String(), 4000)), Category: "infrastructure", Retryable: true}
	}

	output, readErr := os.ReadFile(outputPath)
	if readErr == nil {
		return Result{Output: output, Duration: duration}, nil
	}
	stdout := bytes.TrimSpace(combined.Bytes())
	if len(stdout) == 0 {
		return Result{Duration: duration}, RunError{Message: "firecracker runner produced no output", Category: "infrastructure", Retryable: true}
	}
	return Result{Output: stdout, Duration: duration}, nil
}
