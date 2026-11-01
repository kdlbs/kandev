---
id: "03-unread-list-repair"
title: "Repair unread state on task list load"
status: done
wave: 2
depends_on: ["01-unread-read-model"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-UNREAD-DOT-001
acceptance_criteria:
  - AC-TASKS-TASK-UNREAD-DOT-001.8
system_design:
  - ../../specs/tasks/system-design/task-unread-dot.md
---

# Task 03: Repair unread state on task list load

## Summary

A task list load fills `unread_messages` for tasks whose summary row is missing
and corrects it for existing rows, so tasks unread before an upgrade, and tasks
whose event was missed, show the right dot after a reload.

## In scope

- Unread state read through the narrow `SessionUnreadStateRepository` interface
  (task 01), type-asserted on the service's session repository like
  `completionGateSummary`; no setter and no `backendapp` wiring.
- `rebuildMissingSummaries` fills the aggregate.
- `reconcileExistingSummary` and `summaryNeedsReconcile` compare the batch result
  with the row's aggregate and repair on mismatch through the existing CAS path.
  A CAS-rejected retry re-reads unread for that task.

## Out of scope

- The live projector path (task 02) and any frontend change.

## Acceptance

- A missing summary row with a cursor and qualifying later message is rebuilt
  with `unread_messages: true`.
- An existing row with `unread_messages: false` is repaired to `true` when
  persisted cursors and qualifying messages derive unread.
- An existing row with `unread_messages: true` is repaired to `false` after
  persisted cursors catch up to all qualifying messages.
- A competing projector advances the cursor and commits `false` after the
  reconciliation read but before its CAS. The rejected write retries from the
  winner and authoritative unread state; the final row remains `false` without
  requiring a later event.
- A list load over a task with no cursor-bearing sessions makes no unread read.

## Verification

```bash
(cd apps/backend && go test ./internal/task/service/... ./internal/task/statussummary/... ./internal/backendapp/...)
(cd apps/backend && make lint)
```

## Files likely touched

- `apps/backend/internal/task/service/service_status_summary_rebuild.go` and `service_status_summary_unread_test.go`

## Dependencies

`01-unread-read-model` provides `ListSessionUnreadState`.

## Risks

- One extra read per list load; it only runs for cursor-bearing sessions of the
  listed tasks.
- `maxSummaryReconcileAttempts` exhaustion must keep the existing
  fall-back-to-rebuild behavior.

## Parallelism

`parallel-safe` with `04-task-row-unread-dot` (disjoint backend and frontend files).

## Inputs

- System design: Restore and repair.
- `ReconcileTaskStatusSummaries`, `reconcileExistingSummary`, `reloadSummaryReconcileState`.

## Results

Done. `service_status_summary_unread.go` reads unread state once per list load
for cursor-bearing sessions through the type-asserted
`repository.SessionUnreadStateRepository`; `rebuildMissingSummaries` attaches
ranks to the rebuild input, and `reconcileExistingSummary` compares and repairs
the aggregate, re-reading the task's unread value after a rejected CAS. Verified
by `service_status_summary_unread_test.go` (missing row rebuilt unread, stale
`false` and `true` rows repaired, racing mark-read kept `false`, no read without
cursors).
