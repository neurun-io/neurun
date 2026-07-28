# Neurun operator dashboard handoff specification

Status: implementation handoff
Target: Chromium desktop first, responsive down to tablet
Frontend ownership: separate client project
Backend contract: public `/v1` API and published OpenAPI only

## 1. Outcome

Build an operator dashboard that makes every Neurun execution explainable. An
operator should be able to answer:

1. What is running or waiting?
2. What function versions and resources did a run use?
3. Did transport succeed but data validation fail?
4. Which identity, profile, proxy, agent, and browser handled the work?
5. What evidence exists, and what should happen next?

The client must not import any server package or infer internal database state.
For the current foundation, `api/openapi.yaml` is the transport authority,
including the `JobEvent` shape returned by the job-events endpoint.
`contracts/events/job-event.schema.json` mirrors that snapshot for fixture
validation, but it does not imply a streaming endpoint or a separate event
vocabulary.

## 2. Non-goals

- No visual workflow builder.
- No arbitrary JavaScript or native-function editor.
- No CAPTCHA-solving interface.
- No mobile-native application.
- No direct PostgreSQL, NATS, object-store, Prometheus, or agent access.
- No rendering captured HTML in the dashboard's trusted document.
- No frontend-owned retry, lease, cost, or failure-classification logic.

## 3. Technical boundary

Use TypeScript with a component-based web framework. Framework selection is
left to the frontend owner, but the implementation must provide:

- generated API types from `api/openapi.yaml`;
- a single authenticated API client;
- runtime validation at network boundaries for unknown enum values and future
  SSE payloads;
- query caching with explicit invalidation after mutations;
- authenticated `fetch` streaming when SSE contracts are published;
- route-level error boundaries;
- accessible, keyboard-operable components;
- deterministic tests with a mocked HTTP server and future SSE fixtures.

Do not hand-maintain copies of API response interfaces when they can be
generated. UI-only view models are encouraged.

### Current server capability boundary

This document preserves the full dashboard product direction, but the current
`0.1.0` OpenAPI foundation exposes only:

- health, readiness, and version;
- function catalog, definition, version, manifest bundle, and invocation;
- invocation list and detail for both direct and job-owned executions, plus
  direct-invocation cancellation, without a current reverse job/attempt
  filter; job-owned work is canceled through its job endpoint;
- job creation, list, detail, attempts, events, and cancellation;
- synchronous or asynchronous HTTP fetch.

The current all-in-one server uses process-local job, invocation, outbox, and
queue adapters. Asynchronous job creation, asynchronous function invocation,
and asynchronous fetch are disabled unless the operator explicitly sets
`NEURUN_ALLOW_VOLATILE_JOBS=true`. When disabled, those mutations return HTTP
503 with `error.code="durable_backend_unavailable"`; synchronous execution
remains available.

Every accepted asynchronous mutation returns a required `durability` field and
the `Neurun-Job-Durability` response header. The all-in-one server currently
returns `process_local`, which means jobs disappear on process restart. Show a
persistent development warning for that connection and never label
`process_local` work as durable. A 202 response by itself does not imply
durable persistence.

Routes and fields marked **future backend** below remain product requirements,
not claims about the current OpenAPI.

## 4. Authentication

The API uses:

```http
Authorization: Bearer neu_<environment>_<prefix>.<secret>
```

For the local MVP, provide a connection screen that accepts:

- control-plane base URL;
- API key.

The current API key determines the authenticated project. A request
`project_id` must be omitted or match that project; the client must not treat a
locally selected project ID as authority. Project discovery and switching
require the future operator-session and project APIs.

Keep the key in memory by default. An explicit “remember for this browser
session” option may use `sessionStorage`; never use persistent `localStorage`,
IndexedDB, service-worker caches, analytics payloads, URLs, or error-report
breadcrumbs for secrets.

Before a production browser dashboard ships, the backend must add either:

