---
created: 2026-10-30
status: implemented
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-SHARED-WORKTREE-HOLDS-001
system_design:
  - ../../specs/tasks/system-design/shared-worktree-holds.md
  - ../../specs/tasks/system-design/runtime-cleanup.md
legacy_specs: []
---

# Implementation Plan: Archived Borrower Worktree Release

## Overview

Fix [issue #4973](https://github.com/kdlbs/kandev/issues/4973). A child task that
inherited its parent's workspace permanently pins the parent's worktree after
both tasks are archived. The correction narrows one shared store predicate, so
every cleanup path picks it up. Existing pinned worktrees are reclaimed by the
retained jobs and startup reconciliation with no migration.

## Root cause

`SQLiteStore.CountActiveWorktreeReferences` in
`apps/backend/internal/worktree/store.go` counts every session of another task
that references the owning environment. It does not check session state,
borrower archive state, or runtime rows. Its doc comment expects the borrower's
own cleanup to release the association, but nothing does: a borrower's inventory
contains no worktree rows, and the single shared `task_environment_repos` row
cannot be released per borrower. This violates AC-TASKS-RUNTIME-CLEANUP-001.3,
which defers teardown only while an active session holds the environment.

Reproduced with a temporary test using `newReferenceCleanupTestManager`: an
archived owner and an archived borrower with a `CANCELLED` session gave
`references=1`, receipt `RetainedReasons:map[active_reference:1]`, and a checkout
still on disk. The test was deleted after the run.

## Scope

### In scope

- Implement REQ-TASKS-SHARED-WORKTREE-HOLDS-001 in the shared predicate.
- Cover the predicate, manager removal, reclaim, and restart recovery.
- Correct the code comments and the public statement that contradict the rule.

### Out of scope

- Waking the owner's reclaim job when a borrower is archived. The existing
  24-hour recheck and startup wake bound the delay.
- Releasing archived `IDLE` Office sessions. They stay protected.
- Clearing `task_sessions.task_environment_id` on archive. It would destroy
  environment history and require restoration on unarchive.
- Remote task-directory reclamation and container or sandbox teardown, which use
  separate guards.
- Schema changes, new settings, and UI changes.

## Technical approach

Change the query in `CountActiveWorktreeReferences`:

```sql
LEFT JOIN tasks bt ON bt.id = s.task_id
...
AND NOT (
  bt.archived_at IS NOT NULL
  AND s.state IN (?, ?, ?)
  AND NOT EXISTS (SELECT 1 FROM executors_running er WHERE er.session_id = s.id)
)
```

Bind the three states from `models.TaskSessionStateCompleted`, `Failed`, and
`Cancelled`, in `WHERE` order before the optional exclusion list. The `LEFT JOIN`
fails closed: a session with no joinable task row still counts. The runtime check
matches reclaim's rule that any retained runtime defers removal, and covers a
borrower whose stop failed.

Callers need no change. `filterSharedWorktreesForTaskCleanup`,
`archiveReclaimWorktreeInUse`, and `removeWorktreeWithReceipt` share the
predicate. The owner's identity, path, Git registration, and branch checks stay
in force.

Update the doc comments on `CountActiveWorktreeReferences` (`store.go`) and the
interface and `Manager` method (`manager.go`). Correct "An archived borrower
session can continue to protect a shared checkout" in
`docs/public/git-operations.md`, using `/docs-maintainer`.

## Tests

| Criterion | File | Test |
| --- | --- | --- |
| AC-TASKS-SHARED-WORKTREE-HOLDS-001.2 | `internal/worktree/manager_cleanup_archived_borrower_test.go` | `TestCountActiveWorktreeReferences_ArchivedBorrowerMatrix` |
| AC-TASKS-SHARED-WORKTREE-HOLDS-001.2 (unjoinable task row) | same | `TestCountActiveWorktreeReferences_UnjoinableBorrowerTaskHolds` |
| AC-TASKS-SHARED-WORKTREE-HOLDS-001.2 (fails before the fix) | same | `TestCleanupWorktrees_RemovesWorktreeHeldOnlyByArchivedBorrower` |
| AC-TASKS-SHARED-WORKTREE-HOLDS-001.1 (mixed state) | same | `TestCleanupWorktrees_RetainsWorktreeWithMixedBorrowers` |
| AC-TASKS-SHARED-WORKTREE-HOLDS-001.3 | `internal/task/service/resource_cleanup_archive_borrower_test.go` | `TestArchiveReclaimKeepsBorrowerHistoryAfterRelease` |
| AC-TASKS-SHARED-WORKTREE-HOLDS-001.1 | same | `TestArchiveReclaimDefersForLiveBorrower` |
| AC-TASKS-SHARED-WORKTREE-HOLDS-001.3 (existing data) | same | `TestResumeReclaimsWorktreePinnedByArchivedBorrower` |
| AC-TASKS-SHARED-WORKTREE-HOLDS-001.3 (deferred job wake) | same | `TestStartupWakeReclaimsDeferredArchiveJobAfterBorrowerArchive` |

The inventory-repair lifecycle controls asserted the previous protection for an
archived terminal borrower. Their cascade archive case now expects the released
checkout to be reclaimed:
`internal/task/inventoryrepair/legacy_alias_cascade_test.go` and
`internal/task/inventoryrepair/legacy_alias_lifecycle_test.go`.

The matrix covers one borrower per row:

| Borrower task | Session state | Runtime row | Counted |
| --- | --- | --- | --- |
| Archived | `COMPLETED`, `FAILED`, `CANCELLED` | No | No |
| Archived | `CANCELLED` | Yes | Yes |
| Archived | `CREATED`, `STARTING`, `RUNNING`, `WAITING_FOR_INPUT`, `IDLE` | No | Yes |
| Live | `COMPLETED`, `FAILED`, `CANCELLED` | No | Yes |

## E2E tests

None. The behavior has no rendered surface. The service tests exercise the real
manager, SQL jobs, and filesystem.

## Work orders

- [x] [Task 01: Release archived borrower holds](task-01-release-archived-borrower-holds.md)

## Verification results

- Focused worktree tests pass: the 12-case borrower matrix, the unjoinable
  task-row hold, removal of a worktree held only by an archived borrower, and
  mixed live/released retention all match the rule.
- Focused archive reclaim tests pass: borrower history survives release, a live
  borrower defers, restart reclaim removes a checkout pinned before this change,
  and startup wake succeeds for a deferred reclaim job whose future recheck
  means only the wake can admit it.
- The removal and deferred-job wake regressions reproduced `active_reference`
  retention before the query change and pass after it.
- The trusted PR documentation coverage evaluator reproduces locally as
  `covered`: the shared worktree design declares both requirements that the
  work order lists.
- The inventory-repair cascade controls pass with the new expectation that an
  archived terminal borrower releases the shared checkout.
- `go test -trimpath -race -tags fts5 -timeout 20m ./internal/task/service ./internal/worktree -count=1`: pass.
- `go test -trimpath -race -tags fts5 ./internal/task/inventoryrepair -count=1`: pass (47 s).
- `go run ./cmd/sqlguard ./internal`: pass.
- `go test -trimpath -race ./internal/persistence/storeconformance -count=1`: pass (160 s).
- `go build -trimpath ./...`: pass.
- `bash scripts/lint-go-changed` for every changed Go file: 0 issues.
- `node scripts/validate-public-docs.mjs`: 54 pages validated.
- `python3 scripts/lint-spec-files.py --all`: pass.
- `TestUpdateTask_PriorityOnlyUsesFieldScopedRepositoryWrite` now stops the
  cleanup worker before it swaps `svc.tasks`. The full race run exposed that
  pre-existing fixture race, unrelated to this predicate.

## Risks

- **Uncommitted borrower files are removed.** A shared checkout can hold a
  borrower's uncommitted work. After this fix it is deleted when the owner's
  worktree is reclaimed, as ADR-2026-10-17 already does for archived checkouts.
  Committed work follows the existing branch retention policy.
- **Up to 24 hours of delay** when the owner is archived before the borrower.
- **A retained runtime row still pins the worktree**, including a confirmed-dead
  row kept for its resume token. This matches the owner's reclaim rule, so such a
  worktree stays until the row is gone.
