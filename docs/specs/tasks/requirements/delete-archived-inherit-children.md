---
status: active
system: tasks
created: 2026-10-20
owners:
  - kandev
---

# Non-cascade delete with archived inherited children requirements

## Overview

Deleting a task with `cascade=false` promotes its direct children to root
tasks. Before that, each child whose workspace mode is `inherit_parent` is
switched to `shared_group`, because a root task has no parent to inherit from.
The child list includes archived children, but the guarded write required the
child to be unarchived. For an archived `inherit_parent` child the write matched
no row, the delete was reported as a concurrent change, and the API returned
`500 request failed`. The delete rolled back, so no data was damaged, but the
parent could not be deleted at all.

Tasks owns this contract because it owns task deletion and the task workspace
metadata. The [orphaned workspace requirements](orphaned-workspace-task-indicator.md)
define the normalization write (AC-TASKS-ORPHANED-WORKSPACE-005.6); this
requirement fixes which children it covers and how a real conflict is reported.

## Terminology

- **Inherited child:** A direct child whose `metadata.workspace.mode` is
  `inherit_parent`.
- **Normalization:** The guarded write that changes an inherited child's mode to
  `shared_group` and clears its orphan marker keys before the child is promoted.

## Requirements

### REQ-TASKS-DELETE-ARCHIVED-INHERIT-CHILDREN-001: Normalize archived inherited children and report real conflicts

**Intent:** A non-cascade delete succeeds when an inherited child is archived, and
a real concurrent change is a retryable conflict rather than a server error.

**User story:** As a user, I want to delete a parent task without deleting its
archived subtasks, so that I can clean up a board without losing history.

#### Acceptance criteria

- **AC-TASKS-DELETE-ARCHIVED-INHERIT-CHILDREN-001.1:** When a non-cascade delete
  promotes an archived inherited child, the system shall normalize that child the
  same way as an unarchived one: mode `shared_group`, orphan marker keys removed,
  parent cleared. The child shall stay archived, and the delete shall succeed.
- **AC-TASKS-DELETE-ARCHIVED-INHERIT-CHILDREN-001.2:** When the normalization write
  loses its guard, the system shall abort before any child is promoted or task is
  deleted, and return a typed conflict naming the child and the reason
  (`child_deleted`, `child_reparented`, or `child_workspace_changed`). REST shall
  answer `409` with `error_code` `task_delete_child_workspace_changed`,
  `child_task_id`, and `reason`; WebSocket shall answer `CONFLICT` with the same
  details.
- **AC-TASKS-DELETE-ARCHIVED-INHERIT-CHILDREN-001.3:** The delete error text shall
  state the cause once, and the server log of an unmapped delete failure shall
  include `task_id` and `cascade`.

## Out of scope

- Cascade deletes, which delete archived children instead of promoting them.
- The delete preflight, which already lists archived direct children in its
  preview digest and needs no new field.
- The orphan marker stamping paths, which keep requiring an unarchived task.
