---
status: current
system: ui
updated: 2026-09-11
requirements:
  - REQ-UI-COMPOSER-OVERLAY-001
---

# Composer Suggestion Overlay System Design

## Purpose and boundaries

The shared `PopupMenu` primitive positions composer suggestion surfaces from a
direct client-coordinate point or an editor-provided client rectangle. The
browser may report caret rectangles in a different coordinate space from
fixed-position CSS while its software keyboard pans the visual viewport.
Positioning must normalize that boundary before applying containment.

The primitive owns viewport containment, maximum size, reflow subscriptions,
listbox structure, touch-row size, and focus-preserving pointer behavior. Each
consumer continues to own trigger recognition, result loading, selection, and
draft serialization.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-COMPOSER-OVERLAY-001` | [Geometry and reflow](#geometry-and-reflow), [Interaction contract](#interaction-contract) |

## Components and responsibilities

- `PopupMenu` in `apps/web/components/task/chat/popup-menu.tsx` owns the portal,
  anchor contract, positioning lifetime, and one labelled listbox.
- `positionPopupMenu` in the adjacent `popup-menu-position.ts` integrates
  Floating UI's DOM platform with the product's size and placement constraints.
  The library normalizes client rectangles into fixed-position coordinates,
  including WebKit's visual viewport offsets. Unit tests exercise the real DOM
  platform with controlled browser measurements, not a mocked positioning
  algorithm.
- `MentionMenu`, `SlashCommandMenu`, and `EntityReferenceMenu` use the default
  above-anchor placement for chat and shared prompt-composer suggestions.
- `PlanSlashMenu` uses the same primitive with below-anchor placement. Its
  placement semantics are not changed, but unit coverage must protect that path
  from a shared-geometry regression.

## Data and contracts

`PopupMenu` accepts one of two transient anchor contracts:

- a direct `{x, y}` client-coordinate point; or
- a `clientRect` callback whose top or bottom edge supplies the placement point.

The DOM positioning platform reads the browser's visual viewport, or falls
back to the document viewport when that API is absent. Callers must not add
visual offsets themselves. No value is stored or sent over an API.

The existing placement contract remains explicit: `above` renders the
surface's bottom edge above its normalized anchor; `below` renders the surface's
top edge below its normalized anchor. This repair does not introduce automatic
side flipping. Normalization is a containment fallback, not the primary
placement: a visible direct anchor remains unchanged so the overlay stays
attached to its composer.

## Geometry and reflow

Positioning uses Floating UI with a virtual caret reference, fixed strategy,
explicit `top-start` or `bottom-start` placement, and these ordered operations:

1. Offset the surface eight pixels from the caret.
2. Size it against normalized clipping bounds with eight-pixel viewport insets,
   a 420-pixel width cap and a 280-pixel height cap. Use the available space on
   the requested side before shifting, so long lists shrink without covering a
   visible composer and short lists remain naturally caret-adjacent.
3. For an above menu with less adjacent room than a heading, 44-pixel row and
   list padding, allow the available viewport height instead. This prevents an
   occluded anchor from collapsing an otherwise usable menu to zero height.
4. Shift the measured surface into the padded viewport on both axes. Do not
   flip sides. The below-menu path retains its explicit bottom-caret placement
   and does not adopt the above-menu minimum-room fallback.

The surface is a flex column with a non-shrinking heading and one shrinking,
internally scrollable listbox. Its height is content-driven up to the cap;
there is no assumed fixed heading height or vertical translation transform.

While mounted, `autoUpdate` observes ancestor and visual viewport scroll/resize
and reference/menu size and layout changes. Result or anchor changes also
refresh positioning through the React effect. There is no polling or global
mutation observer. The effect removes all subscriptions on close/replacement.
A revision guard rejects stale asynchronous sizing and position writes. The
menu starts hidden until its first position resolves, so it cannot flash at
the document origin.

## Interaction contract

The popup remains a body portal above dialog content. Its title labels one
`listbox`; each selectable row remains an `option` with a minimum 44-pixel touch
height. Pointer-down continues to prevent composer blur, and selection remains
owned by the invoking feature. This contextual, transient action list stays a
popup on mobile; it is not a navigation flow that warrants a drawer.

## Control flow

1. A composer recognizes `@`, `#`, or `/` and supplies a direct point or live
   client rectangle.
2. `PopupMenu` mounts its body portal and starts the positioning lifetime.
3. The DOM platform normalizes the anchor and computes contained fixed CSS
   coordinates. Only the current positioning revision may apply the result.
4. A viewport, layout, or content change repeats positioning without retriggering
   the suggestion plugin.
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

No production telemetry is added. Unit tests cover the recorded Safari
client-coordinate/visual-offset combination through the real DOM platform,
ordinary Chromium geometry, occluded anchors, below placement, reflow, and
cleanup. Browser E2E covers both layout resizing (adjacency) and offset-only
viewport changes (occluded-anchor containment) for `@` and `#`, including
touch selection and retained focus. Offset-only stimuli do not prove adjacency
or emulate WebKit. Neither Chromium nor desktop WebKit emulation certifies a
real iOS software keyboard; keep physical-device results separately identified.

## Related decisions

None. This is a local correction to an existing shared presentation contract
and does not establish a new architecture boundary.
