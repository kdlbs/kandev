---
id: "02-reclaim-clean-archived-worktrees"
title: "Reclaim clean archived worktrees through task lifecycle"
status: pending
wave: 2
depends_on:
  - "01-protect-archived-worktrees"
plan: "plan.md"
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-001
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-002
acceptance_criteria:
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.3
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.1
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.2
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.3
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.4
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-archive.md
---

# Task 02: Reclaim clean archived worktrees through task lifecycle

## Summary

Give each retained archived worktree a durable follow-up job. The existing
task cleanup worker revisits it without storage scheduling, defers it while
dirty, and uses the normal audited archive worktree cleanup once clean.

## In scope

- Extend the cleanup job trigger/state and due query with a non-error
  `waiting_for_clean` outcome and 24-hour recheck interval. Use a stable
  operation identity per task, worktree, and `archived_at` value.
- Persist follow-up intent before archive cleanup succeeds; idempotently
  reconcile archived active rows from older versions in bounded batches.
- Extend archive-job cancellation/claim handling so an unarchive cancels
  waiting work and cannot race a running recheck. Revalidate ownership,
  references, archive identity, path, Git registration, and cleanliness before
  removal; add the final clean gate under the manager's existing locks.
- Reuse the archive branch policy, preserve per-repository decisions, and
  expose bounded outcomes without file contents.

## Out of scope

- Re-running runtime, attachment, or whole-environment cleanup.
- A new user setting or a change to the task-delete dirty guard.
- Generic orphan directories or ignored files outside the existing Git
  inspection contract.

## Acceptance

- A dirty checkout and a failed inspection remain intact; other due jobs and
  clean siblings still progress, including after restart.
- A retained checkout that becomes clean is reclaimed with storage scheduling
  disabled, with branch history preserved by the existing manager policy.
- An unarchive, ownership transfer, active borrower, or new Git change before
  mutation prevents removal; retries remain durable and bounded per pass.

## Verification

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/repository/sqlite ./internal/worktree)
(cd apps/backend && make lint)
```

## Files likely touched

- `apps/backend/internal/task/models/resource_cleanup.go`
- `apps/backend/internal/task/repository/sqlite/resource_cleanup.go`
- `apps/backend/internal/task/repository/sqlite/resource_cleanup_test.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/resource_cleanup_jobs_test.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/worktree/manager_cleanup.go`
- `apps/backend/internal/worktree/manager_cleanup_audit.go`
- `apps/backend/internal/worktree/manager_cleanup_dirty.go`
- `apps/backend/internal/worktree/manager_cleanup_recovery_test.go`

## Dependencies

Task 01 protects retained worktrees before automatic reclamation is added.

## Risks

- A succeeded archive job must stay succeeded; a dirty follow-up is a waiting
  lifecycle state, not a failed archive attempt.
- Backfill and due selection must be idempotent and fair across SQLite and
  PostgreSQL; tests should cover restart and more candidates than one batch.
- A Git change after the first inspection must be caught by the final manager
  gate without changing unrelated worktree cleanup callers.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-001` and `-002`, the paired system design,
  the reclamation ADR, and task runtime cleanup contracts.
- Existing cleanup job worker, store, unarchive, and worktree manager tests.

## Results

Pending.
