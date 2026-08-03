---
status: shipped
created: 2026-08-03
owner: kandev
---

# Kanban card ordering

## Why

Users can drag Kanban cards between workflow steps, but cannot control the order of cards
inside a step. Boards that should reflect priority or work sequencing only show newest-first
order, so manual board hygiene requires workarounds outside the product.

## What

- On the Kanban view (desktop, tablet, and phone), admitted cards in a workflow step SHALL be
  reorderable by drag-and-drop within that step.
- Column display order for Kanban SHALL be `position` ascending, then `createdAt` descending,
  then stable `id` — so boards where every card still has `position = 0` keep today’s
  newest-first look until the user reorders.
- A same-step reorder MUST persist new dense positions (`0..n-1`) for the admitted cards in that
  step via the existing task move contract (`POST /api/v1/tasks/:id/move` / equivalent WS).
- Queued WIP overflow cards (present in the step but not admitted) MUST NOT be reorderable; they
  render after the admitted list and keep their existing promotion semantics.
- Cross-step drag-and-drop MUST continue to work; this slice keeps append-to-end placement when
  the target step differs.
- Pipeline and other non-Kanban views are unchanged.

## Data model

Reuses existing `tasks.position` (integer order within a workflow step). No new columns or
tables. Queued overflow continues to use `queued_for_step_id` / `wip_admitted`.

## API surface

Reuses `MoveTask` with the same `workflow_id`, `workflow_step_id`, and `position`. Same-step
calls only change `position`. No new reorder endpoint in this slice.

## Failure modes

- If any persist call in a densify batch fails, the client restores the pre-drag workflow
  snapshot and surfaces the existing move-error path.
- Concurrent editors of the same column are last-writer-wins for positions.
- Dropping a card on itself or canceling a drag leaves order unchanged.

## Persistence guarantees

- Admitted card order in a step survives reload and restart through `tasks.position`.
- Untouched columns (all `position = 0`) keep newest-first display via the `createdAt` tie-break
  until a reorder densifies them.

## Scenarios

- **GIVEN** two admitted cards A (newer) and B (older) in one step with `position = 0`,
  **WHEN** the Kanban column renders, **THEN** A appears above B (newest-first tie-break).
- **GIVEN** admitted cards A above B in a step, **WHEN** the user drags A below B and drops,
  **THEN** B appears above A immediately and after reload.
- **GIVEN** a step with admitted cards and queued overflow cards, **WHEN** the column renders,
  **THEN** admitted cards appear first (board order) and queued cards appear after them and are
  not sortable.
- **GIVEN** an admitted card, **WHEN** the user drags it to a different workflow step,
  **THEN** it appends to that step as today.
- **GIVEN** a same-step reorder while the step is over its WIP limit, **WHEN** the drop
  completes, **THEN** the reorder still persists (same-step moves remain WIP-exempt).

## Out of scope

- Pipeline / graph2 within-step reorder.
- Reordering queued WIP overflow cards.
- Cross-step insertion index (still append).
- Fractional indexing, a dedicated reorder API, or a data migration backfill.
- Changing WIP promotion rules beyond writing dense admitted positions on reorder.

## Implementation plan

[`docs/plans/kanban-card-ordering/plan.md`](../../plans/kanban-card-ordering/plan.md)
