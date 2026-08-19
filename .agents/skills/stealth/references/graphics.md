# Vector: Graphics — WebGL and Canvas

The highest-entropy vector in the suite, and the one every commercial detector
reads. It has two halves that are usually confused: **what the GPU claims to be**
(strings) and **what it actually draws** (pixels). Spoofing one without the other
is the classic half-measure.

## How it is read

| Probe | Returns | Entropy |
| --- | --- | --- |
| `getParameter(37445)` `UNMASKED_VENDOR_WEBGL` | vendor string | moderate — a few dozen values |
| `getParameter(37446)` `UNMASKED_RENDERER_WEBGL` | renderer string | high — thousands, and it names the driver stack |
| `getParameter(3379, 34076, 3386, 34024, …)` | max texture size, viewport dims, buffer limits | moderate, and **tied to the claimed card** |
| `readPixels` after rendering a known scene | raw pixels | very high |
| `canvas.toDataURL` / `toBlob` after drawing text and shapes | PNG bytes | very high |
| `getImageData` | raw pixels | very high |
| `getSupportedExtensions`, `getShaderPrecisionFormat` | capability list | moderate, tied to the card |

The renderer string is not just an identifier — it is a *claim about the whole
stack*. `ANGLE (…, Direct3D11 vs_5_0 ps_5_0)` says Chromium on Windows;
`… OpenGL ES 3.2` says Android; a bare `Apple M2` says WebKit or Chrome on a Mac.

## What this suite sets

**Strings.** `main_world.js` replaces 37445 and 37446 with the identity's
`webgl_vendor` and `webgl_renderer` on both `WebGLRenderingContext` and
`WebGL2RenderingContext`, passing every other parameter through untouched.

| Platform | `webgl_renderer` | `webgl_vendor` |
| --- | --- | --- |
| Windows · NVIDIA | `ANGLE (NVIDIA GeForce RTX 3080 Direct3D11 vs_5_0 ps_5_0)` | `Google Inc. (NVIDIA)` |
| Windows · Intel | `ANGLE (Intel(R) UHD Graphics 630 Direct3D11 vs_5_0 ps_5_0)` | `Google Inc. (Intel)` |
| Windows · AMD | `ANGLE (AMD Radeon RX 6600 Direct3D11 vs_5_0 ps_5_0)` | `Google Inc. (AMD)` |
| Intel Mac | `Intel Iris OpenGL Engine` | `Intel Inc.` |
| Apple silicon | `Apple M2` | `Apple` |
| iOS | `Apple A16 GPU` | `Apple` |
| Android | `ANGLE (Qualcomm, Adreno (TM) 750, OpenGL ES 3.2)` | `Google Inc. (Qualcomm)` |

**Pixels.** `hardware.js` forces the low bit of R, G and B — a deterministic
function of `(FP_SEED, pixel index)`, never `Math.random()` — in `getImageData`,
and in `toDataURL`/`toBlob`/`convertToBlob` via a **noised clone** so the source
canvas is never mutated. The clone is produced with `drawImage`, which also
covers WebGL-backed canvases.

Three restrictions carry the whole thing, and each exists because a one-line
probe catches its absence:

- **Only pixels differing from their left neighbour, at alpha 255.** A solid
  `fillRect` must read back perfectly uniform and an untouched canvas must be
  all zero. Speckling either is louder than the true hash ever was. Antialiased
  glyph edges — where the entropy actually lives — still qualify.
- **Forcing, not adding.** The write is idempotent, so a canvas read twice
  through different paths (direct `getImageData` vs encode→decode→draw→read)
  agrees with itself. Real hardware round-trips PNG losslessly.
- **Nothing below 16×16.** Detectors render a tiny fixed shape *because* it is
  low entropy and identical on every machine of an engine, which is what makes a
  known-good table possible. Moving it off the table is the signal.

**`readPixels` is deliberately not noised** — see the removals below.

## How it plays with the rest of the suite

- **← User agent.** The renderer must match the OS the UA claims. Direct3D on a
  Mac, or a bare `Apple M2` on Windows, is a contradiction a single regex finds.
  The catalogue enforces this structurally: a card is listed under the system or
  the handset that reports it, and nowhere else.
- **← Brand.** ANGLE is a Chromium layer. A Safari identity that reports an ANGLE
  string is claiming a translation layer WebKit does not use.
- **← Hardware.** A high-end discrete GPU next to 2 cores and 2 GiB of memory
  describes a machine nobody built.
- **← Screen.** A 4K panel with an integrated GPU from 2015 is unusual; a phone
  GPU with a 1920×1080 logical screen is impossible.
- **→ Stability (continuity).** The pixel noise is seeded from the GPU pair among
  other fields, so the canvas hash *is* part of the persona's identity. Changing
  the card changes the hash — correct, and the reason a GPU edit is a new
  persona rather than a tweak.
