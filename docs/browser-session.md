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
| An operator, opening and driving sessions over HTTP | `/v1/browser-sessions…` | session cookie or API key, `browser_sessions:write` |
| An operator, listing sessions and watching a display | `/v1/browser-sessions…` | session cookie or API key, `browser_sessions:read` |

The handler has no API key — it is code the control plane started, not a client
that signed in. The operator has no execution token. Neither can use the
other's door.

**Both doors open onto the same driver.** `service.BrowserDriverService` is the
only thing that knows a browser service exists; the gRPC broker and the HTTP
handlers each resolve who is asking and then call it. A session is the same
thing whichever asked for it, appears in the same list, and is leased the same
way. What differs is what a session belongs to: one an execution opened names
its app and its run, and one an API key opened names neither — an API key is
not a run, and writing down an app it does not have would only be a lie in the
record.

Reading is not driving. `browser_sessions:read` lists sessions and watches a
display; opening one, moving its pointer, typing into it or reading its cookies
takes `browser_sessions:write`. Cookies sit on the write side for the same
reason a profile's state does: the response is live credentials.

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
3. any command below{session_id, …}            as many times as needed
4. CloseSession{session_id}                     including on failure
```

Every call carries the token in `neurun-execution-token` metadata.

There is no heartbeat. **Driving a session renews its lease**, because a browser
being commanded is a browser that is alive — a session left idle past the lease
leaves the list, and one being used never does.

## The commands

Each command is its own RPC, shaped after the browser's own function. The set
grows one command at a time, on purpose: a caller can read what it is allowed to
do off the service definition, and a command nobody serves fails to compile
rather than at runtime.

| | |
| --- | --- |
| `Navigate`, `WaitForNavigation` | drive the page |
| `GetNode` | what an element is, where it is, what it says |
| `HumanMouseMove`, `HumanMouseClick` | the pointer |
| `HumanType` | the keyboard |
| `HumanScrollY`, `HumanScrollYTo` | the wheel |
| `GetCookies`, `SetCookies` | the jar |

**An element is named by CSS selector, every time.** There are no node handles.
A selector is looked up again on each call, so nothing goes stale across a
navigation, nothing has to be released, and there is no map of live elements to
evict. `GetNode` reports the browser's own node id so two matches can be told
apart — not so one can be addressed. Where a command takes both a selector and a
point the selector wins: an element knows where it is, and a caller holding a
rectangle from before the last scroll does not.

**The input is human, and that is the whole point.** A pointer that teleports, a
key held for exactly the same number of milliseconds every time, a scroll that
arrives in one jump — each is a thing no hand does and each is cheap for a page
to notice. `HumanMouseMove` walks a Bezier curve drawn fresh for the move, with
a tremor and a pace that vary step to step; `HumanMouseClick` holds the button
for a length that is drawn rather than fixed; `HumanType` draws a hold per key;
`HumanScrollY` eases to a stop and corrects at the end.

That costs time, and it costs round trips. **A whole gesture is one command**
because the pacing has to happen beside the browser: CDP dispatches one event
per round trip, so a fifty-point trajectory is fifty sequential calls, and
exposing them individually would let the network write the rhythm.

`HumanScrollY` moves the y axis only. The browser's own scroll takes an x
distance and drops it, and a field nothing reads is worse than no field.

## What an operator can do

The same commands, over HTTP, for a caller holding an API key rather than an
execution token — enough to drive a browser with nothing but an HTTP client.

| | |
| --- | --- |
| `GET /v1/browser-sessions` | live sessions, newest first |
| `POST /v1/browser-sessions` | open one |
| `GET /v1/browser-sessions/{id}` | one session |
| `DELETE /v1/browser-sessions/{id}` | stop the browser and drop the session |
| `GET /v1/browser-sessions/{id}/display` | a WebSocket carrying RFB |
| `POST …/navigate`, `POST …/wait-for-navigation` | drive the page |
| `GET …/node?selector=` | describe an element |
| `POST …/mouse-move`, `POST …/click` | the pointer |
| `POST …/type` | the keyboard |
| `POST …/scroll`, `POST …/scroll-to` | the wheel |
| `GET …/cookies`, `PUT …/cookies` | the jar |

`DELETE` stops the browser rather than only forgetting the record. Forgetting
alone would leave a process nothing can reach again, running until the host
restarts — which was survivable while only a handler could open one, because the
handler closed it, and is not once an API key can.

The browser's own answer is what gets translated, not this hop's: a selector
that matched nothing is a `404`, a wait that ran out is a `504`, and a `502` is
this server failing to reach the browser at all.

A display is a signed-in browser rendered as pixels, which is more than any
profile endpoint returns, so it takes its own scope. The credential is checked
**before the handshake completes** — a refused viewer never sees a frame, and an
unreachable service is a `502` rather than a socket that opens and dies.

A framebuffer is Xvfb and x11vnc, so a host without X11 has none: on Windows a
session still opens and drives a browser, and the display endpoint refuses with
`501` rather than dialing a port nothing is serving.

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

It must serve `neurun.browserservice.v1.BrowserService` from
[`proto/browserservice.proto`](../proto/browserservice.proto) — every command
above plus `Open`, `Close` and `StreamDisplay`. What the SDK calls is a separate
file, [`proto/browser.proto`](../proto/browser.proto): one is a public contract
with tenant code, the other an internal detail of this host.

The two files mirror each other deliberately, message for message and field
number for field number, because the control plane relays without reinterpreting
anything. A difference between them would be a translation, and a translation is
a place for the meaning to drift. The one place they part company is
`StreamDisplay`, which the SDK is deliberately not given: it is never told a
browser has an address.

The session id in a `Session` is **the one the service minted**; the control
plane adopts it rather than assigning its own, so both sides call a session by
the same name.

`NEURUN_BROWSER_SERVICE` is the path to its executable. Without it every browser
call refuses rather than failing halfway through opening one.

The implementation drives Chrome over CDP through
[rustenium](../../../rustenium), whose input layer is where the human pointer,
scroll and typing mechanics actually live. Two things it does not have, and
which `neurun-browser` supplies on top: a human scroll that lands on an element,
and a typing rhythm — rustenium's keyboard applies one flat delay to a whole
string, which is the pattern being avoided, so keys are sent one at a time with
a hold drawn for each.

## Status

Built end to end: the session record and its cache repository, the loopback gRPC
server, execution tokens, the supervisor, the driver both doors share,
`neurun-browser` implementing `BrowserService` over rustenium, the operator
endpoints including the command API, the display bridge, the dashboard's list and
detail pages, and both SDKs.

Not built: DOM storage. A profile has `local_storage` and `session_storage`
columns and `Capture` takes them, but nothing fills them — `load_storage` and
`save_storage` are cookies only, which is why every comment about them says so.

The gRPC listener carries no TLS and the control plane dials the browser service
insecurely. Both are correct only because neither leaves the host, and both stop
being correct the moment workers are separate machines — the same cliff the
artifact store crosses.
