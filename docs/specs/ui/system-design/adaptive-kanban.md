---
status: current
system: ui
requirements:
  - REQ-UI-ADAPTIVE-KANBAN-001
created: 2026-09-05
owners:
  - kandev
---
# Adaptive Kanban System Design

## Purpose and boundaries

The UI system owns the desktop Kanban grid, its contained horizontal overflow, and its drag-time geometry.

This design covers desktop column sizing and drag scroll anchoring. It does not change workflow data, move permissions, or task persistence.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-ADAPTIVE-KANBAN-001` | [Components and responsibilities](#components-and-responsibilities), [Drag control flow](#drag-control-flow), [Responsive boundaries](#responsive-boundaries) |

## Components and responsibilities

- `getKanbanColumnGridTemplate` defines each desktop column as `minmax(280px, 1fr)`.
- `AdaptiveDesktopKanban` owns the horizontal scroll window and the lane grid.
- `AdaptiveDesktopKanban` adds drag-only end space outside the grid sizing box.
- `useKanbanDragScrollAnchor` records the source column position before drag state changes the rendered steps.
- `SwimlaneKanbanContent` derives normal steps, move-target steps, temporary steps, and the active drag state.

The lane grid keeps `min-width: 100%`. Columns share available width until their 280px minimum requires contained horizontal overflow.

## Drag end-space contract

A drag can reveal auto-hidden destinations before or after the source column. The scroll window needs end space to restore the source position.

The drag reserve uses logical end margin on the lane grid. The margin extends the scrollable area without reducing space for grid tracks.

The lane grid must not use end padding for this reserve. End padding reduces the grid content box and forces `1fr` tracks to 280px.

The reserve exists only while a task drag is active. The normal board has no additional end space.

## Drag control flow

1. The drag-start handler records the source step and its viewport position.
2. The handler starts the existing drag state.
3. `SwimlaneKanbanContent` adds temporary auto-hidden destinations when required.
4. `AdaptiveDesktopKanban` adds the end margin without changing the track sizing area.
5. `useKanbanDragScrollAnchor` adjusts `scrollLeft` after the rendered step key changes.
6. A drop or cancellation clears the drag state and removes the end margin.
7. The anchor hook restores the final scroll position and then clears its saved anchor.

If the source step no longer exists, the hook keeps the current scroll position. The authoritative task update controls the final rendered steps.

## Responsive boundaries

The desktop layout uses this design. The tablet layout keeps its two-column snap-scrolling surface.

The phone layout keeps one focused column and separate touch drop targets. This correction does not change mobile composition or touch behavior.

The existing mobile auto-hide E2E scenario covers the nearest mobile surface. It proves touch destinations and document-width containment.

## Test strategy

- Component tests assert that drag state uses end margin and does not use end padding.
- The Chromium Kanban E2E scenario compares column width before and during drag.
- The same E2E scenario proves temporary destinations, cancellation, and a successful drop.
- The existing `mobile-chrome` scenario continues to prove mobile drag destinations and document-width containment.

## Related decisions

No architecture decision record applies. This design keeps the existing grid and drag-anchor boundaries.
