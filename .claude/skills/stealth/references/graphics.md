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

**Pixels.** `hardware.js` perturbs about 3% of pixels by ±1 on R, G and B (never
alpha — that would change compositing) in `getImageData`, in `readPixels` on both
WebGL contexts, and in `toDataURL`/`toBlob` via a **noised clone** so the source
canvas is never mutated. The clone is produced with `drawImage`, which also
covers WebGL-backed canvases.

Every noise value is a pure function of `(FP_SEED, pixel index)`. No
`Math.random()` is in the path.

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

## Gaps in this implementation

- Only 37445 and 37446 are spoofed. Capability limits still describe the host.
- `update_gpu.js`, the runtime re-patch, does not `markNative`, so a page that
  stringifies `getParameter` after a mid-session GPU change sees the patch.
- Noise is applied unconditionally rather than only after a draw.
- Nothing coordinates the canvas hash with the *claimed* GPU: two personas with
  the same card get different hashes, which is right, but a persona's hash is not
  derived from anything a real card would produce.
