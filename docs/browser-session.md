# Browser session

One browser a handler has open, while it is open.

A session is **live state, not a record**. It lives in the cache under a lease,
so a worker that dies takes its sessions with it rather than leaving rows that
claim to be running. Nothing reads a closed session, so none is kept. Anything
worth keeping afterwards belongs on the [execution](execution.md).

It is not a [browser profile](browser-profile.md). A profile is who a browser
appears to be and what it remembers between runs; a session is a browser that is
running now. A session names a profile only so you can tell which one it is
wearing, and it may wear none.

## Neurun is the broker

```
SDK ──gRPC──▶ control plane ──gRPC──▶ neurun-browser
                    ▲
   dashboard ──WS───┘
```

The SDK talks to Neurun and to nothing else. It does not know that a browser
service exists, where it listens, or that it was spawned for this host — and it
cannot be told. It asks for a session, gets an id, and drives that id.

That is the whole reason the dashboard can list a session and stream its
display: nothing is happening on a port only the tenant's code knows about.

## Two sides, two credentials

| Who | Reaches | Credential |
| --- | --- | --- |
| A handler, opening and driving its own sessions | the loopback gRPC port | its execution token |
| An operator, listing sessions and watching a display | `/v1/browser-sessions…` | session cookie or API key, `browser_sessions:read` |

The handler has no API key — it is code the control plane started, not a client
that signed in. The operator has no execution token. Neither can use the
other's door.

## What the SDK does

The worker puts two variables in the handler's environment:

| Variable | Meaning |
| --- | --- |
| `NEURUN_GRPC_ADDRESS` | `127.0.0.1:<port>`, the control plane |
| `NEURUN_EXECUTION_TOKEN` | proves the caller is this execution |

That is the entire environment. There is no app id, because **an app id in an
environment variable is a claim, not a credential** — the process holding it is
the tenant's own code and could change it. The token is the one thing it holds
that we minted; the organization, the app and the execution all live on our side
of the lookup. It is issued when the run starts, spent when it ends, and stored
only as its digest.

The loop:

```
1. read NEURUN_GRPC_ADDRESS and NEURUN_EXECUTION_TOKEN
2. OpenSession{browser, browser_profile_id?}   → session id
3. Execute{session_id, command}                 as many times as needed
4. CloseSession{session_id}                     including on failure
```

Every call carries the token in `neurun-execution-token` metadata.

There is no heartbeat. **Driving a session renews its lease**, because a browser
being commanded is a browser that is alive — a session left idle past the lease
leaves the list, and one being used never does.

`Execute` carries an opaque command. The control plane brokers sessions, not
browser semantics: it never parses one, so the browser service's command set can
grow without a release here.

## What an operator can do

- `GET /v1/browser-sessions` — the organization's live sessions, newest first.
- `GET /v1/browser-sessions/{id}` — one session.
- `DELETE /v1/browser-sessions/{id}` — forget it.
- `GET /v1/browser-sessions/{id}/display` — a WebSocket carrying RFB.

A display is a signed-in browser rendered as pixels, which is more than any
profile endpoint returns, so it takes its own scope. The credential is checked
**before the handshake completes** — a refused viewer never sees a frame, and an
unreachable service is a `502` rather than a socket that opens and dies.

No address appears in any response. A viewer names a session and asks the
control plane to stream it.

## Why WebSocket to the browser, gRPC to the service

noVNC connects to a WebSocket natively, so that is what the dashboard gets.
gRPC-Web would mean a proxy, a client library, and unwrapping RFB in the browser
to hand it to noVNC anyway — with no bidirectional streaming, which would close
the door on an interactive display later.

Between the control plane and the browser service both ends are ours, so that
half is `StreamDisplay`, a bidirectional stream of opaque RFB chunks. The
session travels once in `neurun-session-id` metadata rather than on every frame.

## The browser service

One per host, spawned lazily on the first `OpenSession` rather than at boot, on
a port the kernel chose — two planes on a machine never fight over a number. It
binds loopback. Sessions do not outlive it, which is correct: the browsers were
its children.

It must serve `neurun.browser.v1.BrowserService` — `Open`, `Run`, `Close`,
`StreamDisplay` — from [`proto/browser.proto`](../proto/browser.proto).
The session id in a `Session` is **the one the service minted**; the control
plane adopts it rather than assigning its own, so both sides call a session by
the same name.

`NEURUN_BROWSER_SERVICE` is the path to its executable. Without it every browser
call refuses rather than failing halfway through opening one.

## Status

Built: the session record and its cache repository, the loopback gRPC server,
execution tokens, the supervisor, the relay, the operator endpoints, the display
bridge, and the dashboard's list and detail pages.

Not built: `neurun-browser` implementing `BrowserService`, and the SDK.

The gRPC listener carries no TLS and the control plane dials the browser service
insecurely. Both are correct only because neither leaves the host, and both stop
being correct the moment workers are separate machines — the same cliff the
artifact store crosses.