1. an API-key exchange endpoint that creates a short-lived `HttpOnly`,
   `Secure`, `SameSite=Strict` operator session; or
2. a same-origin backend-for-frontend that stores the API key server-side.

Treat that as a production release blocker, not a frontend workaround.

## 5. Information architecture

### Global shell

The shell contains:

- Neurun wordmark and current project indicator;
- active project switcher when the future project-discovery API is available;
- control-plane health and version indicator;
- job and invocation ID navigation on the current foundation;
- global search for job, session, invocation, artifact, and trace IDs when a
  future search contract is available;
- UTC/local-time display preference;
- documentation and API-contract links;
- API connection/account menu;
- persistent side navigation.

Unknown future statuses must render as neutral badges with their raw value.
They must never crash a route or be silently mapped to success.

### Routes

| Route | Purpose | Backend availability |
| --- | --- | --- |
| `/` | Overview | Future dashboard aggregation |
| `/jobs` | Searchable job list | Current foundation |
| `/jobs/:jobId` | Job and attempt detail | Current foundation; richer evidence is future |
| `/sessions` | Browser-session list | Future backend |
| `/sessions/:sessionId` | Live session and resource detail | Future backend |
| `/functions` | Built-in immutable catalog | Current foundation |
| `/functions/:name/:version` | Manifest and invocation detail | Current foundation |
| `/invocations/:invocationId` | Direct or job-owned invocation evidence | Current foundation |
| `/proxies` | Proxy health and target observations | Future backend |
| `/agents` | Agent fleet and capacity | Future backend |
| `/agents/:agentId` | Agent capabilities and recent work | Future backend |
| `/settings/projects` | Project quotas and retention | Future backend |
| `/settings/api-keys` | Key creation and revocation | Future backend |
| `/settings/identities` | Identity versions | Future backend |
| `/settings/profiles` | Profile metadata and controlled import/export | Future backend |
| `/settings/webhooks` | Webhook endpoints and delivery state | Future backend |
| `/settings/audit` | Security and administrative audit events | Future backend |

## 6. Page requirements

### 6.1 Overview

**Future backend.** This page requires the dashboard aggregation endpoint
listed in Section 10. Do not derive fleet-wide rates or costs by downloading
and aggregating unbounded job lists in the browser.

Show a selectable time window and:

- queued, running, retry-wait, failed, rejected, and dead-lettered jobs;
- active sessions;
- healthy/stale/draining agents;
- browser and HTTP slots used versus available;
- transport success, validation rejection, block, crash, and OOM rates;
- accepted record count;
- CPU, browser seconds, transferred bytes, and artifact bytes;
- estimated cost per accepted record;
- recent failures requiring attention.

Every aggregate links to the corresponding filtered list. State the time window,
sample freshness, and whether a metric is unavailable. Do not display missing
data as zero.

### 6.2 Jobs

#### Current foundation

The current job list can show:

- status;
- job ID;
- resolved function name, immutable version, and digest;
- attempt count and maximum attempts;
- created, updated, completed, and canceled times when present;
- next-attempt time;
- failure category;
- process-local durability warning at the connection level.

Current server-side filters are limited to:

- status;
- created-after time;
- opaque cursor and limit.

Do not send `tag`, mode, failure, function, agent, or created-before filters to
the current API. In particular, the current server rejects `tag` rather than
silently ignoring it.

The current job-detail route includes:

1. immutable function input and resolved name, version, and digest;
2. attempts with lease/run timing;
3. current or terminal result;
4. classified failure and persisted retry decision;
5. trace IDs attached to attempts;
6. append-only event stream.

The accepted response normally contains a `queued` job snapshot. Preserve and
render the preceding `job.accepted` and `job.queued` events in order; do not
assume that `accepted` will be observable as a polled job state.

Current actions:

- cancel an active job;
- copy the immutable request after applying client-side secret redaction.

