---
status: active
system: tasks
created: 2026-08-04
owners:
  - kandev
---
# Subtask re-parenting by drag and drop Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001: Subtask re-parenting by drag and drop

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-SUBTASK-REPARENTING-DRAG-DROP-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

Users can detach a subtask or nest a task under another via context-menu actions, but moving a task from under one parent to under another takes two menu hops (un-nest, then nest under), and the sidebar's drag-and-drop only reorders siblings. A direct drag gesture should re-parent a task in one motion with the exact same result as those two menu actions.

## What

- The sidebar task tree (desktop sidebar and the mobile task switcher sheet) lets a user re-parent a task by dragging its row onto another row's **nest drop zone**.
- The result is strictly equivalent to choosing `Un-nest (remove parent)` then `Nest under <target>` from the task's context menu: the task's parent becomes the target, and a task whose workspace mode is `inherit_parent` ends with mode `shared_group` — its materialized workspace and workspace-group membership are unchanged.
- Drop targets are exactly the candidates the context menu offers (the `computeNestCandidates` rules): same-workflow root tasks in the visible group, excluding the dragged task, its current parent, and any subtask (which also excludes the task's own descendants). A task that already has children offers no nest targets.
- While a drag with valid targets is active, candidate rows show a nest drop zone (a left-edge strip) with a `Nest under <title>` affordance. Dropping on a zone re-parents; dropping between rows keeps the existing sibling-reorder behavior; any other drop is a no-op.
- Re-parenting is a single API call on the existing canonical path (`PATCH /api/v1/tasks/:id` with `parent_id`), which already rejects self-parenting, missing/archived/cross-workspace targets, descendant cycles, and one-level-depth violations for kanban tasks.
- The sidebar `Nest under` menu, the Office parent picker, the WS task-update path, and the Office dashboard PATCH all share the same composite semantics: any effective parent change normalizes an `inherit_parent` workspace mode to `shared_group`.
- No confirmation dialog is shown on drop (unlike the detach menu action): the gesture is explicit, the operation is non-destructive (workspace membership is retained) and reversible via the menu or another drag.
- Successful re-parenting is reflected across sidebar, board, and task-detail views without a reload, via the existing optimistic snapshot update and the `task.updated` WebSocket event.
- The mobile task switcher sheet offers the same touch drag-and-drop re-parenting.
- A drag starts only from a row's body. Interacting with a row's context menu (including the Color submenu and every item inside it) never starts a drag and never activates the row: pointer-start and click events inside the menu are contained within the menu and do not reach the row's drag sensor listeners or its click handler.
- On touch, the long-press that opens the context menu cancels any touch-drag the hold started: the menu opens without the row moving, no nest drop zones remain while the menu is open, and continuing to drag inside the open menu does not move or reorder the row. (The drag sensor arms after a 250ms hold, before the ≈700ms long-press opens the menu, so the menu cancels the in-flight drag instead of dropping it.)

## Data model

No table or column is added. Re-parenting updates existing persisted fields in the same task-row write:

- `tasks.parent_id` becomes the target task's id (the previous parent, if any, is replaced).
- `tasks.metadata.workspace.mode` changes from `inherit_parent` to `shared_group` when the parent relationship effectively changes and the mode was `inherit_parent`. Other modes (`shared_group`, `new_workspace`) are unchanged.
- `task_workspace_group_members` is unchanged; active membership remains the durable source of shared workspace access.
- Descendant `parent_id` values, blockers, sessions, repositories, workflow, workflow step, and state are unchanged.

## API surface

No new endpoint. Reuses and extends existing contracts:

- `PATCH /api/v1/tasks/:id` with `parent_id` (non-empty nests; `""` un-nests) — already validated by `Service.resolveParentID` (self, existence, archived, same workspace, descendant cycle) and `validateReparentDepth` (one-level kanban limit, Office trees exempt). **Behavior addition:** when the effective parent changes, `inherit_parent` workspace mode is normalized to `shared_group` (mirroring the detach operation). Success returns the updated task DTO; invalid targets map to `400`; missing task to `404`.
- `PATCH /api/v1/office/tasks/:id` with non-empty `parent_id` — same normalization added for parity; empty parent continues to route through the canonical detach operation.
- WS `task.updated` payload unchanged: `parent_id` is always present (nil when cleared), and `metadata` carries the normalized workspace mode.

## Failure modes

- **Invalid target** (self, descendant, subtask, archived, missing, cross-workspace, or depth violation): the UI filters these targets out before a drop can land, so a drop outside every nest zone is a no-op. If a request is nevertheless rejected by the backend (e.g. a valid-zone target became invalid between render and drop), the UI keeps the task in its original tree position, rolls back the optimistic update, and shows a request-error toast.
- **Persistence failure**: no successful response is returned; the UI rolls back to the original tree.
- **Concurrent submissions** are safe: setting the same parent twice is idempotent, and the optimistic update is reconciled by the authoritative `task.updated` event.
- **No valid targets**: the drag offers no nest zones; only reorder remains possible.

## Scenarios

- **GIVEN** a subtask `C` under parent `A` and a root task `B` in the same workflow, **WHEN** the user drags `C` onto `B`'s nest drop zone, **THEN** `C`'s parent becomes `B`, the sidebar shows `C` nested under `B`, and no reload occurs.
- **GIVEN** an `inherit_parent` subtask `C` under `A`, **WHEN** `C` is dragged onto root `B`'s nest zone, **THEN** `C`'s persisted workspace mode is `shared_group` and its workspace-group membership is unchanged.
- **GIVEN** a root task with no children, **WHEN** it is dragged onto another root's nest zone, **THEN** it becomes that root's subtask.
- **GIVEN** a task that already has children, **WHEN** it is dragged, **THEN** no row offers a nest drop zone (reorder only).
- **GIVEN** a drag over a subtask row or over a task in a different workflow, **WHEN** the pointer rests on the row, **THEN** no nest drop zone is offered.
- **GIVEN** a subtask dragged toward its current parent, **WHEN** the pointer rests on that parent row, **THEN** no nest drop zone is offered.
- **GIVEN** a drag dropped between two sibling rows, **WHEN** the drop lands outside every nest zone, **THEN** the siblings reorder as before.
- **GIVEN** a drop that lands outside every nest zone, **WHEN** the drop completes, **THEN** the task keeps its original parent and no request is sent (a plain no-op); a request-error toast appears only when a valid-zone drop's request is rejected by the backend.
- **GIVEN** a re-parented task, **WHEN** its `task.updated` event arrives over WebSocket, **THEN** cached parent relationships in sidebar, board, and task-detail views are updated.
- **GIVEN** the mobile task switcher sheet, **WHEN** a user touch-drags a subtask onto a root's nest zone, **THEN** the same re-parenting occurs.
- **GIVEN** a task row whose context menu is open, **WHEN** the user presses and moves inside the menu (for example on the Color submenu trigger or one of its swatches), **THEN** no drag starts: no nest drop zones appear, the row neither dims nor moves, and the menu item still works.
- **GIVEN** a task row whose context menu is open, **WHEN** the user clicks a menu item, **THEN** the row is not activated or selected as a side effect of the click.
- **GIVEN** a touch long-press on a task row, **WHEN** the context menu opens, **THEN** the touch-drag started by the hold is cancelled: no nest drop zones remain and the row is not dimmed while the menu is open, and dragging inside the open menu does not move or reorder the row.

## Out of scope

- Nesting via drag on the kanban board (cards keep their step-move drag semantics).
- Lifting the one-level kanban subtask limit (still enforced by `computeNestCandidates` and `validateReparentDepth`; Office trees keep arbitrary depth).
- Drag-to-re-parent in the Office task list (Office keeps its parent picker).
- Bulk or multi-select drag re-parenting.
- A keyboard/AT drag gesture; the existing context menus remain the accessible path to the same operation.

## Implementation plan

See [the implementation plan](../../../plans/subtask-reparenting-drag-drop/plan.md).