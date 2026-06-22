---
id: "01-runtime-inventory"
title: "Runtime inventory"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/runtime-cleanup.md"
---

# Task 01: Runtime Inventory

## Acceptance

- `executors_running` can be listed by `task_id` without filtering on
  `task_sessions.state`.
- Startup reconciliation continues to use the global `ListExecutorsRunning`
  inventory.
- Repository tests cover active and terminal task-scoped runtime rows.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite
```

## Files likely touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/executor.go`
- `apps/backend/internal/task/repository/sqlite/executor_test.go`

## Dependencies

None.

## Inputs

- Spec: `docs/specs/tasks/runtime-cleanup.md`
- ADR: `docs/decisions/0025-runtime-cleanup-uses-executors-running.md`
- Existing repository patterns in `apps/backend/internal/task/repository/sqlite/executor.go`

## Output contract

Report repository methods added, tests run, and any schema/status-field limitation
that affects retryable cleanup in later tasks.

## Result

- Added `ListExecutorsRunningByTaskID`.
- Shared executor-row scanning so task-scoped and global runtime inventory include
  the same fields.
- Verified with `go test ./internal/task/repository/sqlite`.
