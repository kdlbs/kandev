---
status: current
system: ui
requirements:
  - REQ-UI-ADAPTIVE-KANBAN-001
  - REQ-UI-ADAPTIVE-KANBAN-002
created: 2026-09-05
updated: 2026-09-11
owners:
  - kandev
---
# Adaptive Kanban System Design

## Purpose and boundaries

The UI system owns the desktop Kanban grid, its contained horizontal overflow, and its drag-time geometry.

This design covers column sizing, workflow heights, and drag scroll anchoring. It does not change workflow data, move permissions, or task persistence.

Workflow-height allocation, horizontal sizing, and drag behavior are implemented and covered by focused browser tests.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-ADAPTIVE-KANBAN-001` | [Components and responsibilities](#components-and-responsibilities), [Drag control flow](#drag-control-flow), [Responsive boundaries](#responsive-boundaries) |
| `REQ-UI-ADAPTIVE-KANBAN-002` | [Workflow height allocation](#workflow-height-allocation), [Height measurement](#height-measurement), [Responsive boundaries](#responsive-boundaries) |

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
That component owns a definite column-area height between 200px and 400px. Constants use root-font-scaled units equivalent to these values.
The workflow header and native horizontal scrollbar add their own measured height outside this column area.
The tablet layout receives the definite height directly. The desktop inner track receives it through `AdaptiveDesktopKanban`.
The desktop outer scroll window uses intrinsic height so its native horizontal scrollbar stays outside the column area.
`KanbanDragSurface` keeps the existing wrapper, while descendant columns resolve `h-full` against the bounded track.

The outer swimlane container owns vertical travel between workflows. Each column owns vertical travel through its tasks.
The desktop scroll window retains horizontal overflow. The document never becomes a board scroll owner.
On a short viewport, the outer container scrolls instead of compressing lanes below the minimum.

## Height measurement

`VirtualizedColumnTaskList` already obtains its logical content height from `virtualizer.getTotalSize()`.
An optional callback reports this total after layout, including measured rows and the WIP divider.
`KanbanColumn` adds its measured header, spacing, and padding before reporting its natural height to the workflow.
The total must not come from the constrained column box or its viewport height.

The workflow computes `clamp(max(column natural heights), minimum, maximum)` over the current display steps.
The empty set uses the minimum only when an actual column area renders. Existing no-column empty states retain intrinsic height.
Initial unmeasured columns use the minimum. The virtualizer then mounts its first window and supplies estimates plus measured row heights.
Callbacks and measurements remain local to the lane. Unchanged values cause no state update.
`useCompactSwimlaneHeight` owns the lane measurements and drag freeze. `useColumnNaturalHeight` observes column chrome and adds logical row totals.
Removed steps discard their measurements. Width changes remeasure mounted rows and header chrome.
Observers disconnect on unmount and remain disabled outside compact mode.

During a task drag, the lane retains its last settled height so temporary destinations do not move the target vertically.
After drop or cancellation, the current projection determines the height again.
New callbacks must participate in `KanbanColumn` memo comparisons and remain stable to preserve render isolation.

No setting, API, dependency, migration, or new user-facing copy is required.
This extends the existing layout boundary and requires no new ADR.

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

## Related decisions

No architecture decision record applies. This design keeps the existing grid and drag-anchor boundaries.
