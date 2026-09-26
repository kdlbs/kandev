---
id: "02-bounded-cleanup-retries"
title: "Bound cleanup retries"
status: done
wave: 2
depends_on: ["01-idempotent-missing-resources"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/runtime-cleanup.md"
---

# Task 02: Bound Cleanup Retries

## Acceptance

- Failed claims 1-7 enter `retry_wait` with the documented delay for that
  attempt.
- A failed eighth claim enters terminal `failed`, retains the final diagnostic,
  has no next-attempt deadline, and receives a completion timestamp.
- Terminal failed jobs are excluded from automatic due-job selection across
  worker ticks and backend restarts.

## Verification

```bash
cd apps/backend && go test -tags fts5 -run 'Test(TaskResourceCleanupRetryDelay|RetryTaskResourceCleanupJob|TaskResourceCleanupJobFailed)' ./internal/task/repository/sqlite ./internal/task/service
```

## Files likely touched

- `apps/backend/internal/task/models/resource_cleanup.go`
- `apps/backend/internal/task/repository/sqlite/resource_cleanup.go`
- `apps/backend/internal/task/repository/sqlite/resource_cleanup_test.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs_test.go`

## Dependencies

Task 01.

## Parallelism

`sequential`; it shares cleanup service behavior and tests with Task 01.

## Inputs

- Runtime cleanup spec: retry schedule, terminal state, and persistence
  guarantees.
- Storage-maintenance spec: `task_resource_cleanup_jobs` state machine.
- Existing optimistic claim-generation behavior in
  `CompleteClaimedTaskResourceCleanupJob`.

## Output contract

Report the RED failures, retry-state implementation, files changed, exact
verification result, terminal-state persistence evidence, residual risks, and
update this task plus `plan.md` status.

## Result

- RED coverage reproduced the fixed one-minute retry and missing terminal completion metadata.
- Added the seven-step backoff schedule, eight-attempt ceiling, terminal `failed` state, and completion timestamps for failed jobs.
- Verified the targeted retry tests, full SQLite/service suites, and the same suites under `-race`.
