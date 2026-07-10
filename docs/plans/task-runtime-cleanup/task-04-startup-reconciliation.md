---
id: "04-startup-reconciliation"
title: "Startup reconciliation"
status: completed
wave: 2
depends_on: ["01-runtime-inventory", "02-task-cleanup-ordering"]
plan: "plan.md"
spec: "../../specs/tasks/runtime-cleanup.md"
---

# Task 04: Startup Reconciliation

## Acceptance

- Backend startup scans persisted `executors_running` rows.
- Terminal and missing-session rows are stopped by `agent_execution_id` before
  runtime tracking is removed.
- Stop failures preserve runtime rows.

## Verification

```bash
cd apps/backend && go test ./internal/task/service ./internal/orchestrator/...
```

## Files likely touched

- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/orchestrator/event_handlers.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
- `apps/backend/cmd/kandev/*.go`

## Dependencies

Tasks 01 and 02.

## Inputs

- Stale runtime inventory method from Task 01
- Cleanup service behavior from Task 02
- ADR-0009 fail-closed cleanup semantics

## Output contract

Report where reconciliation is owned, startup order implications, files changed,
tests run, and any rows intentionally left untouched.

## Result

- Orchestrator startup reconciliation owns this pass.
- Terminal completed/cancelled sessions and rows with missing sessions now stop
  through `StopAgentWithReason` before row deletion.
- Rows without a stoppable execution handle are intentionally preserved with a
  warning.
- Verified with `go test -run TestReconcileSessionsOnStartup ./internal/orchestrator`.
