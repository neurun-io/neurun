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
)

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatal(err)
	}

	deploymentService, err := service.NewConfiguredDeploymentService(cfg)
	if err != nil {
		log.Fatal(err)
	}
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
