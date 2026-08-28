---
status: current
system: ui
requirements:
  - REQ-UI-PERSISTENT-STATUS-MOTION-001
  - REQ-UI-PERSISTENT-STATUS-MOTION-002
  - REQ-UI-PERSISTENT-STATUS-MOTION-003
---

# Persistent Status Motion System Design

## Purpose and boundaries

This design moves persistent task, session, and run motion from main-thread CSS
sampling to compositor-backed HTML animation targets. It changes rendering
ownership only. Task state, icon selection, copy, motion design, and interaction
behavior do not change.

The shared grid component is covered because a single instance can remain
mounted for the lifetime of a running turn and its nine CSS animations were the
dominant recurring source in the captured steady-state trace. Optimizing the
shared component also benefits bounded uses without changing their contract.
One-shot attention and transition effects remain outside this design.

## Requirement mapping

| Requirement                           | Design section                                                                                                                                |
| ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-PERSISTENT-STATUS-MOTION-001` | [Motion primitive](#motion-primitive), [Status surfaces](#status-surfaces), [Desktop and mobile composition](#desktop-and-mobile-composition) |
| `REQ-UI-PERSISTENT-STATUS-MOTION-002` | [Grid activity motion](#grid-activity-motion), [Animation lifecycle and fallback](#animation-lifecycle-and-fallback), [Verification and performance evidence](#verification-and-performance-evidence) |
| `REQ-UI-PERSISTENT-STATUS-MOTION-003` | [Opacity pulse motion](#opacity-pulse-motion), [Desktop and mobile composition](#desktop-and-mobile-composition), [Verification and performance evidence](#verification-and-performance-evidence) |

## Motion primitive

`apps/packages/ui/src/compositor-spin.tsx` owns a small presentation primitive.
It renders an `inline-flex` HTML `span` with the existing transform rotation and
the `will-change: transform` hint. The child SVG stays static and inherits its
size and color from the wrapper.

The wrapper retains the `animate-spin` class for the existing selector and
compatibility contract. In browsers with `Element.animate`, the primitive reads
the duration from that class, disables the CSS animation, and starts an
infinite linear Web Animations API animation from `rotate(0deg)` to
`rotate(360deg)` on the HTML element. It applies the transform promotion hint
and cancels the animation when the primitive is cleaned up. If Web Animations
are unavailable, the retained CSS animation remains the fallback.

The wrapper owns animation classes, dimensions, margins, status test IDs, and
other non-SVG attributes. The SVG owns only its path and `aria-hidden` state.
This split gives Chromium a stable HTML transform target that it can promote to
a compositor layer.

The primitive accepts normal wrapper attributes and children. It does not
select task state, add labels, or own status precedence.

## Grid activity motion

`apps/web/components/grid-spinner.tsx` keeps its existing status wrapper and
nine `.spinner-grid-cube` HTML children. The component establishes one Web
Animations API transform effect per cube when `Element.animate` is available.
The effects use the current scale keyframes, 1.3-second duration, easing, and
per-cell stagger from `apps/web/app/globals.css`.

The CSS classes remain the visual and compatibility contract. The component
reads the computed CSS timing before it disables CSS sampling on each live
animation target. The Web Animations effects then own the same staggered motion.
The grid wrapper retains its dimensions, `role="status"`, translated accessible
label, and public class names. The cube elements remain decorative.

The component does not replace the grid with a rotating icon, static state,
canvas, or request-animation-frame loop. It does not add permanent
`will-change` declarations unless the repeat trace demonstrates that Chromium
needs them; nine unnecessary promoted layers would trade CPU for memory.

## Opacity pulse motion

`apps/packages/ui/src/compositor-pulse.tsx` owns a narrow HTML opacity-motion
primitive patterned after `CompositorSpin`. It accepts ordinary span attributes
plus the minimum opacity needed by the surface. It reads the current CSS
duration and easing, disables CSS sampling only after setup, and starts an
infinite Web Animations API opacity effect on the HTML span.

The chat composer replaces its animated `::after` pseudo-element with an
`aria-hidden` absolute HTML pulse target inside the existing isolated wrapper.
The target retains the current running and starting glow classes, box shadows,
opacity bounds, and two- or three-second cadence. The editor, resize handle,
focus behavior, and input geometry remain unchanged.

Long-lived `animate-pulse` status dots on task, session, agent, and run surfaces
use `CompositorPulse` after a call-site audit. Short request feedback and
one-shot attention cues remain on their existing implementations. State owners
continue to decide when each target mounts; the primitive owns only motion.

## Animation lifecycle and fallback

Each Web Animations effect starts after mount and is cancelled during effect
cleanup or before replacement. A duration or active-state change first restores
the CSS declaration, reads the new computed timing, and then replaces the live
effect. Grid presentation-class changes leave the existing effects in place so
they do not reset the visible stagger. This prevents stale inline `animation:
none` from suppressing later motion.

If `Element.animate` is unavailable, CSS keeps the existing animation. If the
computed animation name is `none`, including under a surface's existing
reduced-motion media rule, the compositor path does not start a replacement
effect. A mounted target also subscribes to `prefers-reduced-motion` changes:
enabling reduced motion cancels the Web Animations effect and restores the CSS
declaration, while disabling it allows the target to establish the compositor
effect again. This preserves current reduced-motion behavior without creating a
new setting.

## Status surfaces

The first migration covers persistent indicators that can remain mounted in an
active Kandev task view:

- shared task and session icons in `apps/web/lib/ui/state-icons.tsx`;
- sidebar task rows in `apps/web/components/task/task-item.tsx`;
- task board cards in `apps/web/components/kanban-card-content.tsx`;
- focused-task topbar, session timeline, agent-turn, and queued-run indicators
  in `apps/web/components/task/simple/components/`;
- task-list and Office task, agent, and run status rows that use the same
  long-lived domain states.

Implementation starts with a call-site audit. A spinner is in scope only when
its lifetime follows task, session, agent, or run state. A spinner that follows
one bounded UI request stays unchanged.

`getTaskStateIcon` and `getSessionStateIcon` keep their current selection and
precedence logic. Their icon configuration separates static icon classes from
the Boolean rotation state. Rotating configurations use the shared wrapper.

## Desktop and mobile composition

Desktop and mobile task switchers share task-row rendering. The wrapper keeps
the current inline size and does not add a new touch target, scroll owner, or
layout branch. Focusable tooltip wrappers and accessible labels stay outside
the rotating element.

The grid spinner and composer glow also keep the existing shared desktop/mobile
components. The closest mobile exemplars are the Quick Chat tab activity state
and the task chat composer. No presentation branch changes: the existing dialog
or task surface remains the scroll owner, current touch targets remain in place,
and the motion target stays pointer-inert. Mobile coverage uses those surfaces
and confirms that the document has no horizontal overflow.

## Verification and performance evidence

Component tests assert that the animation class and transform hint are on an
HTML wrapper. They also assert that the nested SVG has no animation class.
Existing state-precedence tests continue to validate which icon appears.

Grid component tests stub `Element.animate` and assert nine infinite effects,
the existing keyframes and stagger, cleanup cancellation, preserved semantics,
and CSS fallback. Pulse tests assert computed timing, opacity bounds, cleanup,
CSS fallback, and suppression when the computed animation name is `none`.

Desktop Playwright coverage holds a task in its running state. It checks the
rotating wrapper and then checks the settled icon after the state changes.
Mobile Playwright coverage checks the same wrapper in the task switcher and
confirms the existing touch navigation.

Desktop and mobile Playwright coverage also holds a Quick Chat or task turn in
a running state. It checks that grid cells and pulse targets have running
infinite Web Animations effects, that motion ends with the owning state, that
the same user-visible status remains, and that mobile geometry does not
overflow.

After implementation, capture the same steady focused-task state in Chromium.
Attribute recurring `UpdateLayoutTree`, `Layerize`, and frame activity to
individual animation targets. In the production control, keep the live
Web Animations API target running while disabling unrelated grid and persistent
status animations, wait for the page to settle, and inspect the following
8.34-second window. That window records zero recurring layout-tree or layerize
events and no target invalidations. With the unrelated grid animation enabled,
the page records 150 of each event and 1,350 grid-cube invalidations. A CSS
animation control for the same target records 41 of each event, while the Web
Animations API path records only one-time setup activity. The remaining
enabled-page frame work is therefore attributed to the unrelated grid
animation, not the migrated status target.

The next acceptance capture repeats the same production-build protocol. After
one second of settling, it records an 8.34-second window with one grid spinner
and the running composer glow visible. Node attribution must show no recurring
`UpdateLayoutTree` or `Layerize` work from their Web Animations targets. A CSS
fallback control must remain visibly animated and provides the comparison when
the Web Animations path is disabled.

## Failure and compatibility

If a browser does not provide Web Animations, the retained CSS animation still
rotates the wrapper normally. No state or content is lost. The duration is
read from the existing CSS class on the Web Animations path, so speed and
visual timing remain compatible.

The same fallback rule applies to grid and opacity motion. If an effect cannot
be established, the component leaves the CSS animation intact rather than
showing a static indicator. Partial grid setup cancels effects already created
and restores CSS for all nine cells so one grid never mixes timing engines.

The wrapper preserves existing test IDs and semantic attributes. Tests and
assistive technology do not need to depend on the nested SVG element.
