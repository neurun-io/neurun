# Vector: Automation Artifacts

Everything above describes a machine. This vector asks a different question: **is
this environment being driven, and has it been tampered with?** It is scored
independently of the persona, so a perfect identity fails here on its own — and
it is the vector where spoofing badly is strictly worse than not spoofing.

## How it is read

| Probe | Looks for |
| --- | --- |
| `navigator.webdriver` | the standard flag |
| `Function.prototype.toString` on any native-looking function | injected source |
| `Object.getOwnPropertyDescriptor` / `getOwnPropertyDescriptors` / `hasOwn` / `hasOwnProperty` on "deleted" properties | ghosts of removed properties |
| getter `name`, `enumerable`, `configurable`, presence of a setter | descriptors that do not match a real accessor |
| `Error.stack` from inside a patched function | frames belonging to injected script |
| `window.chrome` shape, `chrome.runtime`, `chrome.loadTimes` | a Chrome that is not one |
| `navigator.plugins`, `mimeTypes` prototypes | plain objects posing as `Plugin` |
| `navigator.permissions.query({name:'notifications'})` vs `Notification.permission` | the classic headless mismatch |
| codec support, `matchMedia('(prefers-reduced-motion)')`, `WebGL` presence | headless gaps |
| iframe and worker contexts | patches applied only to the main world |
| CDP side effects — `Runtime.enable`, exposed bindings, extra `console` behaviour | the protocol itself |

## What this suite sets

**Remove the tell at the source.** `--disable-blink-features=AutomationControlled`
is passed at launch, so `navigator.webdriver` is already `false`. The injected
script patches it **only if it is still true** — which happens when attaching to
a browser the flag never reached. A conditional patch is deliberately better than
an unconditional one: a `webdriver` getter that stringifies oddly is worse than a
correct native `false`.

**The toString cloak** (`property_modifier.js`) is the load-bearing piece:

- `Function.prototype.toString` is replaced with a Proxy that returns
  `function {name}() { [native code] }` for any function registered through
  `markNative`, and defers to the original for everything else. The Proxy itself
  is marked native.
- `Object.getOwnPropertyDescriptor`, `Reflect.getOwnPropertyDescriptor`,
  `Object.getOwnPropertyDescriptors`, `Object.hasOwn` and
  `Object.prototype.hasOwnProperty` are patched so a removed property is absent
  through **every** path, not just direct access. All five are marked native.
- Spoofed getters are installed with `set: undefined`, `configurable: true`, the
  original's `enumerable`, and a `name` of `get {property}`.

**Scope containment.** The whole bootstrap is wrapped in an IIFE, so
`PropertyModifier` and the per-brand locals never become global lexical bindings
a page can probe for. It is registered with
`Page.addScriptToEvaluateOnNewDocument`, so it runs before page JS in every
document — including iframes.

**Per-brand shape.** Chrome gets three plugins; Edge gets an empty list; Safari
and all of iOS get `window.chrome` removed, `navigator.vendor` set to
`Apple Computer, Inc.`, `window.safari.pushNotification` added with a matching
`toString`, and `userAgentData` and `deviceMemory` deleted.

## How it plays with the rest of the suite

- **It gates every other vector.** If `toString` leaks, the graphics, audio and
  navigator patches are not merely detected — they are *evidence*, and far more
  damning than the unspoofed values would have been.
- **← User agent.** The brand block must match the claimed brand: `window.chrome`
  present on a Safari UA, or absent on a Chrome UA, is a contradiction.
- **← Device class.** Headless gaps (missing codecs, no GPU, odd `matchMedia`)
  contradict a claimed consumer machine.
- **→ Behaviour.** Perfect artifact hygiene with robotic pointer movement fails
  anyway; the two are scored together and the automation skill covers the second.

## How it gets caught

1. **An unmarked patch.** One function whose `toString` returns source
   characterises the environment. Everything you add must go through
   `markNative`.
2. **Descriptor archaeology.** A property deleted from `navigator` but still
   visible via `Object.getOwnPropertyDescriptors(Navigator.prototype)`.
3. **Prototype identity.** `navigator.plugins[0] instanceof Plugin`,
   `PluginArray.prototype.item.call(navigator.plugins, 0)` — the plugin list here
   is plain objects, and deep probes see through it.
4. **Stack traces.** An exception thrown inside a patched accessor carries frames
   from the injected script unless the patch is careful.
5. **Worker and iframe contexts.** Patches that only reach the main world leave a
   worker reading the host's `deviceMemory` — a known hole here.
6. **Permissions/Notification mismatch**, still one of the cheapest headless
   checks and not addressed by this suite.
7. **CDP artifacts.** Driving over CDP has observable side effects; a target that
   watches for them sees a debugger attached regardless of the JS layer.
8. **Over-spoofing.** Patching `webdriver` when the flag already handled it,
   noising a canvas nobody drew on, or claiming plugins on a browser that has
   none — each creates a value no real browser produces.

## Gaps in this implementation

- Workers are not covered; `deviceMemory` in particular is JS-main-world only.
- Plugin objects are not real `Plugin` instances.
- No `permissions`/`Notification` reconciliation.
- `Error.stack` scrubbing is not attempted.
- `update_gpu.js` (the runtime GPU re-patch) does not `markNative` — it defeats
  the cloak for `getParameter` if used mid-session.
- Nothing hides the fact that CDP is attached; that is a property of the
  transport, not of the page.
