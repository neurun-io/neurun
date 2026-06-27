package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dagflows/builder/internal/config"
	listenersqs "github.com/dagflows/builder/internal/listener/sqs"
	"github.com/dagflows/builder/internal/service"
	"github.com/dagflows/builder/internal/storage"
)

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatal(err)
	}

	store, err := storage.NewR2(cfg.R2)
	if err != nil {
		log.Fatal(err)
	}

	buildService := service.NewBuildService(store)
	github := service.NewGitHubService(cfg.GitHub)
	deploymentService := service.NewDeploymentService(buildService, github)
	listener, err := listenersqs.NewListener(cfg, deploymentService)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := listener.Listen(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}
