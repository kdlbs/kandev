# ADR-2026-08-08-workspace-scoped-task-create-workflow-memory: Remember Task-Create Workflows Per Workspace

**Status:** accepted
**Date:** 2026-08-08
**Area:** backend, frontend

## Context

The standard Create Task dialog receives a workflow from the current board or
list filter when one is active, but receives no workflow from All Workflows and
other unscoped entry points. Existing backend-owned task-create preferences
remember repository, branch, agent profile, and executor profile, but not the
workflow that successfully created the previous task. A single global workflow
ID would also lose the remembered choice whenever the user creates a task in a
different workspace.

## Decision

Extend `users.settings.task_create_last_used` with
`workflow_ids_by_workspace`, a map from workspace ID to the workflow ID used by
the most recent successful task creation in that workspace. HTTP and WebSocket
task creation update only the entry for the created task's workspace, preserving
entries for all other workspaces and the other task-create preferences.

The backend remains the durable source of truth under ADR 0028 and ADR 0041.
The frontend may maintain the existing in-memory queued overlay until the
backend settings update arrives, but it does not persist workflow memory in
browser storage.

For the standard Create Task dialog, workflow resolution uses this precedence:

1. A workflow explicitly locked by a feature-specific flow.
2. A workflow selected manually in the currently open standard dialog.
3. The current workspace's valid, visible last-used workflow.
4. A valid, visible workflow supplied by the current board or list context.
5. The sole visible workflow in the workspace.
6. No selection when multiple visible workflows remain.

A board or list filter does not override a valid last-used workflow. Missing,
deleted, hidden, or cross-workspace remembered IDs are ignored. A successful
task creation records the effective workflow; merely opening the selector,
changing it, or cancelling the dialog does not update the durable preference.

## Consequences

- Returning to a workspace restores that workspace's own task-create workflow,
  even after tasks were created in other workspaces or another browser.
- A filtered board can display one workflow while Create Task defaults to the
  workflow most recently used for task creation in that workspace.
- The JSON preference update needs a targeted nested-map merge for both SQLite
  and PostgreSQL so concurrent or alternating workspace writes do not clobber
  unrelated entries.
- Existing settings rows need no SQL schema migration; an absent map behaves as
  an empty history.
- The frontend must validate remembered IDs against the current workspace's
  visible workflows before using them.

## Alternatives Considered

1. **Store one global last-used workflow ID.** Rejected because creating a task
   in workspace B would erase workspace A's remembered choice.
2. **Use `workflow_filter_id` as the task-create default.** Rejected because the
   board/list filter expresses what the user is viewing, and the requested task
   creation default must remain independent of that filter.
3. **Persist selection immediately when the picker changes.** Rejected because
   cancelled dialogs would become durable history even though no task used the
   workflow.
4. **Store the value in browser storage.** Rejected by ADR 0041 because portable
   preferences have one backend-owned source of truth.
