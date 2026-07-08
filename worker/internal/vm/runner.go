package vm

import (
	"context"
	"time"

	"github.com/dagflows/worker/internal/artifact"
)

type Runner interface {
	Run(ctx context.Context, workload *artifact.PreparedWorkload) (Result, error)
}

type Result struct {
	Output   []byte
	Duration time.Duration
}

type RunError struct {
	Message   string
	Category  string
	Retryable bool
}

func (e RunError) Error() string {
	return e.Message
}

func timeoutFor(timeoutSeconds int) time.Duration {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	return time.Duration(timeoutSeconds+30) * time.Second
}

func tail(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[len(value)-max:]
}
