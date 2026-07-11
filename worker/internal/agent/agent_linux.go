//go:build linux

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mdlayher/vsock"
	"golang.org/x/sys/unix"

	"github.com/dagflows/worker/internal/protocol"
)

const (
	root         = "/srv/dagflows"
	codeDir      = root + "/code"
	depsDir      = root + "/deps"
	runtimeDir   = root + "/runtime"
	inputPath    = root + "/input.json"
	manifestPath = root + "/manifest.json"
	overlayDir   = "/run/dagflows-overlay"
)

func Run() error {
	conn, err := vsock.Dial(vsock.Host, protocol.Port, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := json.NewEncoder(conn).Encode(protocol.Ready{Type: "ready"}); err != nil {
		return err
	}
	var request protocol.RunRequest
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		return err
	}
	return json.NewEncoder(conn).Encode(execute(request))
}

func execute(request protocol.RunRequest) protocol.RunResult {
	start := time.Now()
	result := protocol.RunResult{Type: "result", ID: request.ID, ExecutionToken: request.ExecutionToken}
	fail := func(err error, category string, retryable bool) protocol.RunResult {
		result.Error, result.Category, result.Retryable = err.Error(), category, retryable
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}

	var manifest protocol.Manifest
	if request.Type != "run" || json.Unmarshal(request.Manifest, &manifest) != nil || len(manifest.Command) == 0 {
		return fail(errors.New("invalid run request"), "infrastructure", true)
	}
	if err := mountWorkload(request); err != nil {
		return fail(err, "infrastructure", true)
	}

	timeout := time.Duration(manifest.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, manifest.Command[0], manifest.Command[1:]...)
	cmd.Dir = runtimeDir
	cmd.Env = append(os.Environ(), environment(manifest.Env)...)
	cmd.Stdin = bytes.NewReader(request.Input)
	output, err := cmd.Output()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fail(errors.New("node execution timed out"), "timeout", false)
	}
	if err != nil {
		if exitErr := new(exec.ExitError); errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return fail(err, "execution", false)
	}
	output = bytes.TrimSpace(output)
	if len(output) == 0 {
		output = []byte(`{}`)
	}
	if !json.Valid(output) {
		return fail(errors.New("node output is not valid JSON"), "permanent", false)
	}
	result.Output = output
	result.DurationMS = time.Since(start).Milliseconds()
	return result
}

func mountWorkload(request protocol.RunRequest) error {
	for _, path := range []string{root, codeDir, depsDir, runtimeDir, overlayDir} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return err
		}
	}
	if err := unix.Mount("/dev/vdb", codeDir, "ext4", unix.MS_RDONLY, ""); err != nil {
		return err
	}
	if err := unix.Mount("/dev/vdc", depsDir, "ext4", unix.MS_RDONLY, ""); err != nil {
		return err
	}
	if err := unix.Mount("tmpfs", overlayDir, "tmpfs", 0, "size=128m"); err != nil {
		return err
	}
	upper, work := filepath.Join(overlayDir, "upper"), filepath.Join(overlayDir, "work")
	if err := os.MkdirAll(upper, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	options := fmt.Sprintf("lowerdir=%s:%s,upperdir=%s,workdir=%s", codeDir, depsDir, upper, work)
	if err := unix.Mount("overlay", runtimeDir, "overlay", 0, options); err != nil {
		return err
	}
	if err := os.WriteFile(inputPath, request.Input, 0o600); err != nil {
		return err
	}
	return os.WriteFile(manifestPath, request.Manifest, 0o600)
}

func environment(extra map[string]string) []string {
	env := []string{
		"DAGFLOWS_WORK_DIR=" + runtimeDir,
		"DAGFLOWS_CODE_DIR=" + codeDir,
		"DAGFLOWS_DEPS_DIR=" + depsDir,
		"DAGFLOWS_INPUT=" + inputPath,
		"DAGFLOWS_MANIFEST=" + manifestPath,
	}
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	return env
}
