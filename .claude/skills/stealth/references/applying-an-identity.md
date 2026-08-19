# Applying an Identity

What `rustenium-identity` does between `IdentitySession::launch(identity)` and a
page load. Knowing the sequence tells you which record fields are load-bearing,
which are decoration, and where a spoof can be caught.

## 1. Resolve the timezone

`tz::resolve_timezone` uses the explicit value if the record has one; otherwise it
issues `GET http://ip-api.com/json/?fields=timezone` **through the identity's
proxy**. Consequences:

- A launch with an empty timezone makes a real outbound request before the
  browser starts, from the exit IP.
- If the proxy is down, the launch fails on timezone resolution rather than on
  the first navigation, which is a confusing error unless you know this.
- Setting the timezone explicitly removes both.

## 2. Launch flags

- `--disable-blink-features=AutomationControlled` — removes the
  `navigator.webdriver` tell **at the source**. The injected JS only patches
  `webdriver` if it is still `true`, which happens when attaching to a browser
  the flag never reached. Patching it unconditionally would be a worse spoof than
  not needing to.
- `--use-gl=angle --use-angle=gl --ignore-gpu-blocklist` — keeps WebGL alive on a
  host with no GPU. ANGLE's default path there is bundled SwiftShader over
  Vulkan, which cannot initialize headless, and Chrome blocklists the software
  renderer it falls back to, so `getContext("webgl")` returns `null`. These point
  ANGLE at the system GL and unblock it. They say nothing about which card the
  page sees — that is the identity's GPU, spoofed on `getParameter`. See
  `references/graphics.md`.
- `--proxy-server=http://127.0.0.1:{port}` when a proxy is set. Chrome cannot
  carry credentials on that flag, so the crate starts a **local overlay proxy**
  first (`local_proxy_server.rs`), binds it on `127.0.0.1:0`, and has it inject
  `Proxy-Authorization` upstream from the `user:pass@host:port` in the URL.
- CDP is forced on and BiDi forced off, whatever the caller's `ChromeConfig`
  said. An identity needs CDP.

**Scheme limit.** The overlay parses `http://` and `https://` only and errors on
anything else — a `socks5://` URL is rejected at launch despite appearing in the
crate's own README example. Store proxies as `http://`.

## 3. CDP emulation

In order, all before the first navigation:

| Command | Carries |
| --- | --- |
| `Emulation.setUserAgentOverride` | UA string, `acceptLanguage` = `language[0]`, `platform` = `navigator_platform`, client-hints metadata |
| `Network.setUserAgentOverride` | the same, belt and braces — both surfaces answer |
| `Emulation.setDeviceMetricsOverride` | **mobile only**: logical width/height, `deviceScaleFactor` = ratio, `mobile: true`, `screenWidth/Height` = logical × ratio |
| `Emulation.setTouchEmulationEnabled` | when `has_touch`, with `maxTouchPoints: 5` |
| `Emulation.setEmitTouchEventsForMouse` | when `has_touch`, `Mobile` configuration on a phone and `Desktop` otherwise — automation mouse input becomes real `touchstart`/`move`/`end` |
| `Emulation.setLocaleOverride` | `language[0]` |
| `Emulation.setTimezoneOverride` | the resolved IANA name |
| `Emulation.setHardwareConcurrencyOverride` | `hardware_concurrency` |
| `Page.addScriptToEvaluateOnNewDocument` | the stealth bootstrap, below |

`deviceMemory` is **not** a CDP override — there is no such command. It is only
spoofed in JS, which is why a worker that reads `WorkerNavigator.deviceMemory`
can still see the host's real value. The fp-spoofer source notes the same gap.

## 4. Client hints

Built for Chrome and Edge, **skipped entirely on iOS and for Safari**.

- `brands`: `Chromium/{major}`, the grease brand at `24`, then
  `Google Chrome/{major}` or `Microsoft Edge/{major}`.
- `fullVersionList`: the same three with full versions and grease `24.0.0.0`.
- **The grease brand differs by platform**: `Not-A.Brand` on Android,
  `Not;A=Brand` everywhere else. That difference is itself checkable.
