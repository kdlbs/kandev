---
id: "01-idempotent-missing-resources"
title: "Make missing cleanup resources idempotent"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/system-design/runtime-cleanup.md"
---

# Task 01: Make Missing Cleanup Resources Idempotent

## Acceptance

- Missing task-environment rows are classifiable through
  `ErrTaskEnvironmentNotFound` without relying on error text.
- A `cascade_delete` job succeeds when its captured worktree or environment row
  is already absent.
- Generic worktree and repository failures remain retryable.

## Verification

```bash
cd apps/backend && go test -tags fts5 -run 'Test(DeleteTaskEnvironmentMissing|TaskResourceCleanupMissingResources|TeardownEnvironmentResources)' ./internal/task/repository/sqlite ./internal/task/service
```

## Files likely touched

- `apps/backend/internal/task/repository/repoerrors/errors.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/errors.go`
- `apps/backend/internal/task/repository/sqlite/task_environment.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_test.go`
- `apps/backend/internal/task/service/service_task_environments.go`
- `apps/backend/internal/task/service/service_task_environments_test.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs_test.go`

## Dependencies

None.

## Parallelism

`sequential`; Task 02 shares the cleanup service and its regression test file.

## Inputs

- Runtime cleanup spec: idempotency requirements and missing-resource scenarios.
- Confirmed issue #2027 reproductions for missing worktree and missing
  task-environment row.
- Existing `runtimeStopAlreadyComplete` typed-sentinel pattern in
  `service_tasks.go`.

## Output contract

Report the RED failures, sentinel and cleanup behavior implemented, files
changed, exact verification result, residual risks, and update this task plus
`plan.md` status.

## Result

- RED coverage reproduced both issue paths: missing worktree and missing task-environment row.
- Added typed `ErrTaskEnvironmentNotFound` classification and made only typed not-found cleanup outcomes idempotent.
- Verified with `go test -tags fts5 -run 'Test(DeleteTaskEnvironmentMissing|TaskResourceCleanupMissingResources|TeardownEnvironmentResources)' ./internal/task/repository/sqlite ./internal/task/service` (6 tests passed).
