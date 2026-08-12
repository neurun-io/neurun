# Vector: Transport

Everything the network stack says before a single byte of JavaScript runs. It is
the vector a presentation-only spoof cannot touch, and the reason this
architecture drives a real browser instead of an HTTP client.

## How it is read

| Layer | Signal |
| --- | --- |
| TLS `ClientHello` | JA3 / JA4 — cipher suites, extensions, curves, signature algorithms, GREASE placement, and their **order** |
| ALPN | `h2` vs `http/1.1`, and the order offered |
| HTTP/2 | SETTINGS frame values and order, window size, priority tree, pseudo-header order |
| HTTP/1.1 | header order and casing |
| Headers | `Sec-Fetch-Site/Mode/Dest/User`, `Sec-CH-UA*` completeness, `Accept` string exactness |
| Timing | connection reuse, request pipelining, resource fetch order |

A JA4 fingerprint identifies the *library*, not the machine: Chrome, Firefox,
curl, Go's `net/http` and Python's `requests` are all instantly separable. So the
question a detector asks is simple — **does the TLS fingerprint match the browser
the UA claims?**

## What this suite does

Nothing, deliberately. It drives the real Chromium binary, so the ClientHello,
the HTTP/2 SETTINGS, the header order and the `Sec-Fetch-*` set are Chrome's own
and match the claimed brand by construction.

That property is fragile in exactly one way, covered below.

## The failure mode: re-issuing requests

The older bot architecture intercepts paused requests and, for some of them,
re-issues them from an async HTTP client through the proxy, then fulfils the
browser's request with the result. That is a useful pattern — it routes selected
hosts through a different exit, or caches — and it **breaks this vector for every
request it touches**:

- The TLS fingerprint becomes the HTTP client's, not Chrome's.
- HTTP/2 SETTINGS and header order become the client's.
- Header **casing** changes: many clients normalise, Chrome does not.
- `Sec-CH-UA*` and `Sec-Fetch-*` have to be reconstructed by hand.

The same code shows the mitigation it needed: when the identity is not Chromium
— Safari, or anything on iOS — it **strips the Chromium-only headers** before
re-issuing (`sec-ch-*`, `device-memory`, `dpr`, `ect`, `rtt`, `downlink`,
`viewport-width`, and the rest). Sending client hints from a claimed Safari is a
self-refutation at the header layer, exactly as it is at the JS layer.

**Rule:** route at the browser, not around it. Use `--proxy-server` with a local
overlay that injects upstream credentials, so the browser's own stack makes every
connection. Re-issue a request only when you accept that its transport
fingerprint is no longer Chrome's, and strip the headers that cannot be true.

## How it plays with the rest of the suite

- **← User agent.** The UA is a claim the TLS fingerprint either corroborates or
  refutes. This is the most decisive cross-layer check in the whole suite,
  because presentation spoofing cannot reach it.
- **← Locale and network.** The exit's ASN class is read on the same connection.
  A residential exit with a Go TLS fingerprint is a bot on a residential IP.
- **→ Automation artifacts.** Both are "environment" checks rather than "machine"
  checks, and detectors that run one usually run both.
- **→ Behaviour.** Resource fetch order and connection reuse are behavioural at
  the transport layer: a page whose subresources arrive in an order no real
  renderer produces is visible without reading a single JS value.

## How it gets caught

1. **JA3/JA4 mismatch with the claimed browser** — decisive, and cheap to check.
2. **Header order or casing** that no Chromium build produces.
3. **Missing or malformed `Sec-Fetch-*`** on navigations and subresources.
4. **Client hints sent by a claimed Safari**, or absent from a claimed Chrome.
5. **An `Accept` string** that does not match the browser and resource type.
6. **HTTP/1.1 where the browser would have negotiated h2**, which happens when an
   intermediary or a re-issuing client downgrades.
7. **Requests that never traverse the proxy** — a fallback to direct on proxy
   error leaks the real IP with a friendly log line. Fail the request instead.

## Gaps

- Nothing in this stack verifies its own transport fingerprint. There is no
  preflight that checks JA4 against the claimed brand; `ROADMAP.stealth` lists it
  as a contract the server would need to publish.
- The overlay proxy is HTTP CONNECT-based, so it carries TLS end to end — good —
  but it speaks only `http://` and `https://` upstream.
- WebRTC and DNS egress are not bound to the proxy. See `locale-and-network.md`.
