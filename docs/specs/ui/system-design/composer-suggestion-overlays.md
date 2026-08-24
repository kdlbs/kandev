---
status: current
system: ui
updated: 2026-08-24
requirements:
  - REQ-UI-COMPOSER-OVERLAY-001
---

# Composer Suggestion Overlay System Design

## Purpose and boundaries

The shared `PopupMenu` primitive positions composer suggestion surfaces from a
direct point or an editor-provided client rectangle. This design corrects its
vertical geometry when mobile browsers retain layout-viewport caret coordinates
below a software-keyboard-reduced visual viewport.

The primitive owns viewport containment, maximum size, reflow subscriptions,
listbox structure, touch-row size, and focus-preserving pointer behavior. Each
consumer continues to own trigger recognition, result loading, selection, and
draft serialization.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-COMPOSER-OVERLAY-001` | [Geometry and reflow](#geometry-and-reflow), [Interaction contract](#interaction-contract) |

## Components and responsibilities

- `PopupMenu` in `apps/web/components/task/chat/popup-menu.tsx` resolves the
  anchor, reads viewport bounds, subscribes to viewport changes, portals the
  surface to `document.body`, and renders one labelled listbox.
- `computePopupMenuStyle` performs deterministic horizontal and vertical
  geometry without reading the DOM. It is the unit-test boundary for viewport
  containment.
- `MentionMenu`, `SlashCommandMenu`, and `EntityReferenceMenu` use the default
  above-anchor placement for chat and shared prompt-composer suggestions.
- `PlanSlashMenu` uses the same primitive with below-anchor placement. Its
  placement semantics are not changed, but unit coverage must protect that path
  from a shared-geometry regression.

## Data and contracts

`PopupMenu` accepts one of two transient anchor contracts:

- a direct `{x, y}` fixed-position point; or
- a `clientRect` callback whose top or bottom edge supplies the placement point.

The browser's `window.visualViewport`, when present, supplies `offsetLeft`,
`offsetTop`, `width`, and `height`. Otherwise `window.innerWidth` and
`window.innerHeight` supply a zero-offset fallback viewport. No value is stored
or sent over an API.

The existing placement contract remains explicit: `above` renders the
surface's bottom edge above its normalized anchor; `below` renders the surface's
top edge below its normalized anchor. This repair does not introduce automatic
side flipping. Normalization is a containment fallback, not the primary
placement: a visible direct anchor remains unchanged so the overlay stays
attached to its composer.

## Geometry and reflow

`computePopupMenuStyle` shall use the current viewport rectangle for both axes:

1. Derive padded viewport bounds with the existing eight-pixel margin.
2. Clamp popup width and left position to the padded horizontal bounds.
3. For `above`, derive the rendered bottom edge from `position.y - margin` and
   clamp that edge to the padded vertical bounds. Calculate available height
   from the clamped edge to the padded viewport top, not from the raw layout
   anchor. Keep the existing `translateY(-100%)` transform so short result sets
   stay bottom-anchored. When the requested edge is already inside the bounds,
   clamping must be an identity operation so the surface remains directly
   adjacent to the composer.
4. For `below`, retain top-edge placement and calculate height from its
   normalized top edge to the padded viewport bottom. Cover this path so the
   above-placement repair cannot change its transform or ordinary geometry.
5. Cap the outer surface at the existing menu-height limit, subtract the header
   height for the internally scrollable listbox, and never produce a negative
   size.

When the software keyboard leaves a composer anchor below the visual viewport,
step 3 moves the rendered bottom edge to the visible viewport's padded bottom.
The list then grows upward into visible space. Anchors already inside the
viewport retain their current geometry and composer adjacency.

While open, `PopupMenu` continues to rerender on window resize and on visual
viewport resize or scroll. The geometry helper then recomputes from the current
bounds. No polling, global store, or new observer is required.

## Interaction contract

The popup remains a body portal above dialog content. Its title labels one
`listbox`; each selectable row remains an `option` with a minimum 44-pixel touch
height. Pointer-down continues to prevent composer blur, and selection remains
owned by the invoking feature. This contextual, transient action list stays a
popup on mobile; it is not a navigation flow that warrants a drawer.

## Control flow

1. A composer recognizes `@`, `#`, or `/` and supplies a direct point or live
   client rectangle.
2. `PopupMenu` resolves the placement point and current viewport.
3. `computePopupMenuStyle` returns the contained fixed-position rectangle.
4. A viewport event increments the local revision and repeats steps 2 and 3.
5. The consumer handles touch or keyboard selection and updates the draft.

## Failure and recovery

- Without Visual Viewport API support, the menu uses the layout viewport and
  preserves current desktop behavior.
- If the visible viewport is too small for the header and one row, geometry
  remains non-negative and contained; the browser may expose less usable
  content until the viewport expands.
- If the composer anchor itself is occluded, containment takes precedence over
  adjacency because no placement can be both attached to that hidden anchor and
  visible. Once layout reflow brings the anchor into view, direct adjacency is
  restored.
- A missing anchor keeps the menu closed. A failed result lookup remains the
  invoking feature's empty or error state and does not affect geometry.
- Closing and retriggering remains available, but viewport reflow must not
  require it.

## Persistence

None. Viewport bounds, menu state, selected index, and result scrolling are
transient browser state.

## Security

The change introduces no new trust boundary. Consumer-provided labels and
descriptions continue through React rendering, and the popup does not evaluate
result data or URLs.

## Observability

No production telemetry is added. Deterministic geometry unit tests cover raw
layout anchors outside a shrunken visual viewport. A phone-browser E2E covers a
saved-prompt result after the page and composer reflow to a keyboard-sized
viewport, including direct composer adjacency, containment, touch selection,
and focus retention. Existing mobile `#` and `/` scenarios provide
sibling-consumer regressions. Browser evidence must resize the page layout with
the visible viewport; replacing only `visualViewport` describes an occluded
composer and is not valid visual evidence for adjacency.

## Related decisions

None. This is a local correction to an existing shared presentation contract
and does not establish a new architecture boundary.
