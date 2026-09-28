---
id: "03-activity-log-backend"
title: "Activity log backend"
status: pending
wave: 2
depends_on:
  - "01-shared-interface"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-ACTIVITY-LOG-001
  - REQ-COORDINATOR-ACTIVITY-LOG-003
  - REQ-COORDINATOR-ACTIVITY-LOG-004
  - REQ-COORDINATOR-ACTIVITY-LOG-005
acceptance_criteria:
  - AC-COORDINATOR-ACTIVITY-LOG-001.1
  - AC-COORDINATOR-ACTIVITY-LOG-001.2
  - AC-COORDINATOR-ACTIVITY-LOG-001.3
  - AC-COORDINATOR-ACTIVITY-LOG-001.4
  - AC-COORDINATOR-ACTIVITY-LOG-001.5
  - AC-COORDINATOR-ACTIVITY-LOG-001.6
  - AC-COORDINATOR-ACTIVITY-LOG-003.2
  - AC-COORDINATOR-ACTIVITY-LOG-003.3
  - AC-COORDINATOR-ACTIVITY-LOG-003.4
  - AC-COORDINATOR-ACTIVITY-LOG-003.5
  - AC-COORDINATOR-ACTIVITY-LOG-004.1
  - AC-COORDINATOR-ACTIVITY-LOG-004.2
  - AC-COORDINATOR-ACTIVITY-LOG-004.3
  - AC-COORDINATOR-ACTIVITY-LOG-005.1
  - AC-COORDINATOR-ACTIVITY-LOG-005.2
  - AC-COORDINATOR-ACTIVITY-LOG-005.3
system_design:
  - ../../specs/coordinator/system-design/activity-log.md
---

# Task 03: Activity Log Backend (WP-6)

## Summary

Write a log row in the same transaction as every proposal change, coalesce
refusals, and serve the list, summary and undo routes, the coordinator's
read tool and daily retention. This is the phase-3 evidence source.

## In scope

- `internal/coordinator/activity.go`: `Record(tx, row)` and
  `RecordRefusal(...)` with 60-second coalescing under the per-coordinator
  lock ([design](../../specs/coordinator/system-design/activity-log.md#refusals)).
- Hooks in the phase-1 create path: propose insert, completion to
  `approved` (with `edited`), reject, failure to `failed`. Task 04 adds the
  same calls for the other kinds through the shared writer.
- Routes `GET activity`, `GET activity/summary`, `POST activity/:rid/undo`
  with cursor paging, class filter, scopes and 404 across workspaces.
- Undo for `create_task` (archive) and `move` (move back) with the
  per-row mutex, `already_undone`, `not_undoable`, `undo_conflict`. Move undo
  reads `outcome_json` written by task 04; its tests seed that column.
- `list_coordinator_activity_kandev` and the `coordinator.list_activity`
  MCP action, principal-scoped, without user identities.
- `Service.ActivitySummary(ctx, coordinatorID, days)` and the daily retention
  ticker plus the startup-pass run.

## Out of scope

- The What it did screen (task 08).
- The guard calling `RecordRefusal` (task 02 wires it; this task provides
  and tests the function).
- Registering the read tool in the profile (task 02's `ToolNames`).

## Acceptance

- Each proposal change leaves exactly one row in the same transaction; a
  failed row insert rolls the change back.
- Concurrent refusals within 60 seconds leave one row with the summed count.
- Undo reverses a create or a move once; conflicts return their codes.

## Verification

```bash
make -C apps/backend test PKG=./internal/coordinator/...
make -C apps/backend test PKG=./internal/mcp/...
```

Tests: a fault-injected row insert leaves the proposal unchanged
(`001.6`); a losing claimer writes no row; 10 concurrent refusals give one
row with count 10 (`001.3`); a direct Resume writes nothing (`001.5`, a
service-level test that calls the orchestrator stub and asserts zero rows);
two concurrent undos of one create archive once and return one 409
(`003.4`); a moved-again task returns `undo_conflict` (`003.3`); a failed
marker transaction leaves the row undoable and a retry only marks it
(reversal already done); a `noop` move row is `not_undoable` and lists
`undoable` false; the summary's `approved` includes edited approvals and
`undone` counts under the reversed row's class (`005.2`); a reader's
undo is 403 (`003.5`); the read tool never returns another coordinator's
rows or any user id (`004.1`, `004.2`); limit 0 and 51 are refused
(`004.3`); retention deletes a 401-day-old row and keeps a 399-day-old one
(`005.1`); with `phase2` off the startup pass and ticker do not run and a
401-day-old row survives (`005.1`); a move undo retried after a failed
marker finds the task on `from_step_id`, skips `MoveTask` and marks the row
(`003.3`); summary counts and `days=0`/`91` refusals (`005.2`); another
workspace's coordinator is 404 (`005.3`).

## Likely files

- `apps/backend/internal/coordinator/activity.go`, `activity_routes.go`,
  `proposals.go`, `retention.go`
- `apps/backend/internal/mcp/server/coordinator_tools.go`,
  `internal/mcp/handlers/` (the list action)

## Dependencies

- Task 01 (table, types, client shapes).

## Risks

- Holding a database lock across a task-service write deadlocks SQLite;
  the undo mutex is in-process by design.
