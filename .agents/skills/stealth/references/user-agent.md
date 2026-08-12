# Vector: User Agent, Client Hints and Navigator

The cheapest vector to read and the cheapest to fake, which is why nobody scores
it alone — it is scored by whether its **four independent encodings of the same
facts** agree. The UA string, the Client Hints, `navigator.*` and the HTTP
headers each state the OS, the browser and the version, and a mismatch between
any two is decisive.

## How it is read

| Probe | Surface |
| --- | --- |
| `navigator.userAgent`, `appVersion` | the classic string |
| `Sec-CH-UA`, `Sec-CH-UA-Platform`, `-Platform-Version`, `-Arch`, `-Bitness`, `-Model`, `-Mobile`, `-Full-Version-List` | request headers, low- and high-entropy hints |
| `navigator.userAgentData.getHighEntropyValues()` | the same, from JS |
| `navigator.platform`, `vendor`, `appName`, `product`, `oscpu` | legacy navigator fields |
| header **order** and casing | transport — see `transport.md` |

## What this suite sets

**The UA string** is built per OS × brand pair by `ua.rs`, and the pair itself is
constrained: `build_user_agent` errors on anything it does not know, so an
unsupported combination fails at launch rather than shipping a broken claim.

| OS | Brands | Shape |
| --- | --- | --- |
| Windows | chrome, edge | `Windows NT {10.0\|6.3\|6.2\|6.1}; Win64; x64` … `Chrome/{v} Safari/537.36`, Edge appends ` Edg/{v}` |
| Macintosh | chrome, safari, edge | `Macintosh; Intel Mac OS X {v_underscored}` — Safari uses `AppleWebKit/605.1.15 … Version/{v} Safari/605.1.15` |
| Linux | chrome | `X11; Linux x86_64` |
| Android | chrome, edge | `Linux; Android {v}; {device_model}` … `Mobile Safari/537.36`, Edge inserts ` Mobile EdgA/{v}` |
| iOS | safari, chrome, edge | `{platform}; CPU iPhone OS {v_underscored} like Mac OS X` … `Version/{v} Mobile/15E148 Safari/604.1`, or `CriOS/{v}`, or `EdgiOS/{v}` |

Three details that look like bugs and are not: macOS says **Intel Mac OS X** even
on Apple silicon; version dots become underscores in the UA but not in the
record; and Windows has **two** encodings of the release — NT `6.1` for 7 in the
UA, `0.0.0` for 7 in Client Hints.

**Client hints** are built for Chrome and Edge and **skipped entirely on iOS and
for Safari**. Brands are `Chromium/{major}`, a grease brand at `24`, and
`Google Chrome`/`Microsoft Edge` at `{major}`; the grease brand is `Not-A.Brand`
on Android and `Not;A=Brand` elsewhere. `platform` is `macOS`, not the record's
`Macintosh`. `mobile` is derived from `device_model` being set, and `architecture`
and `bitness` are empty strings on mobile.

**Navigator** gets `platform` from the record, plus `vendor` =
`Apple Computer, Inc.` on Safari/iOS, where `window.chrome` is removed and
`userAgentData` and `deviceMemory` are deleted outright.

Both `Emulation.setUserAgentOverride` and `Network.setUserAgentOverride` are sent,
so the JS surface and the header surface answer identically.

## How it plays with the rest of the suite

- **→ Graphics.** The OS in the UA decides which renderer strings are possible at
  all. This is the single most-checked cross-vector pair.
- **→ Screen.** `Sec-CH-UA-Mobile: ?1` with a 1920×1080 logical screen and no
  touch is incoherent; the mobile flag and the device metrics come from the same
  `device_model` test.
- **→ Hardware.** `Sec-CH-UA-Model` names a handset whose cores, memory and GPU
  are then checkable against public spec tables.
- **→ Locale.** `Accept-Language` is `language[0]`, and both are sent through the
  same override, so they cannot drift apart.
- **→ Transport.** A UA claiming Chrome 139 obliges a Chrome 139 TLS fingerprint
  and HTTP/2 SETTINGS. The suite gets this right only because it drives the real
  browser — re-issuing requests from an HTTP client breaks it silently.
- **→ Automation artifacts.** Safari's block *deletes* `userAgentData`; a
  detector that finds it present alongside a Safari UA has a contradiction, and
  one that finds a deleted property still visible through
  `Object.getOwnPropertyDescriptor` has an instrumented environment.

## How it gets caught

1. **UA says one thing, client hints say another.** Chrome 139 in the string,
   Chrome 124 in `fullVersionList`.
2. **Client hints present where they cannot be.** Safari and all of iOS have no
   UA-CH. Sending them is a self-refutation.
3. **`navigator.platform` disagreeing with the UA OS** — `Win32` inside a
   `Macintosh` UA is a stock check in every fingerprinting library.
4. **Mobile flag without mobile anything.** `?1` with mouse events, no touch
   points, and a desktop viewport.
5. **A model that does not exist**, or an Android UA with no model at all — the
   builder silently defaults to `Pixel 7`, so an unset model claims a Pixel 7's
   spec sheet.
6. **The wrong grease brand for the platform**, or a brands array in an
   implausible order.
7. **Legacy fields left alone.** `oscpu`, `appVersion` and `productSub` are not
   patched here; on a claimed platform where they differ, they still read the
   host.

## The version can only move forwards

A browser updates itself. Over a profile's life its version is expected to climb,
and one that never moves is a weak signal on its own — a machine that has not
restarted its browser in eight months.

**Backwards is not a weak signal.** No install downgrades itself, so a profile
whose version regressed between sessions is claiming an event that does not
happen. Combined with continuity — same cookies, same fingerprint, older browser
— it is a stronger tell than the version being stale in the first place.

The dashboard enforces this asymmetry:

- **Forward** saves straight through, with a note that says so. It is the normal
  case and should not feel like a warning.
- **Backward** is gated behind the same typed confirmation as a fingerprint
  change (`frontend/components/browser-profiles/confirm-fingerprint-change.tsx`),
  because the only honest reason to do it is that the persona was wrong from the
  start.

`versionMove` in `frontend/lib/view/browser-identity.ts` compares the version
arrays part by part, so `139.0.6889.109 → 139.0.6889.200` is forward and
`139.0.6889.109 → 139.0.6889.1` is backward.

Two related rules that are not enforced anywhere yet: a version should not jump
so far forward that the intervening releases never existed, and it should not
outrun the real stable channel.

## Gaps in this implementation

- Only `platform` is spoofed on navigator; `appVersion`, `oscpu`, `productSub`
  and `buildID` are untouched.
- Nothing verifies the browser version is one that exists, or is recent enough
  to be plausible — the catalogue lists real versions, but a stale snapshot ages
  into a weak signal.
- Header order and casing are Chrome's own, which is correct while the real
  browser sends them, and wrong the moment a request is re-issued elsewhere.
