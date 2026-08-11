# Neurun dashboard

The client for the Neurun control plane. Built against the public `/v1`
API and the published OpenAPI document only — it imports no server package and
infers no internal database state.

Where the current `0.1.0` contract cannot support a page, the route says so and
names the endpoints still required rather than rendering mock data.

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

From the repository root, Docker Compose builds and starts this dashboard, the
control plane, PostgreSQL, and NATS:

```bash
docker compose --env-file .env up --build
```

For host development, `make dev` starts the control plane and this dashboard
together. To run the dashboard alone:

```bash
npm install
cp .env.example .env.local     # point NEURUN_API_BASE_URL at your control plane
npm run dev                    # http://localhost:3001
```

Sign in with an email and password. Create an account first — at `/register`, or
against the control plane directly:

```bash
curl -sS -X POST "$NEURUN_API_BASE_URL/v1/auth/register" \
  -H 'content-type: application/json' \
  -d '{"email":"you@example.com","password":"secret","organization_name":"Acme"}'
```

Passwords are at least 6 characters. Roles are `admin` (all scopes), `operator`
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
  session/            session state and observed server capabilities
  view/               status legend, time, units, redaction, schema-driven forms
  storage/            useSyncExternalStore-backed Web Storage (display prefs only)
components/
  auth/               login screen
  ui/                 shadcn primitives, aligned to the design system
  neurun/             the evidence language: Panel, StatusBadge, CopyId,
                      KeyValue, JsonView, Timestamp
  shell/              top nav, side nav, durability banner, sign-in gate
```

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
- **Query keys are partitioned by session** (project + user ID), and the
  cache is cleared on sign-out, so evidence cannot bleed between users.
- **Sign-in failures are indistinguishable.** An unknown username, a wrong
  password and a disabled account all return the same `invalid_credentials`
  response, and the server spends comparable time on each so timing cannot be
  used to enumerate accounts. Repeated failures are throttled with `429` plus
  `Retry-After`.
- **Unknown enum values render.** A status this build does not recognise appears
  as a neutral badge carrying its raw value. It is never mapped onto success and
  never crashes a route.
- **Polling, not streaming.** The current contract exposes no SSE endpoint.
  Non-terminal resources poll and stop on a terminal state; polling pauses while
  the document is hidden.
- **A missing metric is `—`, never `0`.**

## Testing

```bash
npm test
```

MSW fixtures live in `tests/msw/`.

Covered: sign-in success and failure, session probe and stop-on-401, the client
assembling no credential of its own, the error envelope, cursor pagination
including the empty-string terminator, unknown enums and partial payloads,
status and failure rendering, time and unit handling, secret redaction, and
schema-generated forms.

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
required either an exchange endpoint issuing a short-lived `HttpOnly` session
session, or a BFF holding the key server-side. **The first option now exists**:
the control plane has real accounts and `POST /v1/auth/login` issues the
session. No API key reaches the browser, so that blocker is closed.

Two honest limits remain:

- **Sessions are process-local.** Accounts are durable, but sessions live in the
  server's memory, so a restart signs everyone out and more than one replica
  does not work yet.
- **Registration is open.** `POST /v1/auth/register` creates an account, its
  first project, and a session, and is the only way an account comes into being
  — there is no CLI. Per-IP limiting belongs at the edge; the server holds no
  rate limiter of its own.

A bearer API key is still accepted on every `/v1` route for scripts and CI. When
both a header and a cookie are present, the header wins.
