# Vector: Device Class — Screen, Hardware and Power

Individually weak, jointly decisive. Screen size, pixel ratio, core count,
memory, touch capability and battery state all answer the same question — *what
kind of machine is this* — and a detector scores the **agreement between them**
rather than any single value.

This is the vector where the catalogue's handset binding earns its place: a real
phone's screen, ratio, GPU, cores and memory shipped in one box, and offering
them as five free choices produces devices that never existed.

## How it is read

| Probe | Notes |
| --- | --- |
| `screen.width/height`, `availWidth/availHeight` | the panel, minus OS chrome for `avail*` |
| `window.innerWidth/innerHeight`, `outerWidth/outerHeight` | viewport and window; the deltas expose browser chrome |
| `devicePixelRatio` | 1, 1.25, 1.5, 2 on desktop; 2–3.5 on phones |
| `navigator.hardwareConcurrency` | true core count |
| `navigator.deviceMemory` | RAM rounded to a power of two, capped at 8 |
| `navigator.maxTouchPoints` | 0 on desktop, 5 on a phone |
| `matchMedia('(pointer: coarse)')`, `(hover: none)`, `(any-pointer: fine)` | the input model, independent of `maxTouchPoints` |
| `navigator.getBattery()` | charging, level, chargingTime, dischargingTime |
| `screen.orientation` | `portrait-primary` on a phone |

## What this suite sets

**Screen.** `Emulation.setDeviceMetricsOverride` is sent **on mobile only**: the
logical size as width/height, the ratio as `deviceScaleFactor`, `mobile: true`,
and `screenWidth`/`screenHeight` computed as logical × ratio. On desktop nothing
is overridden — the real window is used.

Because the override computes the physical size, the record's logical, ratio and
physical values **must multiply out**. The form derives the physical pair rather
than asking for it.

**Cores.** `Emulation.setHardwareConcurrencyOverride` plus a JS spoof of
`navigator.hardwareConcurrency`. Both, because the CDP override does not reach
every context.

**Memory.** JS only — there is no CDP command for `deviceMemory`. That leaves a
real hole: a worker that reads `WorkerNavigator.deviceMemory` sees the host. The
values are restricted to {1, 2, 4, 8} because the browser quantises RAM to a
power of two and caps it at 8; the measured tables' 16 GiB and 1.5 GiB entries are
installed RAM, which no `navigator` ever reports.

**Touch.** When `has_touch`: `Emulation.setTouchEmulationEnabled` with
`maxTouchPoints: 5`, and `setEmitTouchEventsForMouse` with the `Mobile`
configuration on a phone. That second command is what turns the automation's
pointer input into genuine `touchstart`/`touchmove`/`touchend` — a "phone" that
emits `mousemove` is not a phone.

**Power.** `navigator.getBattery` returns a synthetic manager: `charging` is
`!has_battery || has_mouse`, times are `Infinity`/`7200` on battery and
`0`/`Infinity` on mains, and the level is randomised 20–100% **per launch** —
deliberately not seeded, because a level frozen across sessions is its own tell.
The listener methods are present and inert; an object without them is not a
`BatteryManager`.

## How it plays with the rest of the suite

- **← User agent.** `Sec-CH-UA-Mobile` and `Sec-CH-UA-Model` come from the same
  `device_model` field that decides whether device metrics are sent at all.
- **← Graphics.** A phone GPU with a desktop screen, or a discrete GPU with 2 GiB
  of memory, describes no real machine. The handset binding removes the
  possibility on mobile; on desktop it is left to judgement.
- **← Behaviour.** A touch identity must be driven with taps and swipes, not
  clicks and wheel events. `setEmitTouchEventsForMouse` handles the translation,
  but the *pacing* is the automation skill's problem.
- **→ Locale.** Weakly: device class correlates with region in ways sophisticated
  scorers use (a 2015 Android budget phone in a wealthy metro is unusual).
- **→ Continuity.** Screen and resolution feed the fingerprint seed, so the device
  class is baked into the canvas and audio hashes.

## How it gets caught

1. **Logical × ratio ≠ physical.** The device metrics override computes one of
   them; a record that disagrees describes an impossible panel.
2. **`deviceMemory` of 16, 6, 3 or 1.5.** Only 1, 2, 4, 8 exist.
3. **Touch claimed, mouse events delivered** — or `maxTouchPoints: 5` with
   `(pointer: fine)` and `(hover: hover)`.
4. **A desktop that is always on battery**, or a phone that is never charging and
   never discharges.
5. **A battery level that never moves** across a long session, or moves backwards
   while charging.
6. **Cores and memory that do not match the named handset.** The model is public;
   its spec sheet is public.
7. **Window geometry that no real browser produces** — `outerHeight` equal to
   `innerHeight` (no chrome), or a viewport larger than the screen.
8. **A phone in landscape with a portrait screen record**, or an orientation API
   that disagrees with the dimensions.

## The desktop screen is the framebuffer, and the window needs a maximize

Nothing spoofs `screen` on desktop — no device metrics are sent there, so
`screen.width/height` is read straight off the X server. On a headless host that
means **`Xvfb`'s geometry is the persona's screen**, and a fixed framebuffer
gives every persona the same one no matter which the catalogue picked.

The window is a separate trap. On a bare X server with no window manager:

| asked for | `outerWidth` on a 1920×1080 screen |
| --- | --- |
| `--window-size=1920,1080` | **1919×1079** |
| `Browser.setWindowBounds` | ignored entirely — nothing services the request |
| `--start-fullscreen`, `--kiosk` | ignored — both need a window manager |
| `--window-size=1600,900` | 1600×900, exact |

Chrome will not make a window exactly the size of the screen; it lands a pixel
short. Any size *below* the screen is honoured exactly, so the off-by-one only
appears in the case you actually want. Left alone it is the same wrong number on
every session — one integer linking the whole fleet, which is worse than the
value being unusual.

**Run a window manager and maximize.** `openbox` plus `--start-maximized` gives
`outerWidth == screen.width` exactly, with an `innerHeight` that leaves realistic
room for the toolbar (1920×937 on a 1920×1080 screen). Fullscreen also fills the
screen but leaves `inner == outer`, which is a kiosk, not someone browsing.

## Gaps in this implementation

- `deviceMemory` is JS-only, so workers see the host value.
- `matchMedia` pointer/hover queries are not overridden; on a desktop host
  claiming a phone they will report `fine`/`hover`.
- `screen.availWidth/availHeight` and the outer/inner deltas are not managed on
  desktop — they come from the real window, so the framebuffer geometry and the
  maximize above are what set them.
- `screen.orientation` is untouched.
- Battery does not simulate charging progress or unplug events.
