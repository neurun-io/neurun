# Vector: Audio and Text Metrics

Two low-visibility, high-stability vectors. Neither is worth much alone; both are
excellent for **linking** — they change slowly, survive cookie clearing, and a
value shared by two "different" users proves they are one machine.

## AudioContext

### How it is read

Render a fixed oscillator graph through an `OfflineAudioContext`, read the
samples, hash them. The output varies with the audio stack, the CPU's floating
point behaviour and the build — enough entropy to bucket a machine, and stable
enough to link sessions months apart. Some libraries also read
`AnalyserNode.getFloatFrequencyData` on a live context.

### What this suite sets

- `AudioBuffer.getChannelData` — every sample shifted by
  `(rand(i) - 0.5) * 1e-7`, where `rand` is seeded from the identity.
- `AnalyserNode.getFloatFrequencyData` — shifted by `1e-4`, the scale that
  survives an FFT without being audible.
- The buffer is tracked in a `WeakSet` and noised **once**. `getChannelData` hands
  back the live array, so re-noising on every call would compound the error and
  break the two-pass comparison detectors rely on.

The magnitudes are chosen to move a hash without moving the audio: `1e-7` on a
float sample is far below any perceptual or functional threshold.

### How it gets caught

- **Compounding noise** — call `getChannelData` twice, get two different arrays.
  The `WeakSet` exists precisely to prevent this.
- **Noise that is too large** — a value that changes the rendered waveform enough
  to be audible, or that pushes samples outside the range the graph could produce.
- **Zero noise** — the host's real audio hash, shared with every other profile on
  the machine.
- **A hash that changes per session** — instability, again.

## Text metrics and fonts

### How it is read

Two different things, often conflated:

1. **Metric hashes** — measure a long string in a series of fonts and hash the
   widths. `measureText`, `getBoundingClientRect`, `offsetWidth`/`offsetHeight`.
   Sensitive to rasterisation, DPI and the font's actual metrics.
2. **The installed font set** — measure a string in font X with a known fallback;
   if the width equals the fallback's, X is absent. Enumerating a few hundred
   fonts yields a set that is strongly OS- and locale-specific, and often
   machine-specific.

### What this suite sets

- `measureText` — width perturbed by ±0.02% of the width, keyed by a hash of
  **text and font together**, so the same string in the same font always measures
  the same.
- `getBoundingClientRect` — width and height perturbed at the same scale, keyed by
  the original dimensions, returned as a fresh `DOMRect`.

The older extension patched `offsetWidth`/`offsetHeight` on `HTMLElement`
directly with a fixed offset, which changes layout the page itself can observe.
Metric-level perturbation is the better trade.

### The gap that cannot be closed from JS

**The font set is not hidden.** Equal-width fallbacks stay equal, so a probe still
learns which fonts exist. That is an OS/profile-level problem: it needs real font
control in the browser profile, not a JS patch. A Linux host claiming Windows
will fail a font-set probe no matter how good the metric noise is.

This is worth stating plainly when choosing an identity: **claim an OS whose font
set the host can actually present**, or accept that font-set probes will
disagree with the UA.

### A server ships almost no fonts, and that is the tell

The default is not a *wrong* font set, it is an empty one. A fresh Ubuntu server
carries on the order of a dozen families; a desktop carries hundreds. Chrome's
own runtime dependencies pull in only `fonts-liberation`, so a browser host built
from the package list alone presents a set no human desktop has ever had —
before any persona is chosen, and unaffected by every JS patch above.

Fix it in the image, since nothing else can. What an Ubuntu desktop actually
ships:

```
fonts-dejavu-core fonts-dejavu-extra fonts-liberation fonts-ubuntu
fonts-noto-core fonts-noto-color-emoji fonts-freefont-ttf
fonts-crosextra-carlito fonts-crosextra-caladea
```

Emoji matter more than their share of the list suggests: `fonts-noto-color-emoji`
is what makes an emoji canvas render like a desktop's instead of as tofu, and
emoji renders are where the canvas fingerprint's entropy is concentrated.

Count families with `fc-list : family | tr ',' '\n' | sort -u | wc -l` — under
about 20 says server, a few hundred says desktop.

## How they play with the rest of the suite

- **← Graphics.** Both are seeded from the same `FP_SEED`, so a persona's canvas,
  audio and text hashes move together. They should: they describe one machine.
- **← User agent.** The font set implies an OS. A macOS UA on a host with no
  macOS system fonts is caught by the vector the noise does not cover.
- **← Screen.** Text metrics scale with the device pixel ratio; a claimed DPR of
  3 with metrics measured at DPR 1 is inconsistent, though few detectors check.
- **→ Continuity.** These are the strongest *linking* vectors in the suite. If two
  profiles are meant to be strangers, they must not share an audio or text hash —
  which the per-identity seed guarantees, provided the seeded fields differ.

## Practical rules

1. Never re-seed per session. Stability is the whole point.
2. Never noise per call. Noise once per artefact, then return the same value.
3. Keep magnitudes sub-perceptual: `1e-7` audio samples, ±0.02% widths, ±1 on 3%
   of pixels.
4. Two personas that should be unrelated must differ in at least one seeded field
   — OS, release, GPU pair, id, resolution, cores, memory or platform. Two
   profiles built from the same handset and the same defaults share a seed.
5. Do not claim an OS whose fonts the host cannot present.
