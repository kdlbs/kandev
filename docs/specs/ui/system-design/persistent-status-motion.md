---
status: current
system: ui
requirements:
  - REQ-UI-PERSISTENT-STATUS-MOTION-001
---

# Persistent Status Motion System Design

## Purpose and boundaries

This design moves persistent task, session, and run rotation from SVG elements
to a shared HTML transform wrapper. It changes rendering ownership only. Task
state, icon selection, copy, and interaction behavior do not change.

Short request spinners remain outside this design. They have bounded lifetimes
and were not a material source in the captured steady-state trace.

## Requirement mapping

| Requirement                           | Design section                                                                                                                                |
| ------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-PERSISTENT-STATUS-MOTION-001` | [Motion primitive](#motion-primitive), [Status surfaces](#status-surfaces), [Desktop and mobile composition](#desktop-and-mobile-composition) |

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

The closest mobile exemplar is the existing task-switcher row. Mobile coverage
uses this shared row and also confirms that the document has no horizontal
overflow.

## Verification and performance evidence

Component tests assert that the animation class and transform hint are on an
HTML wrapper. They also assert that the nested SVG has no animation class.
Existing state-precedence tests continue to validate which icon appears.

Desktop Playwright coverage holds a task in its running state. It checks the
rotating wrapper and then checks the settled icon after the state changes.
Mobile Playwright coverage checks the same wrapper in the task switcher and
confirms the existing touch navigation.

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

## Failure and compatibility

If a browser does not provide Web Animations, the retained CSS animation still
rotates the wrapper normally. No state or content is lost. The duration is
read from the existing CSS class on the Web Animations path, so speed and
visual timing remain compatible.

The wrapper preserves existing test IDs and semantic attributes. Tests and
assistive technology do not need to depend on the nested SVG element.
