# Unit: Human Behaviour

Human behaviour depict how human perform actions on the web and how it should translate / influence automation sub-commands. Which, if you mimick the sub-commands should get a result as a human performing said actions

## Pointer movement

A real pointer follows a curve with knots, distortion and a correction at the
end. The working implementation generates a human curve between the current
position and the target with:

- **Overshoot proportional to distance.** A base offset plus roughly a quarter of
  the distance, jittered ±20%, then a correction back. Long moves overshoot;
  short ones barely do.
- **Knot count and target points scaled by distance** — a 40 px nudge and a
  1200 px sweep are not the same gesture sampled differently.
- **Distortion** applied along the path (mean ~0.2, stdev ~0.5, applied to a
  fraction of points), so the curve is not a clean bezier.
- **Duration that scales with distance**, in the 0.2 s–0.4 s × distance range, not
  a constant.
- **Probability of overshoot as a parameter**, randomised per move rather than
  always or never.

**Landing.** Into a random point inside the element's box, inset from the edges,
not the centre. Between actions, the pointer does not stay where it was: real
pointers drift.

## Reading and dwell

Time on page is not `sleep(n)`. Model it as a **budget spent against content**:

1. Take the element's scrollable height and a reading speed (px/second) to get a
   total time.
2. Compute an average time per pixel.
3. Advance in chunks of roughly 0.7–1.0 of the viewport height, sleeping the
   chunk's share of the budget.
4. With ~20% probability per chunk, scroll **back up** 15–35% of a viewport, pause
   0.5–2.5 s, then return — "wait, what did that say". This single behaviour is
   the difference between reading and scrolling.
5. Track time owed when an interruption eats the budget, and pay it back rather
   than silently reading faster.

Two things fall out of doing it this way: dwell correlates with content length
(a short page is read quickly), and no two runs produce the same scroll trace.

## Scroll mode follows the identity

- Touch identity → swipes, with momentum.
- Mouse identity → wheel most of the time, arrow keys occasionally (~10%).
- Wheel deltas vary, and the pace differs between reading and travelling.

Scrolling with a wheel event on a device that reports `maxTouchPoints: 5` is a
cross-vector contradiction, not a stylistic choice.

## Idle behaviour

A person between actions is not a process between actions. Things worth
modelling, all from the working implementation:

- Drifting the pointer to a random point in the document or over the element
  being read.
- Moving toward the window's exit region — the "am I done here" gesture that
  exit-intent scripts watch for, and whose *absence* is itself unusual on a real
  session.
- Following a link because it was there, rather than because the script needed
  it.
- Occasional clicks on non-interactive space at the start of a scroll.

These also serve a mechanical purpose: they keep the pointer's position history
plausible so the next real interaction does not start from a suspicious place.

## Concurrency and the shared pointer

If input is driven at OS level rather than through CDP, **the pointer is a single
shared resource across every session on the host**. The working implementation
guards it with a cross-process value so only one bot moves the mouse at a time,
brings its window to the front first, and releases the guard afterwards.

Two consequences: sessions serialise on pointer work whether you meant them to or
not, and a session whose window is not in front is dispatching input into
somebody else's page. CDP dispatch avoids all of this, at the cost of not
defeating checks that compare against OS-level input.

## Cadence across runs

Behaviour is also what happens between sessions:

- Sessions of near-identical length are a fleet signature.
- Every persona active in the same narrow window is a fleet signature.
- A persona that only ever does one thing, in one order, is a script.

Nothing in this stack schedules runs, so this belongs to whatever does. →
`stealth` skill, `references/continuity.md`.

## How it gets caught

1. **Straight lines and constant velocity.** The cheapest tell there is.
2. **Centre-of-element clicks**, every time.
3. **Zero-variance timing** — identical dwell, identical inter-key delay,
   identical scroll steps.
4. **No idle behaviour at all.** A pointer that only ever moves to targets.
5. **Instant interaction after load**, before a person could have read anything.
6. **Impossible sequences** — a click on an element that was never scrolled into
   view, a tap on a hover-revealed menu, typing into a field that was never
   focused.
7. **Honeypot contact** — interacting with elements a human cannot see. Check
   computed visibility before acting, not just presence in the DOM.
8. **Perfect success rate.** Never mis-clicking, never scrolling past, never
   correcting.

## Checklist

1. Curved, distance-scaled pointer moves with occasional overshoot?
2. Landing points scattered inside targets?
3. Dwell budgeted from content, spent in viewport chunks, with re-reads?
4. Scroll mode matching the identity's input model?
5. Some idle behaviour between actions?
6. Element scrolled into view *and* visible before interaction?
7. Variance everywhere a constant would otherwise sit?
