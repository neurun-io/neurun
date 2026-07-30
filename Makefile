.PHONY: all build test vet fmt check check-all run dev clean \
	web-install web-build web-test web-check

# The Go package set is listed explicitly rather than using ./..., because
# frontend/node_modules contains vendored Go files (npm's `flatted` ships one)
# that ./... would otherwise pick up and try to vet, test and cover.
GO_PACKAGES = ./cmd/... ./internal/... ./migrations/...

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

# Control plane plus the operator dashboard, with the dashboard proxying
# same-origin to the server. Ctrl-C stops both.
dev:
	./scripts/dev-stack.sh

clean:
	go clean

# ---------------------------------------------------------------------------
# Operator dashboard (frontend/)
#
# Separate targets rather than folded into `check`: the Go checks must stay
# runnable without a Node toolchain installed. Use `check-all` for both.
# ---------------------------------------------------------------------------

web-install:
	cd frontend && npm ci

web-build:
	cd frontend && npm run build

web-test:
	cd frontend && npm test

# `check:api-drift` regenerates the client from api/openapi.yaml and fails on
# an uncommitted diff, so the dashboard cannot drift from the contract.
web-check:
	cd frontend && npm run typecheck
	cd frontend && npm run lint
	cd frontend && npm test
	cd frontend && npm run check:api-drift

check-all: check web-check