Cancellation requires confirmation and is intrinsically idempotent for an
already-terminal job. Generate and reuse an `Idempotency-Key` for
`POST /v1/jobs` and for `/invoke` or `/fetch` requests with
`execution=async`. Do not require a key for cancellation. A key used after a
network failure must be reused with the byte-equivalent logical request.

Manual job retry is intentionally unavailable in the current release. Display
server-owned `retry_wait`, `next_attempt_at`, failure retryability, and retry
metadata without implementing retry policy in the frontend.

On every 202 response, read `AcceptedJob.durability`. If it is
`process_local`, show that the job can be lost on restart. The current Job
schema does not repeat durability on list or detail responses, so retain the
connection-level warning rather than inventing a per-job guarantee.

#### Future evidence enrichment

When the corresponding schemas and endpoints are published, add:

- optional job names, tags, mode, and workflow summary;
- function-invocation timeline and job/attempt correlation;
- extracted-data preview and provenance;
- validation result and individual rule failures;
- identity, profile, proxy, agent, and browser references;
- usage, estimated cost, artifacts, and configured trace links;
- server-authorized result download;
- confirmed manual retry for an eligible terminal job.

### 6.3 Sessions

**Future backend.** No session endpoint or SSE usage contract exists in the
current OpenAPI.

List:

- status, age, TTL, and reconnect grace;
- assigned agent;
- identity, profile, and proxy;
- current CPU/memory/process/network pressure;
- latest sample age;
- browser version.

Detail:

- connection history;
- signed CDP URL reveal/copy control with an expiry warning;
- live screenshot action;
- save-profile action;
- close action;
- CPU, cumulative CPU seconds, and throttling;
- resident and peak memory with hard limit;
- process count with limit;
- RX/TX totals;
- uptime;
- resource-warning and terminal-event timeline.

Render live usage from authenticated SSE. Routine samples may be visually
coalesced; warnings, limit events, OOMs, crashes, and close events may not be
dropped.

### 6.4 Functions

Catalog filters:

- category;
- capability;
- execution context and side-effect class, which may be applied locally to the
  returned immutable manifests;
- active/deprecated status after the backend publishes lifecycle fields. The
  current `status` query accepts only `available`.

Function detail:

- immutable version and digest;
- description and category;
- input and output JSON Schemas;
- execution-context requirements;
- capabilities;
- default/maximum timeout;
- retry-eligible failures;
- redaction fields;
- telemetry dimensions;
- bundle and schema versions.

Compatibility versions remain a future backend field. Several manifest policy,
retry, redaction, telemetry, artifact, and resource-policy objects are currently
open extension maps in OpenAPI. Generated clients must preserve those values as
unknown records and validate the fields used by the UI at runtime until the
backend publishes fully typed component schemas.

Provide a direct-invocation form generated from the input schema with:

- a raw JSON fallback;
- version pinned by default;
- explicit ephemeral-browser or existing-session choice when those future
  execution contexts become publicly supported;
- timeout bounded by the manifest;
- clear treatment of secret fields;
- sync/async selection when supported.

In the current release, asynchronous execution is available only for manifests
whose execution context is `none` or `http_attempt`. Async requests must omit
`context` and `timeout_ms`; `max_attempts` is between 1 and 10 and defaults to
3 when omitted. Async function invocation and fetch require `jobs:write` in
addition to `functions:invoke`. Disable the async option after a
`durable_backend_unavailable` response, without disabling synchronous
invocation. `http.fetch` is the current generic-invoke exception: public clients
cannot assign its required trusted HTTP context for a synchronous generic
`/invoke`, so synchronous HTTP work must use `POST /v1/fetch`; generic async
invocation is supported. The current fetch form supports only `auto` and
`http` modes; browser mode is future.

After invocation, show output, schema validation, usage, artifacts, trace, and
classified failure without hiding the resolved digest.

### 6.5 Proxies

**Future backend.** No proxy endpoint exists in the current OpenAPI.

Show:

