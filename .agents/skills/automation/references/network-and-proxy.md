# Unit: Network and Proxy

Where the run's packets come from, and what they say. One request that escapes
the tunnel undoes every other layer, so the rules here are absolute rather than
probabilistic.

## Route at the browser, not around it

The identity's proxy is applied as `--proxy-server=http://127.0.0.1:{port}`,
pointing at a **local overlay proxy** that rustenium-identity starts on
`127.0.0.1:0`. The overlay parses `user:pass@host:port` out of the identity's
proxy URL and injects `Proxy-Authorization` upstream, because Chrome cannot carry
credentials on the flag itself.

Consequences worth knowing:

- Every connection the browser makes uses the exit — including ones you did not
  think about (favicons, prefetches, telemetry, OCSP).
- The browser's own TLS stack makes those connections, so the JA3/JA4 fingerprint
  stays Chrome's. → `stealth` skill, `references/transport.md`.
- The overlay speaks `http://` and `https://` only. A `socks5://` URL is rejected
  at launch, despite appearing in the crate's own README example.
- If the identity has no explicit timezone, the crate makes a real request to
  `ip-api.com` **through the proxy** before launch, so a dead proxy fails the
  launch rather than the first navigation.

## Interception, and its cost

Pausing requests with `Fetch.requestPaused` and either continuing or fulfilling
them is how per-host routing and caching get built. It is also how the transport
vector gets lost:

**`continueRequest`** — the browser makes the request. Fingerprint intact. Use
this for everything you are not deliberately rerouting.

**`fulfillRequest` after re-issuing from your own HTTP client** — you make the
request. The TLS fingerprint, the HTTP/2 SETTINGS, the header order and the
header casing all become your client's. For a claimed Chrome, that is a
contradiction on every request you touch.

If you must re-issue:

- **Strip the Chromium-only headers when the identity is not Chromium.** Safari
  and every iOS browser send no `sec-ch-*`, no `device-memory`, `dpr`, `ect`,
  `rtt`, `downlink` or `viewport-width`. Forwarding them from a claimed Safari is
  a self-refutation at the header layer, exactly as it is in JS.
- Preserve the order and casing you were given, rather than letting the client
  normalise them.
- Forward the method, body and `allow_redirects: false`, and hand the browser the
  real status, headers and body so the page behaves normally.

## Failure handling — the leak rules

| Event | Correct response |
| --- | --- |
| `407 Proxy Authentication Required` | **Stop the run.** Credentials are wrong; retrying burns the account and the next fallback leaks the IP |
| Proxy connection error | Retry once through the proxy, then **fail the request** |
| SSL error through the proxy | Fail the request |
| Any of the above | **Never fall back to direct.** "It will just go direct" is an IP leak with a friendly name |

Failing a request is visible to the page — an image does not load, a script 404s
— and that is the correct trade. A page rendering imperfectly is recoverable; the
real IP arriving at the target is not.

## What the proxy does not cover

- **WebRTC.** ICE candidate gathering happens over UDP and does not traverse an
  HTTP proxy. If the page reads candidates, it can learn the host's real address.
  Nothing in this stack disables it — do it at launch if the target looks.
- **DNS.** Resolution follows the host unless the proxy is a full tunnel, so the
  resolver's location can disagree with the exit's.
- **Anything outside the browser.** Your app's own HTTP calls — including calls to
  the Neurun API — do not go through the identity's proxy, and should not.

## Referrer and arrival

A session that always arrives directly at a deep URL has no story. The working
implementation injects a stored referrer into the first request of a session and
then stops, which is enough to make the arrival plausible without claiming a
referrer on every subsequent request.

## Accounting

Bandwidth through a residential or mobile proxy is the expensive part of a run.
Worth tracking per session, separately: request bytes, uncached response bytes,
proxied response bytes, cached response bytes. That breakdown is what tells you a
run got expensive because it stopped hitting cache, rather than because it did
more work.

Blocking heavy resource types is the usual lever, and it is a trade: a browser
that never loads images or fonts is cheap, faster, and behaviourally distinct
from a real one.

## Checklist

1. Proxy applied at the browser, with credentials injected by the overlay?
2. Everything not deliberately rerouted going through `continueRequest`?
3. Chromium-only headers stripped when the identity is not Chromium?
4. `407` stopping the run rather than retrying?
5. No direct fallback anywhere in the error paths?
6. WebRTC considered, and disabled if the target reads it?
7. Bandwidth accounted for, if the exit is metered?
