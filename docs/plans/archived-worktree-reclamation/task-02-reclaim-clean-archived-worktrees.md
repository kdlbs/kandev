---
id: "02-reclaim-clean-archived-worktrees"
title: "Reclaim clean archived worktrees through task lifecycle"
status: done
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
  waiting work and cannot race a running recheck, including every member of a
  cascade unarchive. Retain the exact jobs cancelled before mutation and restore
  them if the mutation fails. Serialize reclaim insertion and unarchive on the
  task row: insert only for the exact archived generation, and abort the
  unarchive transaction if a reclaim job appeared after its cancellation scan.
  Revalidate ownership, references, archive
  identity, path, Git registration, and cleanliness before removal; add the
  final clean gate under the manager's existing locks after the repository
  cleanup script runs.
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
- A late dirty refusal from the manager retains that worktree without failing
  the original archive job; genuine cleanup failures remain retryable.
- A retained checkout that becomes clean is reclaimed with storage scheduling
  disabled, with branch history preserved by the existing manager policy.
- A single or cascade unarchive, ownership transfer, active borrower, or new
  Git change before mutation prevents removal; rollback restores all cleanup
  jobs cancelled before a failed unarchive mutation.
- A deterministic backfill/unarchive interleaving proves the cancellation scan
  cannot miss a candidate inserted before mutation and a stale candidate cannot
  be inserted after unarchive commits.

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
- `apps/backend/internal/task/service/handoff_cascade.go`
- `apps/backend/internal/task/service/handoff_service.go`
- `apps/backend/internal/task/service/service_tasks.go`
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
- Review regressions for cascade unarchive while reclaim is running and for a
  cleanup script that dirties a checkout after the archive admission check.

## Results

- `go test ./internal/task/service ./internal/task/repository/sqlite ./internal/worktree` passed.
- After the final service helper refactor, `go test ./internal/task/service` passed.
- Cascade reclaim fencing, cancellation rollback, waiting-job cancellation,
  late dirtiness after a cleanup script, mixed genuine errors, and worktree
  cleanup regressions passed with focused Go tests.
- A paused-backfill regression proved task-row serialization in both orders:
  an inserted candidate blocks cascade unarchive, while a late stale insert is
  skipped after unarchive commits.
- The full task-service suite, targeted SQLite unarchive/cancellation tests,
  and backend lint passed after the fence was added.
- `go test ./internal/task/service -count=1` passed after the review fixes.
- `go test ./internal/worktree -count=1` passed after the post-script gate fix.
- `go test ./internal/task/repository/sqlite` passed after the cancellation
  state-query update.
- `make lint` passed with 0 issues after the review fixes.
