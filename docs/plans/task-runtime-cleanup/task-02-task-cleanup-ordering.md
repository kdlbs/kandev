---
id: "02-task-cleanup-ordering"
title: "Task cleanup ordering"
status: completed
wave: 1
depends_on: ["01-runtime-inventory"]
plan: "plan.md"
spec: "../../specs/tasks/runtime-cleanup.md"
---

# Task 02: Task Cleanup Ordering

## Acceptance

- Archive/delete cleanup builds runtime stop targets from `executors_running`
  rows owned by the task.
- Executor rows/worktrees are removed only after stop has been attempted for each
  runtime row.
- Failed or uncertain stops preserve enough durable information for retry and
  diagnosis.

## Verification

```bash
cd apps/backend && go test ./internal/task/service
```

## Files likely touched

- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_tasks_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction.go`

## Dependencies

Task 01.

## Inputs

- Runtime inventory methods from Task 01
- Existing `TaskExecutionStopper` contract
- Existing archive/delete cleanup flow in `service_tasks.go`

## Output contract

Report cleanup ordering changes, which stop-failure state is persisted, files
changed, and tests run.

## Result

- Archive/delete/cascade cleanup builds stop targets from `executors_running`
  before falling back to active sessions.
- Runtime inventory failure aborts archive/delete before task mutation and skips
  cascade cleanup.
- Stop failures preserve executor rows and skip destructive environment,
  worktree, and quick-chat cleanup for retry.
- Verified with `go test ./internal/task/service`.
