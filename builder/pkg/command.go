package pkg

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Run(ctx context.Context, dir string, env []string, name string, args ...string) error {
	_, err := runCommand(ctx, dir, env, name, args...)
	return err
}

func Output(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	return runCommand(ctx, dir, env, name, args...)
}

func runCommand(ctx context.Context, dir string, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return output.String(), fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, tail(output.String(), 4000))
	}
	return output.String(), nil
}
