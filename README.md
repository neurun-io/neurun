# Neurun

Neurun is an execution and evidence plane for reliable HTTP and Chromium
scraping workflows. This repository contains the server-side control API,
worker runtime, contracts, migrations, deployment assets, and the operator
dashboard and public site in `frontend/`.

**Source-available to nobody: this repository is proprietary and not
distributed.** The product is commercial, priced per compute, with no free tier
beyond a 14-day trial. Enterprise customers receive a licensed container to run
in their own network; source code is not part of that.

> The `LICENSE` file still carries Apache 2.0 from the open-source period. It
> contradicts the paragraph above and needs replacing before anything ships
> publicly.

## Current foundation

The deployable vertical slice is the deployment path, end to end:

- projects and apps, with cascading deletes that require a typed confirmation;
- source deployments taken as a ZIP, stored immutably, and built synchronously;
- numbered, immutable builds producing code and install layers as artifacts;
- executions pinned to the build that was ready when they were created, claimed
  with `FOR UPDATE SKIP LOCKED` and finalized by compare-and-set;
- rerun against the exact original build, refused when it is no longer ready;
- accounts from open registration, scoped API keys, and role-derived scopes;
- an OpenAPI contract the dashboard's client is generated from.

The commercial model follows the execution: an app is **executed, not hosted**,
so the meter is the compute an execution consumes between `started_at` and
`finished_at`. Queued time and builds are free. **Runners** — a server holding
one app resident behind an endpoint, metered by resident time — are the next
capability and are not built; see [docs/runner.md](docs/runner.md).

**Browser profiles** are the newest capability: an optional stealth identity plus
the cookies and storage a browser keeps between runs. The control plane stores
them and nothing more — sessions belong to the SDK, which reads a profile,
launches Chrome or Firefox through `neurun-browser` (a Rust gRPC server on
loopback beside it), drives the returned CDP or BiDi endpoint, and PUTs the
captured state back. See [docs/browser-profile.md](docs/browser-profile.md).

AI stealth coherence, an AI automation builder, proxies, fleet aggregation,
webhooks, an activity log and data health remain later milestones. The
dashboard's roadmap routes name the contracts they still need rather than
rendering placeholder data, and the public site's capability matrix marks the
same rows rather than selling them.

## Quick start with Docker

Requirements: Docker Engine or Docker Desktop with Compose v2.

```sh
cp .env.example .env
docker compose --env-file .env up --build -d
docker compose ps
```

One command starts the operator dashboard, control plane, PostgreSQL, and NATS:

- dashboard: `http://localhost:3001`
- API: `http://localhost:1267`

PostgreSQL, JetStream, and artifact payloads persist in Docker-managed local
volumes. Artifact payloads use the filesystem-backed `neurun-data` volume;
there is no MinIO or other object-storage service in the local stack.

Check the API is up — this needs no credential:

```sh
curl http://localhost:1267/healthz
```

Everything under `/v1` requires either a session cookie or a bearer API key.
There is no preinstalled key: register an account, which signs you in, then
issue keys through `POST /v1/api-keys`. See [First account](#first-account).

```sh
docker compose logs -f
docker compose down       # preserve local data
docker compose down -v    # also delete all local volumes
```

The current MVP still uses process-local job, invocation, and queue adapters;
PostgreSQL and JetStream are started so the complete dependency stack is
available, but durable adapters remain a later milestone.

### Host development

Requirements: Go 1.25 or newer, Node 20.9 or newer.

```sh
make dev
```

This builds and starts the control plane on `:1267`, waits for `/healthz`, then
starts the dashboard on `:3001`. Ctrl-C stops both. The dashboard calls the
control plane directly; the server names the dashboard's origin in
`NEURUN_ALLOWED_ORIGINS` and the browser is told where to look with
`NEXT_PUBLIC_NEURUN_API_URL`.

To run only the development binary, export
`NEURUN_TRUSTED_CODE_EXECUTION=true` before `go run ./cmd/neurun`.

### First account

There are no credentials in configuration, and the server creates nothing on
boot — no account, no project, no API key. Accounts come from registration, and
from nothing else:

```sh
curl -sS -X POST http://localhost:1267/v1/auth/register \
  -d '{"username":"ada","password":"a-long-dev-password"}'
```

Or open the dashboard and use the create-account form, which posts the same
request. Registration creates the account as an `admin`, creates the project it
names, and signs it in by setting the session cookie — so a fresh install goes
from nothing to a usable dashboard in one request.

Sign-up is open, so **per-IP limiting belongs at the edge**. The server holds no
rate limiter of its own, matching the sign-in throttle that was removed for the
same reason.

Roles are `admin` (all scopes), `operator` (read plus deploy and execute), and
`viewer` (read only). Later accounts are invited rather than registered, through
`POST /v1/users`, which takes a role and needs `users:write`.

Everything after that is ordinary API work: create apps and API keys through the
endpoints. A key is granted scopes explicitly and can never be granted a scope
the caller does not already hold.

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
frontend/            operator dashboard (Next.js, generated OpenAPI client)
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
keys are compared by hash, operator passwords are stored only as PBKDF2 hashes
with throttled sign-in, session tokens are stored only as digests behind
HttpOnly/Secure/SameSite=Strict cookies, and project-supplied native plugins are
not loaded.
These controls are foundations, not a substitute for host network policy,
container isolation, cgroups, secret management, and the release hardening
called for by the MVP specification.

## License

Apache-2.0. See [LICENSE](LICENSE).
