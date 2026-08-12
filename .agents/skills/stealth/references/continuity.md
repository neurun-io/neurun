# Vector: Continuity — Having a Past

Every other vector answers "what is this?". This one answers "**have I seen this
before, and does its story hold?**" It is the vector that catches profiles which
are individually perfect and collectively obviously synthetic: a thousand
machines that have each existed for four seconds.

## How it is read

| Signal | Where it comes from |
| --- | --- |
| cookies, `localStorage`, `IndexedDB`, cache entries, service workers | the profile's own state |
| the fingerprint hash *across sessions* | canvas, audio, WebGL, text metrics |
| `window.history.length` | JS |
| `document.referrer` and the navigation path | JS and headers |
| account age, session cadence, time-of-day pattern | the target's own records |
| the same fingerprint seen on a different IP, or vice versa | the target's correlation |

## What this suite provides

**State that survives.** A browser profile stores its cookies and DOM storage
between runs (`GET`/`PUT /v1/browser-profiles/{id}/state`), so a run that signed
in yesterday is still signed in today. That is the whole reason profiles exist —
and mechanically it belongs to the `automation` skill.

**A stable fingerprint.** `FP_SEED` is an FNV-1a hash of OS, release, GPU vendor
and renderer, id, physical resolution, cores, memory and navigator platform. All
canvas, WebGL-readback, audio and text noise derives from it, so the persona's
hashes are identical across sessions and distinct from every other persona.

**A history.** `history_count` is realised by `pushState`ing until
`window.history.length` reaches it, so a fresh tab does not report 1.

**A referrer.** The older bot injects a stored referrer into the first request of
a session, so an arrival is not always direct.

## The rules

1. **Do not re-seed.** Anything that changes a seeded field mints a new persona.
   Editing the GPU, screen, cores, memory, OS or release of an existing profile
   *is* a new machine wearing an old cookie jar — which is a contradiction, not a
   tweak. The dashboard gates this behind a typed confirmation whenever the
   profile already remembers something; the gate exists because the cost is
   invisible at save time and arrives a week later.
2. **Let the browser version climb, never fall.** Updating is what a real install
   does unprompted; downgrading is not something that happens. A regressed
   version on an unchanged fingerprint is a stronger tell than a stale one. →
   `user-agent.md`.
3. **Do not share a seed between personas that should be strangers.** Two
   profiles built from the same handset with the same defaults hash identically.
   Vary at least one seeded field, or accept they are linkable.
4. **Do not move a persona's country between sessions** while keeping its state.
   People do travel; fleets do not.
5. **Do not let state and identity drift apart.** A cookie jar full of a German
   site's sessions attached to a `en-US`/New York identity is a story that does
   not hold.
6. **Do not start every session from a blank history and a direct arrival.**
7. **Do not run two sessions from one profile concurrently.** Both capture, and
   the last `PUT` replaces wholesale — the account's own view of the "user"
   becomes two overlapping sessions from one machine.

## How it plays with the rest of the suite

- **← Graphics, audio, fonts.** They *are* the continuity signal. Their stability
  is what lets a target recognise a returning user — which is the goal when you
  want to be recognised as the same person, and the risk when you want two
  profiles to be strangers.
- **← Locale and network.** A stable fingerprint on a rotating IP is a strong
  bot signal; so is a rotating fingerprint on a stable IP. Pick one story: a
  person is a stable machine on a mostly-stable connection.
- **← Device class.** The persona's hardware is in the seed, so device and
  history are welded together. That is correct and worth exploiting: a profile is
  a *machine with a past*, not a set of headers.
- **→ Behaviour.** Cadence is continuity over time. A persona that only ever
  appears for ninety seconds, always doing the same thing, has a shape.

## How it gets caught

1. **A new fingerprint on an old cookie.** The account's machine changed
   completely overnight.
2. **An old fingerprint on a new account**, repeatedly — one machine minting
   accounts.
3. **`history.length === 1`** on a claimed returning visitor.
4. **Always-direct arrivals**, or a referrer that could not have linked there.
5. **Fleet-shaped timing** — every persona active in the same narrow window,
   sessions of near-identical length.
6. **State that contradicts the identity**: language, currency or region in the
   stored cookies disagreeing with the claimed locale.
7. **Impossible travel** — the same profile in two countries within an hour.

## Gaps

- Nothing in the record versions the identity, so there is no way to tell that a
  profile's persona changed on a given date; `ROADMAP.stealth` calls for a
  versioned profile pinned to an execution the way a build is.
- `history_count` pushes same-document entries; the stack has repeats.
- No session-cadence modelling exists at this layer — it belongs to whatever
  schedules the runs, and nothing here enforces it.
- The fingerprint seed does not include the proxy or geo, so moving a persona's
  exit does not change its hashes. That is deliberate (the machine did not
  change) but means geo and fingerprint can drift apart without anything noticing.
