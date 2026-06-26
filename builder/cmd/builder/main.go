package main

import (
	"log"
	"net"
	"strings"

	handlergrpc "github.com/dagflows/builder/internal/handler/grpc"
	"github.com/dagflows/builder/internal/service"
	"github.com/dagflows/builder/internal/storage"
	"github.com/dagflows/builder/pkg"
	"google.golang.org/grpc"
)

func main() {
	if err := pkg.LoadDotEnv(".env"); err != nil {
		log.Fatal(err)
	}

	store, err := storage.NewR2FromEnv()
	if err != nil {
		log.Fatal(err)
	}

	addr := ":" + strings.TrimPrefix(pkg.EnvOr("GRPC_PORT", "50051"), ":")
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}

	server := grpc.NewServer()
	handlergrpc.Register(server, service.NewBuildService(store))

	log.Printf("builderd listening on %s", addr)
	if err := server.Serve(listener); err != nil {
		log.Fatal(err)
	}
}