- health and quarantine;
- concurrency used/limit;
- labels and protocol without credentials;
- target-specific latency and outcome rates;
- recent observations;
- test action and result.

Proxy secrets are write-only. Never return, cache, or render credentials.

### 6.6 Agents

**Future backend.** No agent endpoint exists in the current OpenAPI.

Show:

- healthy, stale, or draining state;
- version and labels;
- last heartbeat;
- capabilities;
- installed function-bundle version/digest;
- active and available browser/HTTP slots;
- CPU and memory;
- recent launches, crashes, OOMs, and invocations.

Flag server/agent/function-bundle incompatibility prominently.

### 6.7 Settings

**Future backend.** Project, API-key, identity, profile, webhook, and audit
management endpoints are not part of the current OpenAPI.

Projects:

- name, quotas, domain policy, robots mode, retention, and allowed origins.

API keys:

- name, prefix, scopes, created/last-used/expiry/revoked timestamps;
- show the complete key exactly once at creation;
- revoke with confirmation.

Identities:

- immutable version history and coherence-validation failures.

Profiles:

- metadata and version history only;
- import/export with elevated-scope warning and confirmation.

Webhooks:

- endpoint, subscribed events, enabled state, recent deliveries;
- rotate secret and replay eligible failed delivery.

Audit:

- actor, action, resource, request/trace ID, and time;
- immutable, cursor-paginated display.

## 7. Shared presentation rules

### Statuses

Use stable lowercase API values. Color is supplementary:

- success: `succeeded`;
- informational: `accepted`, `queued`, `ready`;
- active: `leased`, `running`, `connected`, `provisioning`;
- warning: `retry_wait`, `lease_expired`, `disconnected`, `rejected`;
- danger: `failed`, `timed_out`, `dead_lettered`, `crashed`, `oom_killed`;
- neutral: `canceled`, `closed`, `expired`, unknown.

### Time and units

- Parse RFC 3339 timestamps and keep the original instant.
- Default to relative time plus exact UTC in a tooltip.
- Let the operator select UTC or local display.
- Fields named `*_ms` use milliseconds. The current
  `DurableRequest.retry_policy.initial_backoff` and `max_backoff` fields expose
  Go durations encoded as nanoseconds; convert them explicitly in a UI view
  model and do not infer units from an untyped retry payload. A future contract
  should normalize those fields to named millisecond values.
- Bytes use binary units while preserving exact byte values in detail.
- Distinguish rates, gauges, and cumulative counters.

### Errors

