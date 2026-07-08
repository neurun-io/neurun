package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/dagflows/worker/internal/config"
	listenerredis "github.com/dagflows/worker/internal/listener/redis"
	"github.com/dagflows/worker/internal/service"
)

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatal(err)
	}

	nodeService, err := service.NewConfiguredNodeRunService(cfg)
	if err != nil {
		log.Fatal(err)
	}
	listener, err := listenerredis.NewListener(cfg, nodeService)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := listener.Listen(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}
