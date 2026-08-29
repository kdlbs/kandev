---
spec: docs/specs/tasks/system-design/runtime-cleanup.md
created: 2026-07-29
status: implemented
---

# Implementation Plan: Task Cleanup Idempotency and Retry Bounds

## Overview

GitHub issue #2027 is reproducible through the SQLite-backed cleanup worker:
missing worktrees and missing task-environment rows are returned as cleanup
errors, so `cascade_delete` jobs enter `retry_wait` forever. The repair first
makes those two deletion boundaries explicitly idempotent, then adds a bounded
backoff state machine so any other permanent error reaches a durable terminal
state.

## Root Cause

- `teardownEnvironmentResources` appends `worktree.ErrWorktreeNotFound` to its
  aggregate error instead of recognizing that the desired deletion state is
  already true.
- SQLite task-environment deletion returns an unclassifiable formatted error
  when the row is absent, and `cleanupTaskEnvironment` retries that error.
- `retryTaskResourceCleanupJob` always writes `retry_wait` with a fixed
  one-minute deadline. `Attempts` is used as a claim generation but never as a
  retry limit.

## Backend

### Missing-resource contracts

- Add `ErrTaskEnvironmentNotFound` to
  `apps/backend/internal/task/repository/repoerrors/errors.go`, re-export it
  from `repository/interface.go` and `repository/sqlite/errors.go`, and wrap
  task-environment not-found results in
  `repository/sqlite/task_environment.go`.
- Update `teardownEnvironmentResources` in
  `service/service_task_environments.go` to treat only
  `worktree.ErrWorktreeNotFound` as a successful worktree teardown.
- Update `cleanupTaskEnvironment` in `service/service_tasks.go` to treat only
  `repository.ErrTaskEnvironmentNotFound` from the row deletion as complete.
  Other joined teardown and persistence errors remain retryable.

### Bounded cleanup retries

- Add terminal `TaskResourceCleanupStateFailed` in
  `task/models/resource_cleanup.go`.
- Replace the fixed reschedule in
  `task/service/resource_cleanup_jobs.go` with the spec's seven-step retry
  schedule and an eight-attempt ceiling. Attempts 1-7 write `retry_wait`;
  attempt 8 writes `failed`, preserves the final error, clears
  `next_attempt_at`, and emits no further automatic work.
- Update `task/repository/sqlite/resource_cleanup.go` so `failed` completions
  receive `completed_at`; due-job queries continue selecting only `pending` and
  `retry_wait`.

## Tests

- **What:** deleting a missing task-environment row returns a classifiable
  sentinel.
  **File:** `apps/backend/internal/task/repository/sqlite/task_environment_test.go`
  **How:** SQLite repository test asserting `errors.Is`.
- **What:** both missing-resource variants complete a `cascade_delete` cleanup
  job successfully.
  **File:** `apps/backend/internal/task/service/resource_cleanup_jobs_test.go`
  **How:** table-driven SQLite-backed service-worker test using the real job
  claim/completion path and a worktree destroyer fake only at the external
  worktree boundary.
- **What:** non-not-found teardown failures remain retryable.
  **File:** `apps/backend/internal/task/service/service_task_environments_test.go`
  **How:** focused service test with a generic worktree error.
- **What:** retry delays follow the documented schedule and the eighth failure
  becomes terminal.
  **File:** `apps/backend/internal/task/service/resource_cleanup_jobs_test.go`
  **How:** table-driven delay unit test plus SQLite-backed claim-completion tests
  for attempts 1, 7, and 8.
- **What:** terminal failed jobs have a completion timestamp and are never due.
  **File:** `apps/backend/internal/task/repository/sqlite/resource_cleanup_test.go`
  **How:** SQLite repository test that completes a claimed job as `failed` and
  queries the due inventory.

## Implementation Waves And Parallel Candidates

Execution is sequential because both tasks touch the cleanup service tests and
the second task builds on the error classification established by the first.

- [x] [Task 01: Make missing cleanup resources idempotent](task-01-idempotent-missing-resources.md)
- [x] [Task 02: Bound cleanup retries](task-02-bounded-cleanup-retries.md)

## Documentation Impact

The durable task-runtime and storage-maintenance specs are amended in this
design package. No public CLI, configuration, API, or UI documentation changes
are required.

## Risks

- Treating only typed sentinels as success is deliberate: matching error strings
  could hide unrelated repository or Git failures.
- Terminal failure stops automatic cleanup, so the final error and completion
  metadata must remain queryable. A user-facing replay action and failure UI are
  explicitly out of scope.
