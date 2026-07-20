---
id: "01-durable-task-cleanup"
title: "Durable task cleanup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 01: Durable task cleanup

Replace detached task cleanup with a persisted, restart-safe job and captured resource inventory.

## Acceptance

- Every archive/delete/cascade/workspace-delete/expiration path persists cleanup intent before task
  mutation; deletion cleanup survives task/session/worktree FK cascades.
- Retryable failures resume after worker reconstruction, while archive jobs cancel when the task is
  unarchived.
- Archived-task historical worktree rows and branch metadata remain available after filesystem
  cleanup.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/service
```

## Files likely touched

- `apps/backend/internal/task/models/resource_cleanup.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/resource_cleanup.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/resource_cleanup_worker.go`
- focused `*_test.go` files beside those sources

## Dependencies

None.

## Inputs

- Spec sections: Task cleanup and orphan workspaces; Unarchive compatibility; Persistence guarantees
- `docs/specs/tasks/runtime-cleanup.md`
- ADR 0025 runtime inventory rules
- PR #1687 unarchive CAS and branch recovery behavior

## Output contract

Report schema/model changes, lifecycle paths migrated, retry/cancellation behavior, tests run,
remaining blockers/risks, and update this task plus `plan.md` to done.
