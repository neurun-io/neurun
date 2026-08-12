# Unit: Input Dispatch

The mechanics every behaviour is built from. Input goes through CDP
(`Input.dispatchKeyEvent`, `dispatchMouseEvent`, `dispatchTouchEvent`) so the
events are real to the page — but the *fields you fill in* and the *timing
between them* are what separate a session that reads as a person from one that
does not.

## Keyboard

A key press is at least two events, and every field matters:

| Field | Why |
| --- | --- |
| `type` | `keyDown` when the key produces text, `rawKeyDown` when it does not. Getting this wrong makes modifiers produce phantom input |
| `text`, `unmodifiedText` | what the key produces, with and without modifiers |
| `code`, `key` | physical key and logical value — `KeyA` vs `a` |
| `windowsVirtualKeyCode` | still read by older handlers |
| `location` | 0 for standard, 3 for the keypad, and it decides `is_keypad` |
| `modifiers` | a live bitmask: Alt 1, Control 2, Meta 4, Shift 8 |
| `autoRepeat` | **true on repeats**, false on the first press |

Modifier state is stateful: set the bit on the modifier's own `keyDown`, clear it
on `keyUp`, and pass the current mask on every event in between. A `Shift+A` that
never reported the Shift bit is a synthetic press.

**Held keys.** A real held key fires once, pauses ~500 ms, then repeats at
40–70 ms intervals with `autoRepeat` set. Arrow-key scrolling built on a held key
must model that, or it produces a scroll cadence no keyboard makes.

**Typing rhythm.** Inter-keystroke intervals are not uniform: digraph timing
varies with the key pair, and real typists pause at word boundaries and make
corrections. A constant delay per character is the tell.

## Mouse

**Position is stateful.** The dispatcher tracks `x`/`y` and interpolates from the
last position — a `mouseMoved` that teleports across the viewport is not a move,
it is a jump. Emit intermediate points.

**A click is three events** — move, `mousePressed`, `mouseReleased` — with the
button, `clickCount`, and the modifier mask. Dispatching `mousePressed` at a
coordinate the pointer was never at is the crudest possible click.

**Where to click.** Land on a random point *inside* the target's box, inset from
the edges, not the geometric centre. Click scatter within an element is scored.

## Wheel

`type: mouseWheel` with `deltaX`/`deltaY` at the current position. Three things
matter:

1. **Pace.** Reading, ordinary scrolling and fast scrolling are different
   distributions, not one constant. Approximate latencies from a working
   implementation: reading ~0.7–3.0× the delta, scrolling ~0.7–1.6×, fast
   scrolling ~0.08–0.7×.
2. **Delta variance.** Real wheels emit uneven deltas; occasionally doubling the
   delta mid-read models a flick.
3. **Animation.** A smooth scroll is many small deltas over a bezier-shaped
   velocity curve — accelerate, cruise, decelerate — not N identical steps.

An alternative worth mixing in: scroll by arrow key for a minority of scrolls.
Real users do both.

## Touch

When the identity has touch, `Emulation.setTouchEmulationEnabled` (5 points) and
`setEmitTouchEventsForMouse` are set at launch, so pointer input is translated
into `touchstart`/`touchmove`/`touchend`. That handles the *event type*; it does
not handle the *shape*:

- A tap is a touch down and up at one point, with a short, variable dwell.
- A swipe is a touch move with momentum — accelerating, then decaying — not a
  linear drag.
- Touch coordinates land inside the target with the same scatter as clicks,
  usually with more spread: fingers are less precise than mice.
- A touch device does not hover. Anything that depends on `mouseover` before a
  tap is describing a device that is not there.

## Coordinate spaces

Three different spaces, and mixing them is a common bug:

| Space | Origin |
| --- | --- |
| Element | the element's own box |
| Viewport | the top-left of the rendered page — what CDP input takes |
| Screen | the physical display — what an OS-level pointer library takes |

CDP dispatch is viewport-relative. If you drive a real OS pointer instead (some
implementations do, to defeat "trusted event" checks), everything must be
converted through the document's offset from the screen, and the window must be
in front. Scroll the element into view first, then convert — an element's
position is only meaningful after the scroll settles.

## Trusted events

CDP-dispatched events carry `isTrusted: true`, which is why this path exists at
all — `element.click()` and synthetic `MouseEvent`s do not. Never fall back to
`el.click()` for anything a detector might watch; use it only where the click is
incidental and the page is not adversarial.

## Checklist

1. Modifier mask maintained across the whole sequence?
2. `keyDown` vs `rawKeyDown` chosen by whether text is produced?
3. `autoRepeat` set on repeats, with a realistic delay and interval?
4. Pointer interpolated from its last position, never teleported?
5. Click preceded by a move to a scattered point inside the target?
6. Wheel deltas uneven, paced by intent, animated rather than stepped?
7. On a touch identity: taps and swipes, no hover assumptions?
8. Coordinates in the space the API you are calling expects?
