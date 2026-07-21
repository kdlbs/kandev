---
status: shipped
created: 2026-07-18
owner: kandev
---

# Subtask detachment

## Why

Users can create and reorganize task trees, but they cannot promote an existing subtask to a top-level task from the task menus. Clearing only the hierarchy relationship is also unsafe for subtasks that inherit their parent's materialized workspace because future launches would lose the sharing policy.

## What

- A user can detach a subtask from its parent from the sidebar task context menu and the task card's three-dot and context menus.
- The action is shown only for tasks with a parent and requires confirmation.
- Confirmation explains that detachment changes task hierarchy only and that a workspace shared with the parent remains shared.
- Detachment clears the task's parent relationship without changing its workflow, workflow step, state, blockers, descendants, repositories, sessions, or agent execution.
- A detached task becomes a top-level root of its existing subtree. Its descendants remain attached to it.
- A task using `inherit_parent` workspace mode changes to `shared_group` mode when detached. Its workspace-group membership remains active, so current and future sessions continue using the same materialized workspace.
- Tasks already using `shared_group` or `new_workspace` keep that workspace mode.
- Detaching an already-root task is idempotent and succeeds without changing the task.
- Choosing `No parent` in the Office parent picker uses the same detachment behavior.
- Successful detachment is reflected in all connected board, sidebar, and task-detail views without a reload.

## Data model

Detachment updates existing persisted fields; it adds no table or column.

- `tasks.parent_id` becomes the empty string.
- `tasks.metadata.workspace.mode` changes from `inherit_parent` to `shared_group` when applicable.
- `task_workspace_group_members` is unchanged. Active membership remains the durable source of shared workspace access.
- Existing blocker rows, task-session rows, task-environment rows, and descendant `parent_id` values are unchanged.

The hierarchy update and workspace-mode update are persisted in the same task-row update.

## API surface

`POST /api/v1/tasks/:id/detach`

- Request body: none.
- Success: `200` with the updated task DTO. An already-root task returns its unchanged DTO.
- Missing task: `404`.
- Persistence failure: `500`; no successful response is returned.

On success, the backend publishes `task.updated`. Its payload includes `parent_id` even when cleared so clients can remove a previously cached parent relationship. When metadata changes, the payload includes the updated workspace policy.

## State transition

The operation has one transition:

`child(parent_id != "")` -> `root(parent_id = "")`

If workspace mode is `inherit_parent`, the same operation also transitions it to `shared_group`. No task, session, workflow, or executor lifecycle state changes.

## Failure modes

- If the task no longer exists, the UI keeps its current state and shows the request error.
- If persistence fails, neither the UI nor API reports success. The task remains attached according to durable state.
- Repeated submissions are safe because detaching an already-root task is a no-op.
- Detachment does not create, copy, move, clean, or delete workspace files.

## Persistence guarantees

The cleared parent relationship and normalized workspace mode survive backend restarts. Existing workspace-group membership and materialized workspace ownership remain unchanged and follow the existing cleanup lifecycle for the group.

## Scenarios

- **GIVEN** a subtask in the sidebar, **WHEN** the user chooses `Detach from parent` from its right-click menu and confirms, **THEN** the task appears as a top-level task without a page reload.
- **GIVEN** a subtask card on the kanban board, **WHEN** the user detaches it from the three-dot menu, **THEN** the card remains in the same workflow step and no longer belongs to its former parent.
- **GIVEN** a root task, **WHEN** either task menu opens, **THEN** `Detach from parent` is not shown.
- **GIVEN** an `inherit_parent` subtask, **WHEN** the confirmation opens, **THEN** it warns that the shared workspace remains shared.
- **GIVEN** an `inherit_parent` subtask with an active workspace-group membership, **WHEN** it is detached, **THEN** its mode becomes `shared_group`, its membership remains active, and later sessions resolve the same materialized environment.
- **GIVEN** a `new_workspace` or `shared_group` subtask, **WHEN** it is detached, **THEN** its workspace mode and workspace-group membership remain unchanged.
- **GIVEN** a subtask with descendants and blocker relationships, **WHEN** it is detached, **THEN** its descendants remain nested beneath it and all blocker relationships remain unchanged.
- **GIVEN** an Office task with a parent, **WHEN** the user chooses `No parent` in the parent picker, **THEN** the canonical detach operation applies the same workspace behavior.
- **GIVEN** a detached task update received over WebSocket, **WHEN** the client merges it into cached task state, **THEN** the prior parent relationship is cleared rather than preserved.
- **GIVEN** the mobile task switcher, **WHEN** a user opens the touch-accessible actions for a subtask, **THEN** the same detach and confirmation flow is available.

## Out of scope

- Reparenting a task to a different parent from the sidebar or kanban card menus.
- Copying or provisioning a separate workspace during detachment.
- Removing blocker relationships or changing descendant relationships.
- Stopping or restarting active sessions.
- Bulk detachment.
- Synthesizing an `on_children_completed` trigger for the former parent.

## Implementation plan

See [the implementation plan](../../plans/subtask-detachment/plan.md).
