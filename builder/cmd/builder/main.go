package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	listenersqs "github.com/dagflows/builder/internal/listener/sqs"
	"github.com/dagflows/builder/internal/service"
	"github.com/dagflows/builder/internal/storage"
	"github.com/dagflows/builder/pkg"
)

func main() {
	if err := pkg.LoadDotEnv(".env"); err != nil {
		log.Fatal(err)
	}

	store, err := storage.NewR2FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	builder := service.NewDeploymentService(service.NewBuildService(store))
	listener, err := listenersqs.NewListenerFromEnv(builder)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := listener.Listen(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}
