---
name: automation
description: "Driving a browser session against a hostile site — the SDK session loop over Neurun browser profiles, cookie and DOM-storage capture and restore, CDP input dispatch, human pointer/scroll/typing mechanics, request routing through a proxy, and how a refusal is diagnosed. Use when writing or reviewing an app that opens a browser session, when state is lost or duplicated between runs, when a run starts getting challenged, or when deciding what belongs in the SDK versus the control plane."
user-invocable: true
---

# Automation

The control plane stores profiles and nothing more. **Sessions belong to the
SDK.** `neurun-browser` is a Rust gRPC server on loopback beside your app, not
somewhere the server can reach, so there are no session endpoints to call and no
browser to launch from the dashboard.

The identity gets you past the first request. What follows decides whether you
stay: a session is judged on what it does, how it moves, what it asks for, and
whether it has a past.

## The loop

```
1. GET  /v1/browser-profiles/{id}/state      cookies + storage, in the clear
2. OpenSession  →  127.0.0.1 (gRPC)          identity + that state
                ←  a CDP or BiDi endpoint    which you drive
3. CloseSession →                            the browser hands back what it captured
4. PUT  /v1/browser-profiles/{id}/state      store it
```

Step 4 is what saves. A session abandoned rather than closed leaves the profile
exactly as it was — the safe failure, and sometimes the one you want.

→ `references/session-loop.md` for the scopes, the ordering, and every way it
goes wrong.

## The vectors

| Unit | Reference | Why it matters |
| --- | --- | --- |
| Opening, closing, abandoning; who owns what | `references/session-loop.md` | the boundary the whole design rests on |
| Cookies, per-origin DOM storage, replace semantics | `references/profile-state.md` | where sessions are silently lost or erased |
| CDP key, mouse, wheel and touch dispatch | `references/input-dispatch.md` | the mechanics every behaviour is built from |
| Pointer curves, reading, dwell, idleness | `references/human-behaviour.md` | what a behavioural scorer actually reads |
| Proxy routing, header hygiene, leak prevention | `references/network-and-proxy.md` | one direct request undoes the whole run |
| Challenges, 403/429 cohorts, silent degradation | `references/diagnosing-refusals.md` | naming the layer that broke, not rotating blindly |

## Rules that are not negotiable

- **`PUT .../state` replaces, it does not merge.** The browser returns its whole
  cookie jar, so a cookie missing from the body was deleted.
- **A Firefox session returns an empty state.** Writing it back **erases the
  profile**. Do not PUT after a Firefox session.
- **`GET .../state` takes `browser_profiles:write`, not `:read`.** Reading state
  is exporting live sessions.
- **The gRPC surface on loopback carries no authentication.** That is safe only
  because it is loopback. Exposing that port hands over every logged-in profile.
- **Never fall back to a direct request when the proxy fails.** Fail the request.
- **One profile, one concurrent session.** Both would capture; the last `PUT`
  wins wholesale.

## What belongs where

| Concern | Where |
| --- | --- |
| Which identity to wear | the profile, in the control plane |
| Opening and driving a browser | the SDK, on loopback |
| Cookies and storage between runs | the profile's state, via GET/PUT |
| Retry, backoff, claim, cost | the server. Never the client |
| Behaviour mechanics | the SDK |
| Deciding a run was refused | the execution's record, once the contracts exist |

## Cross-references

- → See the `stealth` skill for the identity itself; `continuity.md` there covers
  the half of state that is a detection signal rather than a mechanism.
- → `docs/browser-profile.md` for the storage boundary and the secrets rules.
- → `frontend/lib/roadmap.ts` (`ROADMAP.stealth`, `ROADMAP.dataHealth`) for the
  contracts that would make refusal diagnosis measurable rather than manual.

## Common mistakes

| Mistake | Fix |
| --- | --- |
| PUT after a Firefox session | It returns empty; the PUT erases the profile |
| Merging captured state into stored state | The capture is authoritative and complete. Replace |
| Expecting storage for an origin you never visited | CDP hands back only what was opened. Visit it, or accept the gap |
| Restoring storage from the wrong origin | You cannot set another origin's storage. Restore visits each in turn |
| Two runs on one profile | Both capture; the last write wins and the account sees overlapping sessions |
| Re-issuing requests from an HTTP client | The TLS and header fingerprint stops being Chrome's |
| Direct fallback on proxy error | An IP leak with a friendly name. Fail the request |
| Retrying after a `407` | Bad credentials do not fix themselves; stop the run |
| Straight-line pointer moves, constant dwell | The cheapest behavioural tell there is |
| Clicking element centres | Real clicks scatter inside the target |
| `sleep(3)` as "reading" | Budget time per pixel of content and spend it in viewport-sized chunks |
| Wheel events on a touch identity | A phone swipes; `setEmitTouchEventsForMouse` translates, pacing does not |
