# Browser profile

Who a browser appears to be, and what it remembers between sessions.

## Two halves

**Presentation** — the `identity`: user agent inputs, screen metrics, timezone,
locale, GPU strings, proxy. It is optional. A profile without one launches the
browser as itself, which is the ordinary case and still keeps its state.

**State** — cookies, localStorage and sessionStorage. This is what makes a
profile worth having: a run that signed in yesterday is still signed in today.

## Organization-scoped

Not project-scoped, unlike apps and deployments. The account that owns a
logged-in state owns it everywhere, and splitting one Amazon login across four
projects would mean four separate sign-ins to keep alive.

## The control plane brokers the browser

`neurun-browser` is a separate Rust gRPC server. Under Neurun it is **spawned
and owned by the control plane**, one per host on loopback, and the SDK never
reaches it: an SDK asks Neurun for a session and drives it by id. See
[browser session](browser-session.md) for that half.

A profile is what a session wears. This document is about the wardrobe, not the
run.

The loop, with Neurun in the middle:

1. `OpenSession{browser, browser_profile_id}` — the profile's identity and
   stored state are applied when the browser launches.
2. `Execute` — commands, relayed.
3. `CloseSession` — the browser hands back what it captured, and the profile's
   state is written from it.

A session abandoned rather than closed leaves the profile exactly as it was,
which is the safe failure and sometimes the one you want.

State **replaces**, it does not merge. The browser returns its whole cookie jar,
so a cookie missing from the capture was deleted — merging would resurrect a
login the site had already ended.

Neither the browser service's port nor a session's display is ever reachable
from outside the host. The authenticated boundary is the Neurun API, and that
has not changed: what changed is that Neurun is now on both sides of it.

> **Later.** Once the central cache is integrated the SDK should upload captured
> state to the cache instead, and the control plane read it from there — which
> removes the state payload from the request path entirely.

## Secrets

Cookie values and the proxy URL are credentials. `GET /v1/browser-profiles` and
`GET /v1/browser-profiles/{id}` return neither: a cookie comes back as its name,
domain, path, flags and `value_size`, and the identity reports `proxy_set`
rather than the URL.

`GET /v1/browser-profiles/{id}/state` returns the values, and takes
`browser_profiles:write` rather than `:read` — reading state is exporting live
sessions. It is a response body rather than anything in a URL, so it stays
inside TLS and out of access logs, browser history and `Referer`.

## What Firefox does not do

Firefox launches and returns a BiDi endpoint. It carries **no profile**:
rustenium-identity drives Chrome over CDP only, and rustenium exposes no BiDi
storage API to move cookies with. `CloseSession` returns an empty state for it.

That empty state is a footgun the server cannot catch. `PUT .../state` replaces,
and an empty body is indistinguishable from a browser that genuinely holds no
cookies — so an SDK that closes a Firefox session and PUTs the result **will
erase the profile**. Until Firefox carries state, the SDK must not write back
after a Firefox session.

## Storage capture is per-origin

DOM storage is partitioned by origin and CDP will not hand over an area the page
never opened, so a close captures the storage of the page that was open.
Restoring visits each origin in turn before the caller is given the session.

Cookies have no such limit — they are read browser-wide.

## Scopes

`browser_profiles:read` for list and detail. `browser_profiles:write` for
create, update, delete, and both reading and writing state.

## Fields

`id`, `name`, `browser` (`chrome` or `firefox`), `identity` (nullable),
`cookies`, `storage_origins`, `created_at`, `updated_at`.
