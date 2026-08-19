---
name: stealth
description: "Browser identity coherence for Neurun browser profiles — what a detector reads, which identity fields bind to which, and what rustenium-identity actually applies at launch. Use when building, reviewing or debugging a stealth identity (OS, release, platform version, brand, screen, GPU, locale, proxy), when extending the identity catalogue, when deciding whether a field is free text or a bound choice, or when a target starts refusing a profile that used to work."
user-invocable: true
---

# Stealth

Anti-bot systems catch **contradictions**, not bad user agents: a ClientHello that
says Chrome while the header order says Go, a datacenter ASN whose timezone is
residential Berlin, a Direct3D renderer on a Mac. Every field is cheap to set and
worthless alone — the identity is the agreement between them.

Two rules govern everything here.

1. **Never offer a field as free text when something else on the record already
   determines it.** The catalogue exists so a caller selects instead of invents.
2. **A persona must be stable across runs and within a page.** A canvas hash that
   changes every time is as loud as a known-bad one; one fingerprint hopping ASNs
   is louder still.

## Where the pieces live

| Piece | Where |
| --- | --- |
| The record | `internal/domain/browser/identity.go` — mirrors `rustenium-identity`'s `Identity` field for field |
| The values it may hold | `internal/domain/browser/catalog.json`, served by `GET /v1/identity-catalog` |
| The bindings, client side | `frontend/lib/view/browser-identity.ts` |
| The form | `frontend/components/browser-profiles/profile-form.tsx` |
| What applies it | `rustenium-identity` (Rust, over CDP), launched by the SDK on loopback |
| Why the control plane never launches a browser | `docs/browser-profile.md` |

## The binding graph, in one table

| Choosing | Fixes |
| --- | --- |
| Operating system | `navigator.platform`, bitness, architecture, which browsers exist, which releases exist, which GPUs can report, desktop vs mobile |
| Release | the UA-CH platform version — a **different string**: Win 11 → `15.0.0`, Win 10 → `10.0.0`, Win 8 and 7 → `0.0.0`, macOS 14 → `14.6`, Android 13 → `13.0.0` |
| Browser | its version list, and which JS APIs exist at all. `chrome` or `safari` — Neurun offers no others, and the field is `browser`, on the identity |
| Browser **version** | the GREASE brand in `Sec-CH-UA` — both its punctuation and its version rotate with the major, so a fixed pair matches at most one release |
| Handset (mobile) | screen, ratio, GPU, cores, memory, `Sec-CH-UA-Model`, release list — they shipped in one box |
| Screen + ratio | the physical resolution. Derived, never typed twice |
| Country | the language list and the IANA timezone |

→ `references/binding-graph.md` for the full rules, the OS × browser matrix and the
UA shapes each pair produces.

→ `references/applying-an-identity.md` for what happens at launch, in order:
flags, CDP emulation, client hints, and the script injected before page JS.

## The vectors

One reference per vector, each covering how it is read, what this suite sets, how
it plays with the rest, how it is caught, and what is still missing.

| Vector | Reference | Weight |
| --- | --- | --- |
| WebGL strings and canvas/readback pixels | `references/graphics.md` | highest entropy; read first, on every visit |
| UA string, Client Hints, navigator | `references/user-agent.md` | low entropy alone, decisive in combination |
| AudioContext, text metrics, font set | `references/audio-and-fonts.md` | low visibility, excellent for linking |
| Screen, ratio, cores, memory, touch, battery | `references/device-class.md` | jointly describes a machine that must exist |
| Language, timezone, IP geography, WebRTC | `references/locale-and-network.md` | triangulated; most often lost to the proxy |
| `webdriver`, `toString`, descriptors, worker gaps | `references/automation-artifacts.md` | gates every other vector |
| TLS/JA4, HTTP/2, header order | `references/transport.md` | presentation spoofing cannot reach it |
| State, history, fingerprint stability, cadence | `references/continuity.md` | catches fleets of perfect strangers |

### How they interlock

The four checks that catch most otherwise-good identities are cross-vector:

1. **UA ↔ graphics** — a renderer string names its OS and its driver stack.
2. **UA ↔ transport** — a claimed Chrome obliges a Chrome TLS fingerprint.
3. **Locale ↔ network** — timezone, `Accept-Language` and exit IP must agree.
4. **Device class ↔ itself** — screen, ratio, cores, memory, touch and battery
   have to describe one machine that shipped.

And two that catch fleets rather than sessions: a fingerprint that is unstable
across runs, and a fingerprint that is *too* stable across supposedly unrelated
profiles.

## Cross-references

- → See the `automation` skill for the session loop, input dispatch and
  behaviour — the layer that gets caught after the identity passes.
- → `docs/browser-profile.md` for the storage boundary and the secrets rules.
- → `frontend/lib/roadmap.ts` (`ROADMAP.stealth`) for the coherence checks the
  server would have to publish before any of this is enforced rather than
  configured.

## Common mistakes

| Mistake | Fix |
| --- | --- |
| Treating no WebGL as safer than a spoofed one | A desktop with `getContext("webgl") === null` fails the first vector every detector reads, and leaves nothing for the GPU strings to apply to. See `references/graphics.md` |
| Building the browser host from Chrome's package dependencies alone | That ships one font family. A desktop has hundreds, and no JS patch can add them — see `references/audio-and-fonts.md` |
| Sizing the window to the screen | Chrome lands a pixel short of it, identically on every session. Maximize under a window manager instead — see `references/device-class.md` |
| Hardcoding the greasey `Not;A=Brand` entry | It is derived from the browser major. A fixed one contradicts the version claimed in the UA beside it — see `references/binding-graph.md` |
| Typing a value the catalogue already lists | Select it. A hand-typed release or GPU is how a profile stops matching any real machine |
| Using the release as the platform version | They are different strings. Win 11 reports `15.0.0`; Win 7 and 8 both report `0.0.0` |
| Offering Safari on Windows | `build_user_agent` errors on unsupported pairs — the profile is refused at launch. Safari is a Mac and iOS answer |
| A Direct3D renderer on macOS | ANGLE over Direct3D exists only on Windows. Bind the GPU list to the OS |
| Claiming a GeForce from a host with no GPU | Only the two strings are spoofed; the limits and extension list come from the real driver. Claim a card on the *same driver stack* the host runs — see `references/graphics.md` |
| `Adreno 740` | Real strings carry the trademark: `ANGLE (Qualcomm, Adreno (TM) 740, OpenGL ES 3.2)` |
| `deviceMemory: 16` | The browser rounds to a power of two and caps at 8. Only 1, 2, 4, 8 appear |
| Client hints on Safari or on any iOS browser | iOS is WebKit throughout — no `navigator.userAgentData`, no `deviceMemory` |
| Editing OS, GPU, screen or cores "slightly" | Those fields seed the hardware fingerprint. Changing one mints a new persona |
| PATCHing a profile without re-sending the proxy | The API never returns it, so an omitted proxy is a cleared proxy |
| Reading `browser` as what launches | It is what the profile *claims*. rustenium-identity drives Chrome only, so `safari` is a Chrome wearing Safari |
| Randomising the canvas per run | Instability is a tell. The noise is seeded from the identity on purpose |
