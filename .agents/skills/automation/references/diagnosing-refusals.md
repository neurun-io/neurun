# Unit: Diagnosing Refusals

When a target starts refusing, the useful question is **which layer diverged, and
when** — not "rotate the proxy". Rotation is what you do after you know; done
first, it destroys the evidence and usually the profile too.

## The four failure shapes

They need different responses, and only the first two look like errors.

### 1. Hard block

`403`, a WAF page, a connection reset. The request never reached the
application. Almost always transport or network position: TLS fingerprint, ASN
class, or an IP already burned.

### 2. Rate limit

`429`, sometimes with `Retry-After`. Says nothing about whether you were
detected — only that you asked too much from one place. Cluster before
concluding: per-IP, per-account, per-endpoint.

### 3. Challenge

A `200` carrying an interstitial, a CAPTCHA, or a JS challenge that never
resolves. **A 200 is not success.** This is the shape that silently corrupts a
run: the scraper "succeeds", the parser finds nothing, and the output looks like
the site changed. Classify challenge pages explicitly and treat them as their own
outcome.

### 4. Silent degradation

A `200`, the right shape, and content that is empty, stale, logged-out, or
subtly different — prices replaced with "sign in to see price", a list that
returned forty rows returning none. Still a string, still non-empty, still
passing every validator downstream.

This is a data-health problem, not a transport one: compare against what the
app's previous executions agreed on, rather than inventing a second baseline.
→ `ROADMAP.dataHealth`.

## Cluster before you conclude

The single most useful discipline. Cluster failures by:

| Axis | What it tells you |
| --- | --- |
| **Profile cohort** — identities sharing an OS, brand, GPU or catalogue vintage | one identity *shape* is burned |
| **Exit** — IP, subnet, ASN, provider | the network position is burned |
| **App** | the behaviour is the problem, not the identity |
| **Target endpoint** | the site changed, or that route is protected differently |
| **Time** | the target deployed something |

If one identity shape fails everywhere → the identity. If one app fails
everywhere → the behaviour. If everything fails at one target from one hour →
they shipped something. Rotating proxies fixes only the second.

## Layer-by-layer triage

Work outside in; each step is cheaper than the one after.

1. **Network position.** Is the exit residential? Was it working an hour ago? Does
   its geo still match the identity's timezone and language? Did the exit rotate
   underneath a persona that kept its cookies?
2. **Transport.** Are you re-issuing any requests from an HTTP client? Did header
   stripping change? A JA4 that stopped matching the claimed brand is the usual
   culprit after a dependency upgrade.
3. **Identity coherence.** Did anything edit the profile? An OS, GPU, screen,
   cores or memory change *also changes the fingerprint hashes* — to the target,
   the machine was replaced. Check `updated_at` against when refusals started.
4. **Continuity.** Is the profile's state intact, or did a bad `PUT` wipe it? A
   suddenly-logged-out profile presenting a fresh jar with an old fingerprint is
   a strong signal.
5. **Automation artifacts.** Did a new patch get added without `markNative`? One
   unmarked function characterises the environment.
6. **Behaviour.** Did the run get faster? A timing regression — a removed sleep, a
   faster machine, a parallelism bump — is a behavioural change nobody logged as
   one.

## What to record

Diagnosis is only possible if the run left evidence. At minimum, per run:

- profile id and its `updated_at` at open;
- the identity's OS, brand, version, GPU and geo — the cohort axes;
- exit IP or provider and the resolved timezone;
- outcome per request class: ok, challenged, rate-limited, blocked, failed;
- the page classification for any non-ok navigation, not just the status code;
- whether state was written back.

Neurun does not record any of this against the execution today. `ROADMAP.stealth`
calls for a versioned profile pinned to an execution the way a build is, and for
a break recorded as an event naming the offending layer and its last coherent
version — that is the contract this triage wants.

## What not to do

- **Do not rotate first.** You lose the one variable you could have tested.
- **Do not retry a `407`.** Bad credentials do not fix themselves.
- **Do not retry a challenge in a loop.** Repeated challenge solving from one
  exit is itself the signal.
- **Do not "fix" it by editing the identity.** Every edit to a seeded field mints
  a new machine on an old cookie jar, which is worse than the original problem.
- **Do not write back state from a challenged session** unless you are sure it
  did not clear the jar.
- **Do not treat `200` as success.** Classify the page.
