---
id: "02-lifecycle-creation-barrier"
title: "Serialize task cleanup with creation"
status: done
wave: 2
depends_on: ["01-task-worktree-persistence"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/session-delete-resource-cleanup.md"
decision: "../../decisions/2026-08-08-task-owned-worktree-lifetime.md"
---

# Task 02: Serialize Task Cleanup With Session and Worktree Creation

## Acceptance

- Archive/delete preparation reserves a durable lifecycle barrier before
  capturing resource inventory.
- Session creation and canonical environment-repository persistence serialize
  against the owning task and cannot commit after cleanup preparation has begun.
- PostgreSQL row locking and SQLite writer serialization enforce the same
  contract without relying on an in-process mutex.
- The barrier transaction commits before filesystem, target-path, repository, or
  Git locks are acquired.
- A worktree physically created before a concurrent barrier rejection is safely
  compensated without touching pre-existing resources.
- `DeleteSession` does not reserve or activate a cleanup job and cannot invoke a
  physical worktree cleaner, including for the last session.
- A new session created after an old session is deleted reuses the retained task
  environment when no task-lifecycle barrier exists.

## TDD Sequence

1. RED: add repository concurrency tests that pause archive/delete after barrier
   reservation and prove session/environment-repository insertion cannot cross
   the barrier.
2. RED: exercise the inverse ordering and prove a committed session/worktree is
   included in the later task snapshot.
3. RED: add a materialization test where the database barrier rejects the owner
   transaction after Git creation and assert bounded, target-specific rollback.
4. RED: add `DeleteSession` regressions for the only session and one of two shared
   sessions, with a spy that fails on cleanup-job activation or physical cleanup.
5. GREEN: implement the prepared-job barrier and task-row serialization for both
   database engines.
6. REFACTOR: make lock ordering explicit in comments/tests and keep Git locks
   outside the database transaction.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite/... -run 'Cleanup|Barrier|Session'
cd apps/backend && go test ./internal/task/service/... -run 'Cleanup|Archive|Delete'
cd apps/backend && go test ./internal/orchestrator/... -run 'DeleteSession|Environment|Worktree'
```

## Files likely touched

- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/repository/sqlite/resource_cleanup.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/worktree/store.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/executor/task_environment.go`
- `apps/backend/internal/orchestrator/executor/executor_environment_reuse.go`

## Dependencies

- Task 01 supplies the canonical environment-repository transaction and
  compensation seam.

## Inputs

- Existing task operation locks and worktree manager target-path/repository lock
  order.
- Existing prepared/activated `task_resource_cleanup_jobs` lifecycle.

## Output contract

Report the exact serialization point for SQLite and PostgreSQL, both tested race
orderings, the lock-order proof, materialization compensation result, and a call
trace showing that session deletion performs no filesystem, Git, branch, or
cleanup-job action.
