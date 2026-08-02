.PHONY: all build test vet fmt check run clean

GO_PACKAGES = ./...

all: check build

build:
	go build -trimpath -o bin/neurun ./cmd/neurun

test:
	go test $(GO_PACKAGES)

vet:
	go vet $(GO_PACKAGES)

fmt:
	gofmt -w $$(find cmd internal -name '*.go' -type f)

check:
	test -z "$$(gofmt -l cmd internal migrations)"
	go vet $(GO_PACKAGES)
	go test -race $(GO_PACKAGES)

run:
	go run ./cmd/neurun

clean:
	go clean
