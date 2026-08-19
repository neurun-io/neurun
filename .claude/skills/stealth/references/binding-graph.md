# The Binding Graph

Which identity fields determine which others, and why each edge exists. The
authority for every rule here is `rustenium-identity` (what applies the record)
and `internal/domain/browser/catalog.json` (what may be selected).

## Operating system — the widest binding

An OS fixes three fields outright and narrows four lists.

| Fixed | Windows | Macintosh | Android | iOS |
| --- | --- | --- | --- | --- |
| `navigator.platform` | `Win32` | `MacIntel` | from the handset (`Linux armv8l`, `Linux armv9l`, `aarch64`, `Android`, `Linux armv7l`) | from the handset (`iPhone`) |
| `platform.bitness` | `64` | `64` | empty | empty |
| `platform.architecture` | `x86` | `x86` | empty | empty |

Bitness and architecture are **empty strings on mobile**, not absent and not
`x86` — `cdp.rs` blanks them whenever `device_model` is set, and client hints are
built from the same test.

Narrowed by the OS: the browser list, the release list, the GPU list, and whether
the form factor is `desktop` (screen and hardware are chosen) or `mobile` (a
handset supplies them). Each of those is a field *on* the OS entry in the
catalogue, so a card or a release cannot be offered under a system that never
reported it.

Switching OS must re-choose everything under it. Carrying the old values across
is how a Mac ends up claiming `Win32` — `withOS` in
`frontend/lib/view/browser-identity.ts` re-derives browser, release, platform
version and GPU on every switch, and clears the handset when moving to a desktop.

## OS × browser is not a preference

`rustenium_identity::ua::build_user_agent` returns
`IdentityError::UaError("unsupported OS/browser combination")` for any pair it
does not know. Offering one in a form produces a profile the runtime refuses.

The crate's pairs are below. **Neurun offers a subset**: `chrome` and `safari`
only, so the Edge column is what the crate can build, not what a profile can ask
for.

| OS | Browsers | UA shape |
| --- | --- | --- |
| Windows | chrome, edge | `Mozilla/5.0 (Windows NT {nt}; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/{v} Safari/537.36`, Edge appends ` Edg/{v}` |
| Macintosh | chrome, safari, edge | `Mozilla/5.0 (Macintosh; Intel Mac OS X {v_underscored}) …` — Chrome/Edge use `AppleWebKit/537.36`, Safari uses `AppleWebKit/605.1.15 … Version/{v} Safari/605.1.15` |
| Linux | chrome | `Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 … Chrome/{v} Safari/537.36` |
| Android | chrome, edge | `Mozilla/5.0 (Linux; Android {v}; {device_model}) … Chrome/{v} Mobile Safari/537.36`, Edge inserts ` Mobile EdgA/{v}` |
| iOS | safari, chrome, edge | `Mozilla/5.0 ({navigator_platform}; CPU iPhone OS {v_underscored} like Mac OS X) AppleWebKit/605.1.15 …` then `Version/{v} Mobile/15E148 Safari/604.1`, or `CriOS/{v}`, or `Version/{v} EdgiOS/{v}` |

Notes that matter:

- macOS says **Intel Mac OS X** even on Apple silicon. That is what real browsers
  report; "correcting" it is the contradiction.
- macOS and iOS versions have dots replaced with underscores in the UA
  (`13.4.1` → `13_4_1`) but not in the record.
- The Android UA embeds `device_model` directly, defaulting to `Pixel 7` when
  absent — so an Android identity without a model silently claims a Pixel 7.
- iOS `Mobile/15E148` is a constant in the builder, matching the `models` column
  the handset tables carry for iPhones.

## Release → platform version

The release a person names and the value that ships in the record are different
strings. This is the single most common source of an incoherent profile.

| OS | Release | UA-CH platform version | Where it comes from |
| --- | --- | --- | --- |
| Windows | 11 | `15.0.0`, `14.0.0`, `13.0.0` | Chromium's Windows mapping |
| Windows | 10 | `10.0.0`, `8.0.0`, `7.0.0` | same |
| Windows | 8, 7 | `0.0.0` | Chromium reports 0 for everything before 10 |
| Macintosh | 14 | `14.6`, `14.5`, `14.4.1`, … | the macOS point release itself |
| Android / iOS | 13, 17.5 | `13.0.0`, `17.5.0` | the release padded to three parts |