- **→ Behaviour.** None. This vector is read before any interaction, which is
  why it is usually the one that gets a run blocked on the first request.

## How it gets caught

1. **Strings spoofed, pixels not.** Claimed NVIDIA, rendered output bit-identical
   to the host's Intel iGPU — and to every other bot on the same host.
2. **Pixels noised, strings not.** The reverse: a unique canvas hash attached to a
   renderer string shared by a million machines.
3. **Unstable noise.** Two reads in one page disagree. Real hardware is
   deterministic; a hash that moves is proof of instrumentation.
4. **A blank canvas that is not blank.** Noising a canvas nobody drew on produces
   a hash no real machine has. Gate noise on an actual draw operation —
   `fillText`, `strokeText`, `arc`, `bezierCurveTo`, `createLinearGradient`,
   `transform`, `rotate` and friends — or on a WebGL context.
5. **Parameters that contradict the card.** `MAX_TEXTURE_SIZE` and precision
   formats come from the *real* GPU because only 37445/37446 are patched. A
   detector that checks whether the limits are plausible for the claimed renderer
   sees the host.
6. **Software renderers.** `Microsoft Basic Render Driver`, `SwiftShader`,
   `llvmpipe`. That is a VM or a headless container, and it is excluded from the
   catalogue for exactly this reason.
7. **`getParameter` stringifying to injected source.** Covered by the `toString`
   cloak — see `automation-artifacts.md`.

## Getting a context at all on a host with no GPU

A server has no GPU, and Chrome's default path there ends in **no WebGL**:
ANGLE tries its bundled SwiftShader over Vulkan, which fails to initialize
headless, and Chrome then blocklists the software renderer it falls back to. The
symptom is `canvas.getContext("webgl")` returning `null`.

That is worse than any renderer string. The highest-entropy vector is simply
absent on a desktop persona that must have it, and the vendor/renderer
substitution has nothing to apply to — a context that does not exist cannot be
spoofed, and the canvas noise has nothing to work on.

`rustenium-identity` therefore launches with:

```
--use-gl=angle --use-angle=gl --ignore-gpu-blocklist
```

`--use-angle=gl` points ANGLE at the system GL — Mesa's llvmpipe on a headless
Linux box — and `--ignore-gpu-blocklist` is what actually unblocks it, since the
renderer is software. `--enable-unsafe-swiftshader` is the wrong lever: it
applies to the SwiftShader backend, which is the one that could not start.

Diagnose with `SystemInfo.getInfo` over CDP rather than guessing. It reports
`featureStatus.webgl`, and the two answers mean different things: `disabled_off`
is a switch or blocklist, `disabled_software` is a fallback. Chrome's own
`glRenderer` there tells you which backend really came up.

## Claim the driver stack you actually have

Only the two strings are substituted, so every other answer — the limits, the
extension list, the precision formats — comes from the real driver. The size of
the hole is therefore the **distance between the card you claim and the stack
underneath it**, and that distance is a choice.

The useful part: `llvmpipe` is a **Mesa** driver. Its surface is shaped by Mesa,
not by silicon — measured on a headless Ubuntu box, 32 extensions with a
desktop-GL flavour (`EXT_clip_control`, `WEBGL_polygon_mode`,
`WEBGL_provoking_vertex`, `EXT_depth_clamp`), `MAX_TEXTURE_SIZE` 16384,
`MAX_VERTEX_UNIFORM_VECTORS` 1024.

Intel and AMD GPUs **on Linux run that same Mesa stack**. Different silicon, same
driver, so nearly the same extension list and limits. NVIDIA's proprietary driver
is a separate codebase with its own extension set and its own limits.

So the ranking, cheapest fix first:

| Claim | Distance from llvmpipe |
| --- | --- |
| Intel iGPU on Mesa | closest — same driver, modest limits |
| AMD integrated on Mesa (`radeonsi`) | close — same driver |
| AMD/NVIDIA discrete | further — same or different driver, much larger limits |
| NVIDIA on the proprietary blob | furthest — different driver entirely |

The catalogue's Linux entry therefore offers Mesa integrated cards only. Naming a
GeForce costs nothing to type and is the one claim the host cannot back up on any
axis.

Match the Mesa version to the release too: Ubuntu 26.04 ships Mesa 26.0.x, so a
renderer string naming 24.2.3 is implausible before anyone looks at the GPU.

## Gaps in this implementation

- Only 37445 and 37446 are spoofed. Limits, extension lists and precision formats
  still describe the real driver. Choosing a card on the same stack (above)
  narrows this; it does not close it.
- **Rendered pixels and speed cannot be spoofed at all.** A software renderer
  draws what it draws, and it is orders of magnitude slower than the card being
  claimed — a page that times a heavy render sees it. Nothing in JS fixes either;
  only real hardware does.
- Nothing coordinates the canvas hash with the *claimed* GPU: two personas with
  the same card get different hashes, which is right, but a persona's hash is not
  derived from anything a real card would produce.
