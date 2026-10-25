---
status: current
system: tasks
requirements:
  - REQ-TASKS-MOBILE-KANBAN-SCROLL-001
---

# Mobile Kanban scrolling design

## Boundary and evidence

This is the phone exception to [Kanban task reordering](kanban-task-reordering.md).
`KanbanCard` previously disabled `useDraggable` only during multi-select.
`KanbanCardShell` previously applied `touch-none md:touch-auto`, blocking native phone
panning. `useSwimlaneKanbanDnd` registers PointerSensor (8px) and TouchSensor
(250ms); removing only the touch sensor would leave pointer activation.

## Components and flow

Use the existing `presentation="mobile"` passed by `MobileKanbanLayout` through
`SwipeableColumns` and `KanbanColumn` to the cards. It comes from the canonical
`useResponsiveBreakpoint` phone boundary. Disable `useDraggable` on that
presentation as well as in multi-select. Suppress drag listeners, drag ARIA
instructions, and keyboard reorder handlers on mobile while preserving semantic
focus and normal task activation. Do not implement a second breakpoint detector.

Remove the phone `touch-none` restriction from `kanban-card-content.tsx`.
The existing carousel `touch-pan-y` permits native vertical panning and keeps
horizontal column navigation with Embla. Keep the shared desktop sensors and
move callbacks: the visible Move to menu currently reaches `moveTaskToStep`
through the shared DnD hook. Removing that hook would accidentally remove moves.

In `swimlane-kanban-content.tsx`, omit mobile keyboard-reorder wiring.
The mobile-only `MobileDropTargets` render and unreferenced module are removed. Preserve shared drag context if needed by the
column's droppable hooks; disabled card activators are the boundary.

## Mobile composition

Entry: Home, Kanban view. Nearest shipped exemplars are `MobileColumnTabs`,
`SwipeableColumns`, and `KanbanCardActions`: keep the focused workflow/step
navigator, single column, card-body navigation, and visible menu. This frequent
listing remains inline; Move to remains a temporary menu choice.
The existing virtualized column owns vertical scrolling. Keep current dynamic
height, safe areas, menu touch targets, and FAB clearance. Shared stores,
ordering, mutations, filtering, selection, empty states, and errors remain in
place. No new copy or controls are needed.

## Requirement mapping and verification

| Criteria | Evidence |
| --- | --- |
| .1, .4 | Card component tests: mobile disables every drag activator; desktop/tablet retain them; resize switches behavior. |
| .1, .2 | Replace mobile-kanban-reorder.spec.ts with real CDP touch scroll tests, including a delayed swipe and unchanged persisted order/step. |
| .3 | Mobile E2E: card tap, visible Move to completion, horizontal navigation. |
| .4 | Existing desktop kanban-reorder.spec.ts plus phone 767px / tablet 768px checks. |

All criteria belong to `REQ-TASKS-MOBILE-KANBAN-SCROLL-001`. Phone E2E must assert actual
scrollTop movement and reaching a lower card, not just a CSS class. Capture a
phone screenshot during the focused run. A long column must include enough
tasks to overflow and exercise virtualization; avoid wheel events as touch proof.

## Data, failure, and compatibility

No API, persistence, permission, telemetry, or migration changes. Menu move
failures retain existing errors and rollback. Test no gesture-triggered reorder
or move request and unchanged stored task positions/steps. Explicit user removal
of mobile drag takes precedence over the previous mobile reorder test; desktop
and tablet retain their shipped contract.
