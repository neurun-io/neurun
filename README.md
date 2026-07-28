# Neurun

Neurun is a self-hosted execution and evidence plane for reliable HTTP and
Chromium scraping workflows. This repository contains the server-side control
API, dispatcher, agent runtime, contracts, migrations, and deployment assets.
The public Go SDK remains an independently versioned repository and imports no
server implementation packages.

## Current foundation

This consolidation establishes the first deployable vertical slice of the MVP:

- immutable, digest-addressed built-in atomic functions;
- authenticated direct invocation and explicitly gated process-local jobs;
- idempotent acceptance, outbox dispatch, leases, cancellation, and events;
- an SSRF-aware HTTP runtime with bounded response bodies;
- structured failures, trace IDs, and per-invocation usage;
- OpenAPI and database contracts for the broader MVP;
- a standalone frontend implementation specification.

Browser sessions, persistent PostgreSQL and JetStream adapters, extraction
breadth, profiles, proxies, and the operator dashboard remain explicit later
milestones. Interfaces and contracts are kept stable so those adapters can
replace the in-process development implementations without changing the public
API. The all-in-one binary reports job durability as `process_local`; restarts
discard its jobs and asynchronous routes remain disabled unless
`NEURUN_ALLOW_VOLATILE_JOBS=true`.

## Quick start

Requirements: Go 1.25 or newer.

```sh
cp .env.example .env
go test ./...
docker compose --env-file .env up --build
```

To run the development binary directly, export at least
`NEURUN_API_KEY=neu_live_local.development-only-change-me` and
`NEURUN_ALLOW_VOLATILE_JOBS=true` before `go run ./cmd/neurun`.

The server listens on `:8080` by default. Use the development key:

```sh
curl -H "Authorization: Bearer neu_live_local.development-only-change-me" \
  http://localhost:8080/v1/functions
```

Never use the example key outside local development.

## User application delivery

Neurun does not clone or build application source inside the control plane.
Application repositories can instead call the reusable GitHub Actions pipeline
in `.github/workflows/user-app-ci-cd.yml`. It validates pinned Neurun
contracts, runs repository-owned test/build/smoke scripts, publishes an
immutable OCI image with provenance and SBOM attestations, and returns the
digest for an environment-gated deployment.

Start from `templates/user-app/`. The caller pins both this repository and the
Neurun server image by digest so a release cannot silently change underneath a
user application.

## Repository layout

```text
api/                 OpenAPI contract
cmd/neurun/          all-in-one development process
contracts/           function and event schemas
internal/api/        HTTP transport and authentication
internal/agent/      leased work execution and capacity
internal/function/   manifests, registry, schemas, invocation
internal/job/        job state, outbox, leases, and events
internal/netpolicy/  outbound request policy
migrations/          PostgreSQL source-of-truth schema
templates/user-app/  external application CI/CD handoff
legacy/dagflows/     preserved Builder and Worker source lineage
```

## Source lineage

The former Dagflows Builder and Worker histories are part of this repository's
linear `main` ancestry. Each rewritten source commit retains its original
author, timestamp, and subject, and includes `Source-Repo` and
`Source-Revision` trailers. Their final trees are retained under
`legacy/dagflows/` for auditability.

The old arbitrary Git build pipeline, SQS/Redis protocols, and
Firecracker/Python node runtime are not active Neurun components. They conflict
with the MVP boundary of release-built immutable Go functions and durable
PostgreSQL/NATS execution. Safe ideas—artifact hashing, bounded admission,
publish-before-ack discipline, process cleanup, and classified errors—are
refactored into the new packages.

See [MIGRATION.md](MIGRATION.md) for the exact boundary.

## Security posture

Neurun runs against untrusted websites. Private-network egress is blocked by
default, response bodies are bounded, secrets are not accepted in URLs, API
keys are compared by hash, and project-supplied native plugins are not loaded.
These controls are foundations, not a substitute for host network policy,
container isolation, cgroups, secret management, and the release hardening
called for by the MVP specification.

## License

Apache-2.0. See [LICENSE](LICENSE).
