---
id: "01-backend-startup-marker"
title: "Backend startup interruption marker"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/interrupted-task-indicator.md"
---

# Task 01: Backend startup interruption marker

## Acceptance

- `models.MetaKeyInterruptedAt = "interrupted_at"` exists in the task metadata
  key block.
- `reconcileOneSessionOnStartup` writes `interrupted_at` (RFC 3339 UTC now)
  onto the owning task when `previousState` is `STARTING` or `RUNNING` and the
  task exists and is not archived.
- Sessions that were `WAITING_FOR_INPUT`, `IDLE`, `CREATED`, `COMPLETED`,
  `CANCELLED`, or `FAILED` at startup do not receive the marker.
- A failed marker write logs a warning and does not abort reconciliation.
- The `sessionExecutorStore` interface exposes `SetTaskMetadataKey` and
  `RemoveTaskMetadataKey` (both already implemented by the concrete
  repository).

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/... -run 'ReconcileSessionsOnStartup'
cd apps/backend && go build ./...
```

## Files likely touched

- `apps/backend/internal/task/models/models.go` — `MetaKeyInterruptedAt`.
- `apps/backend/internal/orchestrator/service.go` — `sessionExecutorStore`
  interface + `reconcileOneSessionOnStartup` active-branch write.
- `apps/backend/internal/orchestrator/task_operations_test.go`,
  `apps/backend/internal/orchestrator/reconcile_restart_test.go` — marker
  assertions for RUNNING/STARTING vs WAITING_FOR_INPUT vs archived.

## Dependencies

None.

## Inputs

- Spec: `What`, `Data model`, `State machine` (unmarked → marked), `Failure
  modes`.
- Plan: `Backend > Marker key and detection`.
- Existing pattern: the task `IN_PROGRESS → REVIEW` guard write in the same
  function (`taskArchived` + `UpdateTaskStateIfCurrentIn`).

## Risks

- Marking a task whose taskID is empty, or an archive committing between a
  guard read and the metadata write — the marker write must be archive-atomic
  (`SetTaskMetadataKeyIfNotArchived`, `archived_at IS NULL` in the same
  statement), so a concurrent archive can never leave a stale marker.
- Test wrappers embedding `sessionExecutorStore` compile unchanged because they
  embed the interface.

## Output contract

Report the marker key, where the write sits in the reconciliation flow, the
exact tests added (including the negative cases), commands and results, then
mark this task `done` and update its checkbox in `plan.md`.
