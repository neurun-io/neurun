# Unit: The Session Loop

Who opens a browser, who stores what, and what each step is allowed to assume.
The boundary is unusual and load-bearing: **the control plane never opens a
browser**, so there is no session resource, no keepalive, and nothing to poll.

## The four steps

### 1. Read the state

```
GET /v1/browser-profiles/{id}/state        scope: browser_profiles:write
```

Returns cookie **values** and storage contents in the clear. It takes the write
scope rather than the read scope because exporting a profile's state is exporting
live credentials — a read-scoped key can list profiles and see cookie names,
domains, flags and value sizes, but never the values.

The response body is the only place these travel. Never a URL, never a query
parameter, never a log line: a body stays inside TLS and out of access logs,
browser history and `Referer`.

### 2. Open the session

`OpenSession` over gRPC to `127.0.0.1`, carrying the identity and that state.
Back comes a CDP or BiDi endpoint.

The browser server is a separate process on loopback **beside the SDK**. The
control plane holds no connection to it and has no session endpoints. That is
why:

- there is nothing to launch from the dashboard, and the UI says so;
- the gRPC surface carries no authentication, which is safe only on loopback;
- exposing that port on a routable interface hands over every logged-in profile
  the machine can reach.

### 3. Drive it

CDP for Chrome, BiDi for Firefox. Everything in `input-dispatch.md` and
`human-behaviour.md` happens here.

### 4. Close and store

`CloseSession` returns what the browser captured;
`PUT /v1/browser-profiles/{id}/state` stores it.

**Closing is what saves.** Abandoning the session — crash, timeout, deliberate
discard — leaves the profile exactly as it was. That is a feature: a run that
corrupted a login can be thrown away by not writing back.

## Failure modes

| Situation | What happens | What to do |
| --- | --- | --- |
| Session abandoned | profile unchanged | nothing; this is the safe path |
| Firefox session closed | capture is **empty** | do not PUT — see `profile-state.md` |
| PUT with a partial jar | the missing cookies are deleted | send the whole capture, never a subset |
| Two sessions on one profile | both capture; last PUT wins | serialise runs per profile |
| Proxy down at launch | launch fails during timezone resolution, before navigation | set the timezone explicitly to remove the dependency |
| `407` from the proxy | credentials are wrong | stop the run; do not retry, do not fall back |
| Browser server unreachable | no session | it is a local process; check it is running, not the network |

## What the control plane will and will not do

**Will:** store profiles, redact secrets on read, replace state on write, cascade
a delete to the profile's state, scope everything to the organization.

**Will not:** open a browser, hold a session, keep state warm, retry anything, or
know that a run happened. A profile has no link to an execution; nothing
correlates them today.

## Scopes

| Endpoint | Scope |
| --- | --- |
| `GET /v1/browser-profiles`, `GET /v1/browser-profiles/{id}` | `browser_profiles:read` |
| `GET /v1/identity-catalog` | `browser_profiles:read` |
| `POST`, `PATCH`, `DELETE /v1/browser-profiles/{id}` | `browser_profiles:write` |
| `GET` and `PUT /v1/browser-profiles/{id}/state` | `browser_profiles:write` |

A key can never be granted a scope its creator does not hold, so a read-only
automation key cannot be talked into exporting state.

## Organization scope, not project scope

Profiles belong to the organization. The account that owns a logged-in state owns
it everywhere, and splitting one login across four projects would mean four
separate sign-ins to keep alive. This has a practical consequence for
concurrency: two projects can reach the same profile, so "one session per
profile" is an organization-wide rule, not a per-app one.

## Where this is going

The state payload currently travels through the control plane on every open and
close. Once the central cache is integrated, the SDK uploads captured state to
the cache and the control plane reads it from there — removing the payload from
the request path entirely. Written contracts for that do not exist yet; the note
in `docs/browser-profile.md` is the current statement of intent.
