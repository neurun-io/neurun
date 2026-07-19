package pkg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

func Run(ctx context.Context, dir string, env []string, name string, args ...string) error {
	cmd := command(ctx, dir, env, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return commandError(name, err, stderr.String())
	}
	return nil
}

func Output(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	output, err := command(ctx, dir, env, name, args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", commandError(name, err, string(exitErr.Stderr))
		}
		return "", commandError(name, err, "")
	}
	return string(output), nil
}

func commandError(name string, err error, stderr string) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return fmt.Errorf("%s failed: %w: %s", name, err, detail)
}

func command(ctx context.Context, dir string, env []string, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = slices.DeleteFunc(append(os.Environ(), env...), func(value string) bool {
		name, _, _ := strings.Cut(strings.ToUpper(value), "=")
		return strings.HasPrefix(name, "AWS_") || strings.HasPrefix(name, "R2_") || strings.HasPrefix(name, "SQS_")
	})
	return cmd
}
