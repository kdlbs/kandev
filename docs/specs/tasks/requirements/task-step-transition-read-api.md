---
status: active
system: tasks
created: 2026-10-19
owners:
  - kandev
---

# Task Step Transition Read API Requirements

## Overview

An external tool that guards a workflow step, for example one that lets a task
leave an approval step only by a person's decision, needs to know who moved a
task and how. The [task-step transition ledger](../system-design/workflow-task-step-transition-ledger.md)
already records every committed step change with its trigger, actor kind, and
actor identifier, but the only HTTP history today is
`GET /api/v1/sessions/:id/workflow/history`, which is per session and carries
no actor kind. The [plugin and canvas trail](../../plugins/requirements/workflow-transition-history.md)
deliberately omits actor fields. Such tools therefore read the database file
directly and break on schema changes.

This capability exposes the existing ledger rows of one task, with their
attribution, to a caller who can already read the task's workspace. It adds no
new semantics and no writes.

## Terminology

- **Recorded move:** A committed row in `task_step_transitions` for the task.
- **Attribution:** The row's `trigger`, `actor_kind`, and `actor_id` as the
  ledger defines them.

## Requirements

### REQ-TASKS-STEP-TRANSITION-READ-001: Attributed task step transition read

**Intent:** A workspace reader can inspect who moved a task between workflow
steps, and how, through the HTTP API instead of the database.

#### Acceptance criteria

- **AC-TASKS-STEP-TRANSITION-READ-001.1:** `GET /api/v1/tasks/:id/step-transitions`
  shall return the task's recorded moves oldest first, ordered by the ledger's
  immutable row identity. Each item shall carry the row ID, source and
  destination workflow and step IDs, the current names of those steps, trigger,
  actor kind, actor ID, and occurrence time, with bounded cursor pagination.
  A step that no longer exists shall keep its recorded ID with a null name.
- **AC-TASKS-STEP-TRANSITION-READ-001.2:** The read shall require
  `workspace.read` on the task's workspace, like other task reads. An unknown
  task, and a task in a workspace the caller cannot reach, shall return the
  same 404 response and no history. With authentication disabled, an unknown
  task shall still return 404.
- **AC-TASKS-STEP-TRANSITION-READ-001.3:** A task with no recorded moves shall
  return an empty list. Moves shall not be reconstructed from messages,
  session history, or the current step.
- **AC-TASKS-STEP-TRANSITION-READ-001.4:** Reading shall never change a task,
  step, workflow, or ledger row. `GET /api/v1/sessions/:id/workflow/history`
  and the plugin and canvas trail shall behave as before.
- **AC-TASKS-STEP-TRANSITION-READ-001.5:** A negative or non-numeric `limit`, a
  malformed cursor, or a cursor issued for another task shall return 400.

## Out of scope

- Attribution on behalf of a person or a channel (`on_behalf_of`); the ledger
  has no such fields.
- A step exit policy enforced by the workflow engine.
- WebSocket, MCP, and plugin variants of this read.
- History from before the ledger's activation.

## Implementation plans

- [Task step transition read API](../../../plans/task-step-transition-read-api/plan.md)
