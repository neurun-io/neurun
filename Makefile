.PHONY: all build test vet fmt check run clean

all: check build

build:
	go build -trimpath -o bin/neurun ./cmd/neurun

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w $$(find cmd internal -name '*.go' -type f)

check:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	go test -race ./...

run:
	go run ./cmd/neurun

clean:
	go clean

