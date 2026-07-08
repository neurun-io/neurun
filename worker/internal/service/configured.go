package service

import (
	"fmt"

	"github.com/dagflows/worker/internal/config"
	"github.com/dagflows/worker/internal/vm"
)

func NewConfiguredNodeRunService(cfg config.Config) (*NodeRunService, error) {
	var runner vm.Runner
	switch cfg.Worker.RuntimeMode {
	case "host":
		runner = vm.NewHostRunner(cfg.Worker.HostPythonBinary)
	case "firecracker", "":
		firecracker, err := vm.NewFirecrackerRunner(cfg.Firecracker.RunnerCommand)
		if err != nil {
			return nil, err
		}
		runner = firecracker
	default:
		return nil, fmt.Errorf("unsupported WORKER_RUNTIME_MODE %q", cfg.Worker.RuntimeMode)
	}

	return NewNodeRunService(runner, cfg.Worker.WorkDir, cfg.Worker.OutputInlineMaxBytes), nil
}