Windows also has a **second** mapping, into the UA string rather than client
hints: NT `10.0` for 11 and 10, `6.3` for 8.1, `6.2` for 8, `6.1` for 7. Two
different encodings of the same release, both derived, neither typed.

## Browser

The identity field is `browser`, and it is the only browser a profile has.

It fixes:

- **Its version list.** Chrome carries four-part Chromium versions; Safari
  carries two-part versions (`18`, `17.6`).
- **Which APIs exist.** Safari — and every browser on iOS, because they are all
  WebKit — has no `navigator.userAgentData` and no `navigator.deviceMemory`, and
  must not have `window.chrome`. Chrome carries three plugins.
- **Client hints or not.** Built for Chrome, skipped entirely on iOS and for
  Safari.

It does **not** fix the WebGL pair: the card belongs to the system or the
handset. See `references/graphics.md`.

## Browser version → the GREASE brand

The greasey entry in `Sec-CH-UA` is not a constant. Chromium derives both halves
of it from the **major version**, indexing two tables:

```
brand   = "Not" + CHARS[major % 11] + "A" + CHARS[(major + 1) % 11] + "Brand"
version = VERSIONS[major % 3]

CHARS    = [" ", "(", ":", "-", ".", "/", ")", ";", "=", "?", "_"]
VERSIONS = ["8", "99", "24"]
```

| Major | Brand | Version |
| --- | --- | --- |
| 150 | `Not;A=Brand` | `8` |
| 151 | `Not=A?Brand` | `99` |
| 152 | `Not?A_Brand` | `24` |
| 153 | `Not_A Brand` | `8` |

151 and 152 were read off running browsers; `grease()` in `cdp.rs` is pinned to
both by test. **A hardcoded pair is wrong for all but one release** — the value
the crate used to ship, `Not;A=Brand` with version `24`, is 150's brand beside
152's version, which no browser has ever sent together.

The OS does **not** enter into it. An earlier Android special case (`Not-A.Brand`)
was removed: the derivation is platform-independent, so the same major greases
the same way everywhere.

The point of the field is that a parser must tolerate junk, which makes it easy
to assume nothing reads it. What it actually carries is a third, independent
statement of the browser version — so a stale one contradicts the UA string and
the full version list beside it.

## Handset — the binding unit on mobile

One model fixes the screen, the ratio, the GPU, the cores and the memory
together, because they shipped in one box. A phone is also
`has_touch: true`, `has_mouse: false`, `has_battery: true`.

What is still a choice after picking a handset: which release (from its own
list), which model code (several share one handset — `SM-S911B`, `SM-S911U`,
`SM-S911B/DS`), which `navigator.platform` when the device reports more than one,
and which of its cards answered when it shipped with more than one.

`Sec-CH-UA-Model` carries the model code, and client-hints `mobile` is derived
from `device_model` being set — not from the OS. An empty model on a phone breaks
both.

## Screen and ratio

`Emulation.setDeviceMetricsOverride` (mobile only) sends the logical size, the
ratio as `deviceScaleFactor`, and `screenWidth/screenHeight` computed as
logical × ratio. The record stores the physical resolution separately, so **the
two must multiply out** or the page sees a screen no device has. The form derives
it; nothing should type it.

On desktop no device metrics are sent at all — the real window is used, and the
logical/physical pair only has to be internally consistent.

## Country → language and clock

A country implies a language list and an IANA timezone. Exit geography that
disagrees with `Accept-Language` or the clock is a documented detection signal,
and the identity carries all three, so they are filled together.

`Accept-Language` and `Emulation.setLocaleOverride` both use `language[0]`, so the
first entry is the one that matters. The rest populate `navigator.languages`.

If the timezone is left empty, rustenium-identity fetches it from
`ip-api.com/json/?fields=timezone` **through the proxy** at launch — a real
network call from the exit IP. Setting it explicitly avoids that round trip and
the dependency.

## What the server enforces

`Identity.Validate` in `internal/domain/browser/identity.go` checks fields
independently: OS, browser and geo are valid enums; os version, navigator platform,
platform version, browser version, languages, screen, hardware and GPU are
non-empty and non-zero.

It does **not** check that the fields agree with each other. A Direct3D renderer
on macOS passes. Coherence is the caller's job today; the preflight verdict that
would refuse an incoherent run is `ROADMAP.stealth`.
