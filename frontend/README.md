# Neurun operator dashboard

The operator client for the Neurun control plane. Built against the public `/v1`
API and the published OpenAPI document only — it imports no server package and
infers no internal database state.

Implements `FRONTEND_SPEC.md`. Where the current `0.1.0` contract cannot support
a page, the route says so and names the endpoints still required rather than
rendering mock data.

## Stack

| Concern | Choice |
| --- | --- |
| Framework | Next.js 16 (App Router), React 19 |
| Styling | Tailwind CSS v4 + shadcn/ui, driven by the Neurun design tokens |
| Data | TanStack Query v5 |
| API types | `openapi-typescript`, generated from `../api/openapi.yaml` |
| Boundary validation | Zod, with open enums and preserved unknown keys |
| Tests | Vitest + MSW + Testing Library |

## Running it

From the repository root, `make dev` starts the control plane and this dashboard
together. To run the dashboard alone:

```bash
npm install
cp .env.example .env.local     # point NEURUN_API_BASE_URL at your control plane
npm run dev                    # http://localhost:3000
```

Sign in with an operator username and password. Create an account first, from the
repository root:

```bash
scripts/create-operator.sh admin admin
```

Passwords are at least 12 characters. Roles are `admin` (all scopes), `operator`
(read plus submit and cancel), and `viewer` (read only) — a viewer cannot start
or cancel execution, enforced server-side by scope.

### Scripts

| Script | Purpose |
| --- | --- |
| `npm run dev` / `build` / `start` | Next.js |
| `npm run typecheck` | `tsc --noEmit` |
| `npm run lint` | ESLint |
| `npm test` | Vitest suite |
| `npm run generate:api` | Regenerate `lib/api/schema.ts` from the OpenAPI document |
| `npm run check:api-drift` | Regenerate and fail on uncommitted contract drift — wire this into CI |

`lib/api/schema.ts` is generated. Never hand-edit it, and never hand-maintain a
copy of a response interface that can be generated. UI-only view models belong
in `lib/view/`.

## Architecture

```
app/
  (dashboard)/        routes behind the sign-in gate, with a segment error boundary
  api/proxy/[...path] same-origin proxy — the only thing that talks to the control plane
lib/
  api/                generated types, the single client, zod boundaries, query hooks
  session/            operator session state and observed server capabilities
  view/               status legend, time, units, redaction, schema-driven forms
  storage/            useSyncExternalStore-backed Web Storage (display prefs only)
components/
  auth/               login screen
  ui/                 shadcn primitives, aligned to the design system
  neurun/             the evidence language: Panel, StatusBadge, StateFlow, CopyId,
                      KeyValue, JsonView, EventTimeline, Timestamp
  shell/              top nav, side nav, durability banner, sign-in gate
```

### Why the proxy exists

The control plane ships no CORS or `OPTIONS` middleware and serves no frontend
assets, so a browser cannot call `/v1` cross-origin. Every request goes to
`/api/proxy/*` on this origin, which forwards it, including the session cookie in
both directions.

The upstream target comes only from `NEURUN_API_BASE_URL` on the server. The
browser cannot choose it, which is what stops the handler being an open relay for
arbitrary outbound requests.

### Design system

Neurun tokens are ported into `app/globals.css` as Tailwind v4 theme variables
and shadcn's semantic layer is resolved onto them, so the shadcn primitives
inherit the system rather than fighting it.

- Namespaced `--nr-*` because three design-system roles (`--accent`,
  `--secondary`, `--muted`) collide with shadcn's own variable names.
- Strictly monochrome. `--destructive` is the inverted accent, not red: danger is
  carried by inversion, glyph and copy.
- Dark is the default; light is a full complementary palette. The theme is
  attribute-driven, so the `dark:`/`light:` variants follow `data-theme` rather
  than the OS media query.
- Status is never colour alone — see `lib/view/status.ts` for the legend and
  `components/neurun/status-badge.tsx` for the treatments.

## Behaviour worth knowing

