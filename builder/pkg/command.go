package pkg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

func Run(ctx context.Context, dir string, env []string, name string, args ...string) error {
	if err := command(ctx, dir, env, name, args...).Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func Output(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	output, err := command(ctx, dir, env, name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w", name, err)
	}
	return string(output), nil
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
