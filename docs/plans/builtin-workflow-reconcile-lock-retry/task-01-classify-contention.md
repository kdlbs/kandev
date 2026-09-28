---
id: "01-classify-contention"
title: "Classify contention in reconciler attempts"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-BUILTIN-WORKFLOW-RECONCILE-001
acceptance_criteria:
  - AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.1
  - AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.2
  - AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.3
  - AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.4
  - AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.5
  - AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.6
  - AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.7
  - AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.8
  - AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.9
  - AC-TASKS-BUILTIN-WORKFLOW-RECONCILE-001.10
system_design:
  - ../../specs/tasks/system-design/builtin-workflow-step-reconciliation.md
---

# Task 01: Classify Contention in Reconciler Attempts

## Summary

Change `tryHealWorkflowStepEvents` to return an attempt outcome that separates
a CAS miss, a transient contention error, and a terminal error. Update both
built-in workflow step reconcilers so transient contention is retried in a new
transaction and exhaustion warns and continues.

## In scope

- Write the real-driver regressions first and confirm they fail on the current
  code with a "locked" error returned from the retry loop.
- Add the outcome type and classify every database error site in the helper.
- Update `tryHealParticipantSeatRow`, `tryHealAgentErrorRow`, and both retry
  loops, including the CAS failpoints and the `retry_cause` / `error` warning
  fields.
- Add the `workflowStepEventsAfterReadHook` and
  `workflowStepEventsDBFailpoint` test seams next to the existing failpoints in
  `base.go`, and a fixture helper that returns the test repository and its
  database file path.
- Add the per-site transient and non-transient regressions and the
  cancellation-between-attempts regression for both reconcilers.
- Add the `isRetryableStateRaceError` table test.

## Out of scope

- Any other `initSchemaContext` step, the SQLite DSN, the transaction mode, the
  retry budget, metrics, and public documentation.
- The comment fan-out reconciler from open PR #3963.

## Acceptance

- The transient, persistent, continuation, CAS-exhaustion, and non-transient
  scenarios in the system design's real-driver regression table pass for both
  reconcilers against a real file-backed SQLite database.
- The per-site transient and non-transient scenarios pass for all five sites,
  and the cancellation scenario passes for both reconcilers.
- Existing reconciler tests pass without weakening, and healthy-path behavior
  and log output are unchanged apart from the new warning fields on exhaustion.
- `isRetryableStateRaceError` table test passes, including wrapped errors and
  `context.Canceled`.

## Verification

```bash
# From apps/backend:
go test ./internal/task/repository/sqlite/ -run 'Heal|Reconcile|RetryableStateRace' -race -count=1
go test ./internal/task/repository/sqlite/ -race -count=1

# From the repository root:
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/builtin_workflow_reconcile.go`
- `apps/backend/internal/task/repository/sqlite/builtin_workflow_agent_error_reconcile.go`
- `apps/backend/internal/task/repository/sqlite/builtin_workflow_seat_reconcile.go`
- `apps/backend/internal/task/repository/sqlite/base.go`
- `apps/backend/internal/task/repository/sqlite/builtin_workflow_agent_error_reconcile_test.go`
- `apps/backend/internal/task/repository/sqlite/builtin_workflow_seat_reconcile_test.go`
- `apps/backend/internal/task/repository/sqlite/builtin_workflow_reconcile_test.go` (new)
- `docs/plans/builtin-workflow-reconcile-lock-retry/plan.md`
- `docs/plans/builtin-workflow-reconcile-lock-retry/task-01-classify-contention.md`

## Risks

- Go function-length and complexity limits in `apps/backend/.golangci.yml`;
  extract a shared loop helper if a loop grows past them.
