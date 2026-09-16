---
status: current
system: ui
requirements:
  - REQ-UI-ADAPTIVE-KANBAN-001
  - REQ-UI-ADAPTIVE-KANBAN-002
  - REQ-UI-ADAPTIVE-KANBAN-003
created: 2026-09-05
updated: 2026-09-16
owners:
  - kandev
---

# Adaptive Kanban System Design

## Purpose and boundaries

The UI system owns the desktop Kanban grid, its contained horizontal overflow, and its drag-time geometry.

This design covers column sizing, workflow heights, and drag scroll anchoring. It does not change workflow data, move permissions, or task persistence.

The 2026-09-16 sizing and overflow extension is implemented through the
[scrolling plan](../../../plans/kanban-swimlane-scrolling/plan.md). It replaces
the 400px compact cap while retaining the existing width, drag, virtualization,
and responsive contracts.

## Requirement mapping

| Requirement                  | Design sections                                                                                                                                                       |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-ADAPTIVE-KANBAN-001` | [Components and responsibilities](#components-and-responsibilities), [Drag control flow](#drag-control-flow), [Responsive boundaries](#responsive-boundaries)         |
| `REQ-UI-ADAPTIVE-KANBAN-002` | [Workflow height allocation](#workflow-height-allocation), [Height measurement](#height-measurement), [Responsive boundaries](#responsive-boundaries)                 |
| `REQ-UI-ADAPTIVE-KANBAN-003` | [Overflow presentation](#overflow-presentation), [Scroll input and accessibility](#scroll-input-and-accessibility), [Mobile design contract](#mobile-design-contract) |

## Components and responsibilities

- `getKanbanColumnGridTemplate` defines each desktop column as `minmax(280px, 1fr)`.
- `AdaptiveDesktopKanban` owns the horizontal scroll window and the lane grid.
- `AdaptiveDesktopKanban` adds drag-only end space after the grid's overflowing tracks.
- `useKanbanDragScrollAnchor` records the source column position before drag state changes the rendered steps.
- `SwimlaneKanbanContent` derives normal steps, move-target steps, temporary steps, and the active drag state.

The lane grid keeps `min-width: 100%`. Columns share available width until their 280px minimum requires contained horizontal overflow.

## Drag end-space contract

A drag can reveal auto-hidden destinations before or after the source column. The scroll window needs end space to restore the source position.

The drag reserve uses a trailing spacer after a width-constrained lane grid. The spacer extends the scrollable area after overflowing tracks without reducing space for grid tracks.

The lane grid must not use end padding or margin for this reserve. Padding reduces the grid content box, and margin starts at the grid border edge instead of after overflowing tracks.

The reserve exists only while a task drag is active. The normal board has no additional end space.

The desktop scroll window hides its native horizontal scrollbar while drag state is active. The window keeps its internal scroll range so pointer-driven and automatic drag scrolling continue to work. Document width remains unchanged.

## Drag control flow

1. The drag-start handler records the source step and its viewport position.
2. The handler starts the existing drag state.
3. `SwimlaneKanbanContent` adds temporary auto-hidden destinations when required.
4. `AdaptiveDesktopKanban` sizes the lane grid to the viewport or its minimum track width, then adds the end spacer after it.
5. `useKanbanDragScrollAnchor` adjusts `scrollLeft` after the rendered step key changes.
6. A drop or cancellation clears the drag state and removes the end spacer.
7. The anchor hook restores the final scroll position and then clears its saved anchor.

If the source step no longer exists, the hook keeps the current scroll position. The authoritative task update controls the final rendered steps.

## Responsive boundaries

The desktop layout uses this design. The tablet layout keeps its two-column snap-scrolling surface and uses the same workflow height allocation.

The phone layout keeps one focused column and separate touch drop targets. This correction does not change mobile composition or touch behavior.

The existing mobile auto-hide E2E scenario covers the nearest mobile surface. It proves touch destinations and document-width containment.

## Workflow height allocation

`SwimlaneContainer` selects the sizing mode after `useRenderedWorkflowLayout` resolves visible workflows.

- Phone Kanban retains its existing full-height focused workflow.
- A single rendered desktop or tablet workflow retains full-height columns.
- Several rendered workflows use compact heights. Collapsed workflows still count toward this condition.
- Pipeline retains its existing content height.

The sortable wrapper must not assign the parent height to every compact lane. `SwimlaneSection` retains its header outside the collapse guard.
An optional presentation prop on `ViewContentProps` carries the compact mode into `SwimlaneKanbanContent`.
That component owns a definite column-area height based on six initial task rows.
The minimum remains 12.5rem. The old 25rem maximum is removed.
The tallest initial segment determines the shared lane height, including column chrome and row spacing.
A short viewport scrolls the outer workflow container rather than compressing the six-card segment.
The workflow header and native horizontal scrollbar add their own measured height outside this column area.
The tablet layout receives the definite height directly. The desktop inner track receives it through `AdaptiveDesktopKanban`.
The desktop outer scroll window uses intrinsic height so its native horizontal scrollbar stays outside the column area.
`KanbanDragSurface` keeps the existing wrapper, while descendant columns resolve `h-full` against the bounded track.

The outer swimlane container owns vertical travel between workflows. Each column owns vertical travel through its tasks.
The desktop scroll window retains horizontal overflow. The document never becomes a board scroll owner.
On a short viewport, the outer container scrolls instead of compressing lanes below the minimum.

## Height measurement

`VirtualizedColumnTaskList` retains `virtualizer.getTotalSize()` for the full logical scroll range.
Its compact sizing report instead sums the first `min(6, taskCount)` logical task-row heights.
The queued WIP divider belongs to its task row and is included once.
Each row includes its existing gap. Avoid an additional spacing layer.
The report never uses the currently visible six rows after scrolling.

Use the virtualizer's existing keyed measurement data and estimates for those first six rows.
Retain measured prefix sizes when those rows leave the mounted window.
Do not mount all tasks or a hidden measurement copy to calculate this prefix.
The current overscan of five lets the initial bounded window measure the prefix.
Width, card presentation, and ordered prefix identity changes invalidate the bounded first-six prefix cache.
Task metadata changes remeasure mounted prefix rows. Offscreen invalidated rows use valid estimates until they return to the mounted window, then their measured size replaces the estimate.
The invalidation is local to the first six logical rows and does not clear the full virtualizer cache or mount additional tasks.
A small pure prefix-height helper can isolate divider, spacing, and missing-measurement behavior for tests.

The existing optional callback carries the prefix total to `useColumnNaturalHeight`.
That hook adds measured header, spacing, and padding before reporting the desired column height.
The desired height is independent of the constrained viewport box.
`useCompactSwimlaneHeight` computes `max(12.5rem, tallest reported height)` over current display steps.
Empty retained columns use the existing minimum. No-column empty guidance retains intrinsic height.

Callbacks and measurements remain local to the lane. Unchanged values cause no state update.
Removed steps discard their measurements. Observers disconnect on unmount and remain disabled outside compact mode.
All added props participate in existing memo comparisons and retain stable callback identity.

During a task drag, the lane retains its last settled height so temporary destinations do not move the target vertically.
After drop or cancellation, the current projection determines the height again.
New callbacks must participate in `KanbanColumn` memo comparisons and remain stable to preserve render isolation.

No setting, API, dependency, migration, or new user-facing preference is required.
The overflow regions use one translated accessible-label frame added to each supported locale.
This extends the existing layout boundary and requires no new ADR.

## Overflow presentation

Use a Kanban-scoped overflow-state hook for native scroll regions.
It reads `scrollTop`, `scrollLeft`, rendered viewport dimensions, and scroll dimensions with a 1px boundary tolerance.
A passive scroll listener schedules at most one geometry read per animation frame.
Resize observers cover the viewport and content wrapper. Virtualizer-total changes trigger a geometry update explicitly.
Disconnect observers, cancel frames, and clear the idle timer on unmount.
Update React state only when edge flags or the active-scroll flag change.

`VirtualizedColumnTaskList` keeps its native scroll element and virtualizer ref.
Place decorative fades in a stationary wrapper around the viewport, below the fixed column header.
`AdaptiveDesktopKanban` uses equivalent stationary left/right overlays outside its horizontal scroll element.
Tablet uses the same cues around its existing snap-scrolling viewport in `SwimlaneKanbanContent`.
Fades use theme background tokens, `pointer-events: none`, and `aria-hidden`.
Start with a 16px fade depth. Final depth is a visual token, not a card-sizing input.
Keep native gutters and focus rings outside the fade area. Avoid masking the entire column or its header.

Add scoped classes in `app/globals.css`. Do not alter global scrollbar rules or other application panels.
Keep native scrollbar dimensions constant. Use transparent thumb and track colors in the idle fine-pointer state.
Use existing thumb tokens on owner hover, focus-within, or active scroll.
Reserve a stable gutter where supported. Never toggle `display`, scrollbar width, or overflow to reveal a thumb.
Keep scrollbar layout space stable during drag while preserving the existing drag-only horizontal thumb suppression.
Active scrolling ends after 800ms without scroll events. Hover or focus keeps the thumb visible independently.
Honor coarse-pointer, reduced-motion, and forced-color media queries.
Coarse pointers retain visible native styling where the platform permits it. OS overlay-scrollbar policy remains authoritative.
Forced colors disable decorative fades and idle transparency. Reduced motion removes transitions.

## Scroll input and accessibility

The existing outer `SwimlaneContainer` owns vertical movement between workflows.
Column regions retain native vertical scrolling and allow `overscroll-behavior-y: auto`.
Audit intermediate horizontal wrappers so they do not trap vertical scroll chaining.
Keep the application root's document-overflow protections.
Do not add wheel interception, synthetic wheel forwarding, or synchronized column positions.

Overflowing regions receive a keyboard focus stop and an accessible name derived from the workflow or step.
Use existing translated names or translated label frames. New frames require all supported locale catalogs.
Native keyboard scrolling must coexist with card reorder key handlers.
Fades do not alter task selection, drag hit-testing, or auto-scroll ownership.
The horizontal edge calculation excludes the temporary drag reserve from meaningful content cues.
Existing drag-anchor and trailing-spacer rules remain authoritative.

## Mobile design contract

Home remains the entry point. `MobileColumnTabs` is the shipped navigator exemplar.
The hierarchy remains workflow, step, then tasks. The temporary drawer selects context, while the focused column owns task scrolling.
Task taps navigate directly. Existing dynamic viewport sizing, safe-area clearance, and touch controls remain in place.
The compact mode never applies below the canonical 768px phone boundary.
Responsive changes do not overwrite saved desktop preferences.

## Test strategy

- A desktop geometry regression seeds two sparse workflows and asserts that both headers and first cards fit at 1440 by 900.
- Mixed sparse and dense lanes prove independent heights, bounded mounts, and access to the last task.
- Collapse, filters, empty retained lanes, and preview resizing prove mode changes and measurement cleanup.
- Tablet coverage uses `tabletTestPage`. Phone coverage uses the configured Pixel 5 and widths adjacent to 768px.
- The implementation plan names the exact suites and acceptance mappings.

- Component tests assert that drag state uses a trailing spacer and does not use end padding.
- The Chromium Kanban E2E scenario compares every rendered desktop column before and during drag.
- The same Chromium scenario covers a board whose minimum track width exceeds the viewport and verifies the added scroll range.
- The Chromium scenario verifies document containment and the absence of a transient drag-only scrollbar.
- The same E2E scenario proves temporary destinations, cancellation, and a successful drop.
- The existing `mobile-chrome` scenario continues to prove mobile drag destinations and document-width containment.

## Implementation plans

- [Six-card swimlane scrolling](../../../plans/kanban-swimlane-scrolling/plan.md)
- [Previous compact-height delivery](../../../plans/kanban-swimlane-height/plan.md)

## Related decisions

No architecture decision record applies. This design keeps the existing grid and drag-anchor boundaries.
