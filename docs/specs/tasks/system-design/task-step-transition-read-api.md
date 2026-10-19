---
status: current
system: tasks
requirements:
  - REQ-TASKS-STEP-TRANSITION-READ-001
---

# Task Step Transition Read API System Design

## Purpose and boundaries

The task system already owns `task_step_transitions` and its writers. This
design adds one read path from the task HTTP handlers through the task service
to the task repository. No table, column, index, migration, writer, WebSocket
action, or MCP tool changes.

## Requirement mapping

| Acceptance criteria                   | Design sections           |
| ------------------------------------- | ------------------------- |
| `AC-TASKS-STEP-TRANSITION-READ-001.1` | Repository read; Response |
| `AC-TASKS-STEP-TRANSITION-READ-001.2` | Authorization             |
| `AC-TASKS-STEP-TRANSITION-READ-001.3` | Repository read           |
| `AC-TASKS-STEP-TRANSITION-READ-001.4` | Compatibility             |
| `AC-TASKS-STEP-TRANSITION-READ-001.5` | Paging                    |

## Repository read

`Repository.ListTaskStepTransitionRecords(task_id, after_id, limit)` selects
`WHERE task_id = ? AND id > ? ORDER BY id ASC LIMIT ?`, served by the existing
`(task_id, id)` index. It left-joins `workflow_steps` twice to resolve the
source and destination step names. The ledger has no foreign key to steps, so a
removed step yields its recorded ID and a null name. The query is the same on
SQLite and Postgres. A task with no rows returns an empty slice.

## Authorization

`Service.ListTaskStepTransitionRecords` calls `authorizeTaskID`, the
`workspace.read` check every task read uses: an unreachable workspace and a
missing task both map to `ErrTaskNotFound`, and the handler's `handleNotFound`
returns the fixed body `{"error":"task not found"}`. For an unscoped caller
(no identity or the synthetic auth-disabled identity) `authorizeTaskID` does not
load the task, so the service reads the task row once to return not-found for
an unknown ID.

The plugin and canvas trail keeps omitting actor fields because a plugin grant
is narrower than a workspace member's access. This endpoint is a separate
model, `StepTransitionRecord`, used only behind `workspace.read`.

## Paging

`limit` defaults to 50 and is capped at 200, the bounds of the existing
transition reads. The cursor reuses the opaque transition cursor encoding with
scope `task-records:<task_id>` and the last returned row ID, so a cursor from
another task, or from the plugin trail, is rejected with
`ErrInvalidTransitionCursor` (400). A move committed during pagination appears
on a later page because the order is ascending by ID.

## Response

```json
{
  "items": [
    {
      "id": "42",
      "from_workflow_id": "wf-1",
      "from_workflow_step_id": "step-todo",
      "from_workflow_step_name": "Todo",
      "to_workflow_id": "wf-1",
      "to_workflow_step_id": "step-approval",
      "to_workflow_step_name": "Approval",
      "trigger": "manual_move",
      "actor_kind": "human",
      "actor_id": "user-1",
      "occurred_at": "2026-10-19T10:00:00Z"
    }
  ],
  "next_cursor": "",
  "has_more": false
}
```

`id` is a decimal string, as in the plugin trail. Nullable endpoint fields
follow the ledger: the genesis row has no source, and detachment has no
destination. `trigger` and `actor_kind` carry the ledger's enum values
unchanged; `actor_id` is the user, session, or watch identifier the ledger
defines, or null.

## Compatibility

The change is additive. `GET /api/v1/sessions/:id/workflow/history`, the
plugin Host and canvas transition reads, and their models are unchanged.