- **The browser holds no readable credential.** Authentication is an `HttpOnly`,
  `Secure`, `SameSite=Strict` session cookie issued by `POST /v1/auth/login`.
  There is no API key in any client module, and nothing is written to
  `sessionStorage`, `localStorage`, IndexedDB, a URL, or an error breadcrumb.
- **Query keys are partitioned by session** (project + operator ID), and the
  cache is cleared on sign-out, so evidence cannot bleed between operators.
- **Sign-in failures are indistinguishable.** An unknown username, a wrong
  password and a disabled account all return the same `invalid_credentials`
  response, and the server spends comparable time on each so timing cannot be
  used to enumerate accounts. Repeated failures are throttled with `429` plus
  `Retry-After`.
- **Unknown enum values render.** A status this build does not recognise appears
  as a neutral badge carrying its raw value. It is never mapped onto success and
  never crashes a route.
- **Polling, not streaming.** The current contract exposes no SSE endpoint.
  Non-terminal jobs, attempts and events poll every 2s and stop on a terminal
  state; polling pauses while the document is hidden.
- **Durability is tracked per session.** `AcceptedJob.durability` and the
  `Neurun-Job-Durability` header are recorded on every accepted async mutation.
  `process_local` raises a standing banner. The Job schema does not repeat
  durability on list or detail responses, so no per-job guarantee is invented.
- **Async gating.** A `durable_backend_unavailable` 503 disables the async
  control and leaves synchronous invocation working.
- **Idempotency keys are reused across retries.** A key is memoised against a
  stable fingerprint of the logical request and released only on a decisive
  outcome — never after a transport failure, when the server may already hold
  the request.
- **Nanoseconds are converted explicitly.** `retry_policy.initial_backoff` and
  `max_backoff` are Go durations in nanoseconds; `formatNanoseconds` is separate
  from `formatDurationMs` so the conversion is visible at every call site.
- **A missing metric is `—`, never `0`.**

## Testing

```bash
npm test
```

MSW fixtures live in `tests/msw/`. The load-bearing one is `acceptedJob`: the
accepted response carries a `queued` snapshot while the event stream holds both
`job.accepted` and `job.queued` in sequence order — `accepted` is normally never
observable as a polled state, and that asymmetry has to survive.

Covered: sign-in success and failure, session probe and stop-on-401, the client
assembling no credential of its own, error envelope and request-ID display,
cursor pagination including the empty-string terminator, durability body/header,
`durable_backend_unavailable` gating, idempotency-key reuse, unknown enums and
partial payloads, status and failure rendering, time/unit/nanosecond handling,
secret redaction, and schema-generated forms.

## Backend gaps

`lib/roadmap.ts` is the checklist, and it is also what the roadmap routes
render. Delete an entry when its contract ships.

Not yet available: dashboard aggregation, capability discovery
(`async_jobs_enabled`, `job_durability`), job/attempt→invocation correlation,
manual job retry, sessions and their event stream, proxies, agents, artifacts,
projects, API keys, identities, profiles, webhooks, audit, and cross-resource
search.

## Authentication

Spec §4 listed the API key in the browser as a production release blocker, and
required either an exchange endpoint issuing a short-lived `HttpOnly` operator
session, or a BFF holding the key server-side. **The first option now exists**:
the control plane has real operator accounts and `POST /v1/auth/login` issues the
session. No API key reaches the browser, so that blocker is closed.

Two honest limits remain:

- **Sessions are process-local.** Accounts come from `NEURUN_OPERATOR_ACCOUNTS`
  and sessions live in the server's memory, so a restart signs everyone out.
  `migrations/000001_core.sql` defines the durable `operators` and
  `operator_sessions` tables for when the PostgreSQL adapter lands.
- **Passwords are PBKDF2-HMAC-SHA256**, not Argon2id, to keep the Go module free
  of third-party dependencies. The iteration count is OWASP's current floor and
  the encoded hash is self-describing, so raising the cost later does not
  invalidate existing accounts.

A bearer API key is still accepted on every `/v1` route for scripts and CI. When
both a header and a cookie are present, the header wins.
