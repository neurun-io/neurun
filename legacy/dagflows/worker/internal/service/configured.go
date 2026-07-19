//go:build linux

package service

import (
	"github.com/dagflows/worker/internal/config"
	"github.com/dagflows/worker/internal/storage"
	"github.com/dagflows/worker/internal/vm"
)

func NewConfiguredNodeRunService(cfg config.Config) (*NodeRunService, error) {
	fetcher, err := storage.NewR2(cfg.R2)
	if err != nil {
		return nil, err
	}

	runner, err := vm.NewFirecrackerRunner()
	if err != nil {
		return nil, err
	}

	return NewNodeRunService(runner, fetcher, cfg.Worker.OutputInlineMaxBytes), nil
}
