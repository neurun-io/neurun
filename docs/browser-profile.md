# Browser profile

Who a browser appears to be, and what it remembers between sessions.

## Two halves

**Presentation** — the `identity`: browser, user agent inputs, screen metrics,
timezone, locale, GPU strings, proxy. Every profile has one. Created without
one, a profile is given a coherent machine drawn from the catalogue and seeded
by its id, so it holds still across runs and differs from the next profile's.

**State** — cookies, localStorage and sessionStorage. A run that signed in
yesterday is still signed in today.

## One browser field

`chrome` or `safari`, on the identity. Chrome is the only browser that launches
— rustenium-identity drives it over CDP and nothing else — so a Safari profile
is a Chrome wearing Safari. The profile's `browser` repeats `identity.browser`
for list views; nothing else sets it.

## Organization-scoped

Not project-scoped, unlike apps and deployments. The account that owns a
logged-in state owns it everywhere.

## The control plane brokers the browser

`neurun-browser` is a separate Rust gRPC server, spawned and owned by the
control plane, one per host on loopback. The SDK never reaches it: it asks
Neurun for a session and drives it by id. See
[browser session](browser-session.md) for that half.

1. `OpenSession{browser, browser_profile_id}` — the control plane resolves the
   identity and sends it with the request, because the browser service has no
   database. No profile named means an ephemeral identity for that session, and
   `browser` is read only then.
2. `Navigate`, `WaitForNavigation` — one RPC per command, shaped after the
   browser's own function, relayed. The set grows as the browser implements
   more.
3. `CloseSession` — the browser hands back what it captured, and the profile's
   state is written from it.

A session abandoned rather than closed leaves the profile as it was.

State **replaces**, it does not merge. The browser returns its whole cookie jar,
so a cookie missing from the capture was deleted.

> **Later.** Once the central cache is integrated the SDK should upload captured
> state to the cache instead, and the control plane read it from there.

## Secrets

Cookie values and the proxy URL are credentials. List and detail return neither:
a cookie comes back as its name, domain, path, flags and `value_size`, and the
identity reports `proxy_set` rather than the URL.

`GET /v1/browser-profiles/{id}/state` returns the values, and takes
`browser_profiles:write` rather than `:read` — reading state is exporting live
sessions.

## Storage capture is per-origin

CDP will not hand over a storage area the page never opened, so a close captures
the storage of the page that was open, and restoring visits each origin in turn
before the caller is given the session. Cookies are read browser-wide.

## Scopes

`browser_profiles:read` for list and detail. `browser_profiles:write` for
create, update, delete, and both reading and writing state.

## Fields

`id`, `name`, `browser`, `identity`, `cookies`, `storage_origins`,
`created_at`, `updated_at`.