Render the standard envelope:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "Human-readable summary",
    "details": {}
  },
  "request_id": "req_..."
}
```

Always expose a copyable request ID. If present, expose the trace ID and
configured trace URL. Current validation failures expose human-readable paths
such as `$.field` inside messages rather than structured JSON Pointer
violations. Preserve and display those messages without brittle parsing. Add
field-level JSON Pointer mapping only after the backend publishes structured
violations.

### Pagination

The current job and invocation lists use resource-specific envelopes:

```json
{
  "jobs": [],
  "next_cursor": ""
}
```

```json
{
  "invocations": [],
  "next_cursor": ""
}
```

An empty string means there is no next page. Function lists, job events, and
job attempts currently return complete named arrays and are not
cursor-paginated. Future large collections should use opaque cursors. Never
parse, sort, or synthesize a non-empty cursor in the browser.

## 8. Live-data behavior

The current OpenAPI exposes no SSE endpoint. Poll non-terminal jobs, their
attempts, and their append-only event snapshots about every two seconds. Pause
nonessential polling while the document is hidden and refresh immediately when
it becomes visible.

The following streaming behavior applies when the future session and event
stream contracts are published. Native `EventSource` cannot attach the bearer
`Authorization` header; use streaming `fetch` with an SSE parser, or consume a
short-lived signed stream URL.

The stream client must:

- send `Last-Event-ID` on reconnect;
- use bounded exponential backoff with jitter;
- honor `Retry-After`;
- stop on 401, 403, 404, session expiry, or explicit close;
- coalesce only routine latest-value samples;
- preserve warning and terminal events;
- surface stale data after three expected sample intervals;
- cancel network work when the route unmounts.

Future session provisioning may poll every second until its live stream is
available.

## 9. Artifact safety

**Future backend.** Apply these rules when artifact metadata and signed
download endpoints are published:

- Screenshots and downloads use short-lived signed URLs.
- Never put signed URLs into analytics or persistent caches.
- Captured HTML must be downloaded or rendered only in a sandboxed iframe with
  scripts disabled and an opaque origin.
- Sanitize extracted strings before display.
- JSON viewers render text nodes, never raw HTML.
- Cap preview size and stream/download large artifacts.
- Mark redacted, expired, missing, and retention-deleted artifacts distinctly.

## 10. API dependencies

### Current supplied public endpoints

- health: `GET /healthz`, `GET /readyz`, and `GET /version`;
- fetch: `POST /v1/fetch`;
- functions: `GET /v1/functions`, `GET /v1/functions/{function_name}`,
  `GET /v1/functions/{function_name}/versions/{version}`,
  `GET /v1/function-manifest-bundle`, and
  `POST /v1/functions/{function_name}/invoke`;
- invocations: `GET /v1/function-invocations`,
  `GET /v1/function-invocations/{invocation_id}`, and
  `POST /v1/function-invocations/{invocation_id}/cancel`;
- jobs: `GET` and `POST /v1/jobs`, `GET /v1/jobs/{job_id}`,
  `GET /v1/jobs/{job_id}/attempts`,
  `GET /v1/jobs/{job_id}/events`, and
  `POST /v1/jobs/{job_id}/cancel`.

There is no current manual job-retry endpoint, invocation-event endpoint,
session API, or public identity, profile, proxy, agent, artifact, webhook,
project, API-key, or audit API.

The current `/version` response reports server version, commit, build time, API
version, schema version, and function-bundle version. The required accepted-job
body reports `durability`, but list and detail Job objects do not repeat it.

### Future backend requirements

The backend must add or extend these contracts before the corresponding pages
are considered complete:

- capability discovery through `GET /version` or a dedicated endpoint,
  including `async_jobs_enabled` and `job_durability`;
- browser-image and compatibility versions in `GET /version`;
- `GET /v1/dashboard/overview?window=&project_id=`;
- optional `POST /v1/jobs/{job_id}/retry` if confirmed manual retry remains a
  product requirement;
- `GET /v1/jobs/{job_id}/attempts/{attempt_id}`;
- job/attempt-to-invocation correlation and invocation event history or stream;
- fully typed manifest retry, redaction, telemetry, artifact, resource-policy,
  and compatibility schemas for generated frontend models;
- session create/list/detail/keepalive/screenshot/save-profile/usage/history,
  event stream, and close endpoints;
- `GET /v1/agents?status=&label=&limit=&cursor=` and
  `GET /v1/agents/{id}`;
- `GET /v1/projects`, `GET /v1/projects/{id}`, and
  `PATCH /v1/projects/{id}`;
- `GET /v1/api-keys`, `POST /v1/api-keys`, and
  `POST /v1/api-keys/{id}/revoke`;
- identity, profile, proxy, artifact, webhook, and audit contracts needed by
  their respective pages, including
  `GET /v1/artifacts?job_id=&attempt_id=&session_id=&limit=&cursor=` and
  `GET /v1/audit-events?type=&created_after=&limit=&cursor=`;
- an authorized cross-resource search contract if global search remains in the
  navigation.

## 11. CORS and browser contract

**Future backend or deployment-layer requirement.** The current server has no
CORS or `OPTIONS` middleware and serves no frontend assets. The current browser
dashboard must therefore use a same-origin reverse proxy/BFF. Before a
separately hosted client is supported, the server or an explicitly managed
gateway must:

- allow only configured dashboard origins;
- allow `Authorization`, `Content-Type`, `Idempotency-Key`, and
  `Last-Event-ID`;
- expose `Request-ID`, `Trace-ID`, `Retry-After`, `Location`,
  `Idempotent-Replayed`, `Neurun-Job-Durability`, and rate-limit headers;
- support `GET`, `POST`, `PATCH`, `DELETE`, and preflight `OPTIONS`;
- never combine wildcard origins with credentials.

Prefer same-origin deployment for the self-hosted default.

## 12. Accessibility

Target WCAG 2.2 AA:

- complete keyboard access and visible focus;
- skip link and semantic landmarks;
- correct heading hierarchy;
- text alternatives for evidence images;
- table headers and accessible names;
- status conveyed through text, icon, and color;
- no required hover interaction;
- reduced-motion support;
- charts with textual summaries and data tables;
- live updates announced selectively, never once per one-second sample.

## 13. Performance

- Route shell interactive within two seconds on a typical operator laptop
  after warm assets.
- Virtualize or paginate long timelines and tables.
- Do not load artifact bodies on list routes.
- Lazy-load JSON viewers, charts, and screenshot tooling.
- Bound cached projects and query history.
- Avoid a React/component render for every one-second usage sample; update a
  bounded time-series buffer.

## 14. Testing and acceptance

Unit tests:

- status and failure rendering;
- units and time handling;
- schema-generated function forms;
- secret redaction;
- mutation idempotency-key reuse;
- process-local durability warnings and async-control gating;
- unknown enums and partial payloads.

Current-foundation integration tests:

- bearer authentication;
- cursor pagination;
- job polling to a terminal state;
- disabled async mutation returning 503 and
  `durable_backend_unavailable`;
- enabled async mutation returning `durability="process_local"` in the body
  and `Neurun-Job-Durability` in the header;
- accepted job fixture containing a `queued` snapshot with ordered
  `job.accepted` and `job.queued` events;
- error-envelope and request-ID display.

Future-backend integration tests:

- operator-session or bearer-stream authentication and reconnect;
- SSE resume with `Last-Event-ID`;
- slow-consumer sample coalescing without warning loss;
- expired signed artifact URL;
- project switching and API-key lifecycle.

Current-foundation end-to-end acceptance:

1. Connect to a local control plane.
2. Discover and invoke `system.echo@1`.
3. With volatile jobs disabled, attempt async submission and keep synchronous
   invocation available after the expected 503 response.
4. With `NEURUN_ALLOW_VOLATILE_JOBS=true`, submit an asynchronous function job,
   show the process-local warning, and watch its timeline complete.
5. Inspect the resolved function version/digest, output, usage, and trace.
6. Cancel an active job with confirmation in a deterministic mocked or
   controlled-duration run.
7. Navigate every current-foundation flow with keyboard only.

Full-product end-to-end acceptance after future backend delivery:

1. Observe a validation rejection distinct from transport failure.
2. Reconnect a live usage stream without losing a warning event.
3. View a screenshot without exposing captured HTML to the trusted DOM.
4. Revoke an API key and stop retrying after the resulting 401.
5. Switch projects without allowing cross-project data to remain cached.

## 15. Handoff deliverables

The frontend owner should deliver:

- application source in its own repository or isolated client directory;
- user-code delivery based on `templates/user-app` and the SHA-pinned reusable
  `.github/workflows/user-app-ci-cd.yml`, including the required Dockerfile and
  repository-owned `scripts/ci/test`, `build`, `smoke`, and `deploy` scripts;
- generated client and reproducible generation command;
- a caller-owned CI check that regenerates the OpenAPI client and fails on
  uncommitted contract drift;
- environment and same-origin deployment instructions;
- component inventory and design tokens;
- mocked API/SSE fixtures;
- unit, integration, accessibility, and E2E suites;
- production auth decision record;
- CSP and artifact-sandbox configuration;
- a backend-gap checklist tied to the endpoint list above.
