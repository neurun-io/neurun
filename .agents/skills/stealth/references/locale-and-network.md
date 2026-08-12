# Vector: Locale, Clock and Network Position

Where the browser says it is, and where the packets say it is. This vector is
scored by triangulation — language, timezone, IP geography and the target's own
CDN edge should all point at one place — and it is the vector most often lost by
otherwise good identities, because the proxy is chosen separately from the
persona.

## How it is read

| Probe | Surface |
| --- | --- |
| `navigator.language`, `navigator.languages` | JS |
| `Accept-Language` | header |
| `Intl.DateTimeFormat().resolvedOptions().timeZone` | JS |
| `new Date().getTimezoneOffset()` | JS, and it must agree with the IANA zone *including DST* |
| `Intl.DateTimeFormat().resolvedOptions().locale`, `Intl.NumberFormat` output | JS |
| the connecting IP: ASN, class, rDNS, geo-IP | server side |
| WebRTC ICE candidates | JS — the classic leak |
| DNS resolver egress | network side |

## What this suite sets

- `Emulation.setUserAgentOverride(acceptLanguage = language[0])` and
  `Network.setUserAgentOverride` — the header.
- `Emulation.setLocaleOverride(language[0])` — `Intl` and formatting.
- `navigator.languages` and `navigator.language` in JS, from the full list.
- `Emulation.setTimezoneOverride(timezone)` — `Intl` and `getTimezoneOffset`
  together, which is why the offset stays consistent with DST.
- The catalogue pairs each of the 18 country codes with a language list and an
  IANA zone, and the form fills all three when a country is chosen.
- The proxy is applied at the browser through a local overlay
  (`--proxy-server=http://127.0.0.1:{port}`) that injects credentials upstream,
  so **all** browser traffic uses the exit — not just the pages you thought about.

**Timezone resolution.** If the record leaves the timezone empty,
`tz::resolve_timezone` fetches it from `ip-api.com/json/?fields=timezone`
**through the proxy** before launch. That makes the clock automatically agree
with the exit, at the cost of a real outbound request from the exit IP and a
launch that fails if the proxy is down. Setting it explicitly is usually better —
and required if the exit rotates.

## How it plays with the rest of the suite

- **← User agent.** `Accept-Language` and `navigator.language` derive from the
  same `language[0]`, so those two cannot drift.
- **← Device class.** Weak correlation, occasionally scored: device model and
  region.
- **→ Behaviour.** Time of day at the *claimed* location matters. A "Berlin" user
  browsing steadily at 04:00 local is not impossible, but a fleet of them is.
- **→ Transport.** The exit's ASN class (residential, mobile, datacenter) is read
  alongside everything above; a perfect presentation layer behind a datacenter IP
  is a well-dressed bot.
- **→ Continuity.** The timezone is not in the fingerprint seed, so changing it
  does not change canvas hashes — but a persona that moves country between
  sessions while keeping one cookie jar is a person who teleports.

## How it gets caught

1. **Timezone vs IP.** `Europe/Berlin` from a US exit. The single most common
   miss, and trivially checked server-side.
2. **Language vs geography.** `en-US` from a Warsaw residential IP, at scale.
3. **Offset vs zone.** A patched `getTimezoneOffset` that does not follow DST for
   the claimed zone. Using the CDP override avoids this; hand-patching does not.
4. **WebRTC.** ICE candidates enumerate the local interfaces and the real public
   IP unless the browser is configured to prevent it. The proxy overlay does
   **not** cover WebRTC — a UDP path around an HTTP proxy is the leak this vector
   is famous for.
5. **DNS outside the tunnel.** Resolving through the host's resolver while
   claiming the exit's location.
6. **ASN class.** Datacenter ranges are enumerated and sold; residential and
   mobile are the point of paying for proxies.
7. **`Intl` inconsistencies.** Currency, first day of week and number formatting
   derive from the locale; overriding only `navigator.languages` and not the
   locale leaves them describing the host.
8. **The launch-time `ip-api.com` request** is itself a signal to anyone
   correlating: a fresh exit that hits a geo-IP API and then a target, every time,
   is a pattern.

## Gaps in this implementation

- **No WebRTC handling at all.** If the target reads ICE candidates, the real IP
  is available. Disable it at launch or accept the leak.
- **No DNS control.** Resolution follows the host unless the proxy is a full
  tunnel.
- The overlay proxy speaks `http://` and `https://` only — `socks5://` is
  rejected at launch despite appearing in the crate's own README.
- No check that the timezone agrees with the exit at run time; the automatic
  resolution happens once, before launch.
- The geo list is 18 countries because the domain enum is; a proxy exiting
  anywhere else has no coherent language/timezone pairing in the catalogue.