- `platform`: `Windows`, `macOS`, `Linux`, `Android`, `iOS` — note `macOS`, not
  the record's `Macintosh`.
- `platformVersion`: the record's platform version, not the release.
- `mobile`: derived from `device_model` being set.
- `architecture` and `bitness`: empty strings on mobile.
- `model`: the device model, empty on desktop.

## 5. The stealth bootstrap

One string, wrapped in an IIFE so `PropertyModifier` and the per-brand locals
never become global lexical bindings a page could probe, registered with
`Page.addScriptToEvaluateOnNewDocument` so it runs **before page JS** in every
document and frame.

Substituted values:

| Placeholder | From |
| --- | --- |
| `NAVIGATOR_PLATFORM`, `HARDWARE_CONCURRENCY`, `MEMORY` | the record |
| `LANGUAGES_JSON`, `LANGUAGE_0` | `language`, and its first entry |
| `WEBGL_VENDOR`, `WEBGL_RENDERER` | the GPU pair, JS-escaped |
| `HISTORY_BLOCK` | a loop that `pushState`s until `history.length` reaches `history_count`; empty when the record has none |
| `CHARGING`, `CHARGING_TIME`, `DISCHARGING_TIME`, `BATTERY_PERCENTAGE` | derived: charging is `!has_battery \|\| has_mouse`; times are `Infinity`/`7200` on battery and `0`/`Infinity` on mains; the level is randomised 20–100% per launch |
| `BROWSER_BLOCK` | Chrome, Safari or Edge block — **Safari's block is used for every browser on iOS** |
| `FP_SEED` | FNV-1a over OS, release, GPU pair, id, physical resolution, cores, memory and platform |

The battery percentage is the one value deliberately randomised per launch: a
level that never moves across sessions is its own tell, and the level is not part
of the persona's identity.

### Per-brand blocks

- **Chrome** — three plugins: `Chrome PDF Plugin`, `Chrome PDF Viewer`,
  `Native Client`.
- **Edge** — an empty plugin list.
- **Safari, and all of iOS** — `window.chrome` removed, `navigator.vendor` set to
  `Apple Computer, Inc.`, `window.safari.pushNotification` added with a
  `[object SafariRemoteNotification]` `toString`, and `navigator.userAgentData`
  and `navigator.deviceMemory` **deleted**.

## 6. The toString cloak

The single most important part, and the one most spoofs miss. `PropertyModifier`
patches, and marks as native:

- `Function.prototype.toString` — via a Proxy that returns
  `function {name}() { [native code] }` for any registered function. Without it,
  `navigator.platform`'s getter stringifies to the injected source.
- `Object.getOwnPropertyDescriptor`, `Reflect.getOwnPropertyDescriptor`,
  `Object.getOwnPropertyDescriptors`, `Object.hasOwn` and
  `Object.prototype.hasOwnProperty` — so a "deleted" property is absent from
  every path a page can ask through, not just from direct access.

Spoofed getters are installed with `set: undefined`, `configurable: true` and the
original's `enumerable`, and given a `name` of `get {property}`.

Anything you add later must go through `markNative`, or it defeats the cloak for
everything else on the page.

## 7. Runtime GPU update

`update_gpu.js` re-patches `getParameter` for 37445/37446 through
`Runtime.evaluate`, for changing the card mid-session. It is the older, simpler
patch — it does not `markNative`, so a page that stringifies `getParameter`
after this runs sees injected source. Prefer setting the GPU before launch.

## What is not covered

- **Transport.** TLS (JA3/JA4) and HTTP/2 SETTINGS are Chrome's own, which is
  right as long as you drive the real browser and do not re-issue requests from
  another HTTP client. → `automation` skill, `references/network-and-proxy.md`.
- **Fonts.** Text metrics are perturbed; the installed font *set* is not hidden.
  Equal-width fallbacks stay equal, which needs OS-level control.
- **Workers.** `deviceMemory` and other navigator values read inside a worker are
  not covered by the main-world script.
- **Coherence.** Nothing here checks that the record agrees with itself.
