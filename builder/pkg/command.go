package pkg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const maxCommandOutput = 1 << 20

func Run(ctx context.Context, dir string, env []string, name string, args ...string) error {
	_, err := runCommand(ctx, dir, env, false, name, args...)
	return err
}

func Output(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	return runCommand(ctx, dir, env, true, name, args...)
}

func runCommand(ctx context.Context, dir string, env []string, capture bool, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = commandEnv(env)

	var stdout cappedOutput
	if capture {
		cmd.Stdout = &stdout
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return stdout.String(), fmt.Errorf("%s failed: %w", name, err)
	}
	if len(stdout) > maxCommandOutput {
		return stdout.String(), fmt.Errorf("command output exceeded %d bytes", maxCommandOutput)
	}
	return stdout.String(), nil
}

func commandEnv(overrides []string) []string {
	all := append(os.Environ(), overrides...)
	result := all[:0]
	for _, entry := range all {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || sensitiveEnv(name) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func sensitiveEnv(name string) bool {
	name = strings.ToUpper(name)
	return strings.HasPrefix(name, "AWS_") || strings.HasPrefix(name, "R2_") || strings.HasPrefix(name, "SQS_")
}

type cappedOutput []byte

func (b *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := maxCommandOutput + 1 - len(*b); remaining > 0 {
		*b = append(*b, p[:min(remaining, n)]...)
	}
	return n, nil
}

func (b cappedOutput) String() string { return string(b[:min(len(b), maxCommandOutput)]) }
