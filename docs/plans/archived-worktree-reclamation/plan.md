---
created: 2026-09-24
status: complete
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-001
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-002
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-archive.md
legacy_specs: []
---

# Implementation Plan: Reclaim Retained Archived Worktrees

## Overview

Protect retained archived checkouts from generic storage deletion first. Then
give each retained worktree a durable, default-on task-lifecycle recheck that
removes it after it becomes clean. Finally, correct archive confirmation and
operator documentation so users know what archive keeps and when it is removed.
The [architecture decision](../../decisions/2026-09-24-archived-worktree-reclamation.md)
keeps optional install-wide storage scheduling separate from this lifecycle.

## Scope

### In scope

- Protect active archived worktree paths from workspace quarantine and purge,
  including entries quarantined before this change and manual force clear.
- Schedule and process durable per-worktree archive follow-up jobs while the
  storage scheduler is disabled; defer dirty checkouts without counting them
  as failed archive jobs.
- Revalidate archive identity, ownership, references, path, and Git changes at
  removal, including a second cleanliness check after the repository cleanup
  script. Fence archive and reclaim jobs for every cascade member before
  unarchive mutation, and restore the exact cancelled jobs if mutation fails.
  Preserve the current branch-compaction policy and multi-repository
  independence.
- Correct the shared archive confirmation summary on desktop and phone, with
  supported translations and public lifecycle documentation.

### Out of scope

- Enabling the install-wide storage scheduler or changing its default.
- A new archive API consent field or a new per-user retention setting.
- Automatic deletion of unrelated non-Git task files, remote task folders, or
  ignored files outside the existing Git change-inspection contract.
- Reopening a succeeded archive cleanup job or changing the task-delete dirty
  checkout guard.

## Technical approach

### Storage protection

In `backendapp/storage_inventory.go`, include active physical worktree rows of
archived tasks in `WorktreePaths`; keep deleted branch-history rows excluded.
`workspaces.Provider` uses the same complete authoritative inventory before
quarantine and again before permanent deletion of a previously quarantined
task root. An active worktree descendant protects the whole root, even when
retention elapsed or `Force clear all` was confirmed. Fail closed if inventory
cannot be loaded. Keep the existing manual Restore path for protected entries
already in quarantine. Reconcile the storage-maintenance design and public
operations guide with this distinction.

### Task-owned recheck

Extend `task_resource_cleanup_jobs` with an `archive_reclaim` trigger and a
non-error `waiting_for_clean` state. On a dirty archive skip, persist a
follow-up job keyed by task ID, worktree ID, and `archived_at`, before the
original cleanup job succeeds. The follow-up snapshot identifies one worktree
and one archive generation. Reconcile pre-existing archived active worktree
rows into missing follow-up jobs in bounded startup/worker batches. Extend
`ListDueTaskResourceCleanupJobs` to include due waiting jobs; defer dirty or
in-use candidates for 24 hours so an old dirty job cannot starve later jobs.
The existing worker remains active regardless of
`StorageMaintenanceSettings.Enabled`.

Run this trigger through the task cleanup claim and cancellation boundary, but
execute only worktree cleanup. `CancelArchiveTaskResourceCleanup` must include
these jobs so unarchive cancels waiting work and refuses a running cleanup.
Before removal, require the original `archived_at` value, current worktree
ownership, no active borrower, matching path/registration, and a clean Git
status. Add a final clean gate inside the manager's path/repository lock for
the archive removal path; a failed or dirty check retains the active row.
Success uses the existing branch-preserving cleanup and its historical row.
Keep bounded outcome logs/metrics without file contents.

### User-facing explanation

Add an archive-specific selection in `task-cleanup-summary.ts` so all archive
confirmation surfaces explain conditional checkout cleanup and branch
retention. Keep delete confirmation's existing warning and discard-consent
copy. Keep the current archive confirmation preference and phone sheet
composition.
Translate changed keys in all required catalogs. Update
`docs/public/tasks-and-workflows.md` and `docs/public/operations.md` to explain
the default-on lifecycle recheck, optional storage scheduler, and protected
archived worktrees.

## ASCII UI preview

UI-01: Existing desktop archive confirmation, worktree task (AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.6)

```text
+-----------------------------------------------------+
| Archive task?                                       |
| Archive "Example task".                             |
| * Clean Git worktrees are removed.                  |
| * A worktree with Git changes stays until clean.    |
|   Unpublished branches remain available.           |
|                                                     |
|                         [Cancel] [Archive]          |
+-----------------------------------------------------+
```

UI-02: Existing phone archive confirmation sheet, same outcome and copy

```text
  +-----------------------------------------------+
  | Archive task?                                 |
  | Archive "Example task".                       |
  | Clean Git worktrees are removed.              |
  | A worktree with Git changes stays until clean.|
  | Unpublished branches remain available.       |
  |                                               |
  | [Cancel]                    [Archive]          |
  +-----------------------------------------------+
```

These previews require the conditional cleanup message and the existing action
order. Exact line wraps are illustrative. The desktop confirmation and phone
sheet keep their current scroll owner, safe-area handling, touch targets, and
preference bypass. `task-archive-confirm-dialog.tsx` and the phone path of
`TaskArchiveConfirmation` are the nearest shipped surfaces. No new overlay or
navigation state is proposed.

## Tests

| Acceptance criteria | Evidence to add or update |
| --- | --- |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.5 | `storage_inventory_test.go`, `workspaces/provider_test.go`, `storage_maintenance_test.go`: archived active versus deleted rows; multi-repository root; pre-existing quarantine, purge, force, and inventory failure. |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.1-.3 | `resource_cleanup_jobs_test.go` and `sqlite/resource_cleanup_test.go`: dirty deferred state, due/fair batch, restart/backfill, clean per-repository removal with storage scheduling off, failure preservation. |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.4 | Service race tests: single and cascade unarchive while reclaim runs, ownership transfer before claim, cancellation rollback; manager checks cleanliness both before and after cleanup scripts. |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.6 | `task-cleanup-summary.test.ts`, archive and delete confirmation component tests, locale checks, desktop and phone Playwright assertions. |

## E2E tests

- Desktop `e2e/tests/task/archive-confirmation-preference.spec.ts`: worktree
  confirmation communicates retention and later removal; preference bypass
  remains unchanged (AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.6).
- Phone `e2e/tests/kanban/mobile-card-archive-confirmation.spec.ts`: the same
  lifecycle message is readable and the existing archive action works on the
  phone sheet (AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.6).
- Backend integration exercises the actual worker and storage-disabled setting
  with a controllable clock, since waiting 24 hours in browser E2E is not a
  meaningful browser test (AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.1-.5).

## Work orders

- [x] [Task 01: Protect archived worktrees from storage purge](task-01-protect-archived-worktrees.md)
- [x] [Task 02: Reclaim clean archived worktrees through task lifecycle](task-02-reclaim-clean-archived-worktrees.md)
- [x] [Task 03: Explain retained worktrees in archive UI and docs](task-03-explain-archive-reclamation.md)
- [x] Review fixes: cascade-wide cleanup fencing, post-script dirty retention,
  and task-row serialization of reclaim backfill against unarchive.

## Dependency order

```text
Task 01 -> Task 02 -> Task 03
```

Storage protection ships before any default-on recheck. The UI describes the
behavior only after the backend delivers it.

## Verification results

- Task 01: `go test ./internal/backendapp ./internal/system/storage/workspaces` and `make lint` passed.
- Task 02: `go test ./internal/task/service ./internal/task/repository/sqlite ./internal/worktree` and `make lint` passed. After the final service helper refactor, `go test ./internal/task/service` and `make lint` passed again.
- Review fixes: focused cascade/reclaim, waiting-job cancellation, rollback,
  post-script cleanliness, late-dirty retention, and mixed-error tests passed.
  Full service and worktree package tests passed; SQLite package tests passed;
  `make lint` passed with 0 issues.
- Backfill/unarchive fence: full `go test ./internal/task/service -count=1`,
  focused SQLite unarchive/cancellation tests, `make lint`, specification
  catalog validation, and specification lint passed.
- Task 03: 95 targeted Vitest tests, `pnpm run i18n:check`, desktop and phone E2E (3 and 2 tests), and public-doc validation passed.

## Risks

- Existing quarantined archived checkouts need an explicit protected purge
  outcome and may need manual Restore before the lifecycle worker can inspect
  their original path.
- The current Git inspector does not report ignored files. This package keeps
  that existing definition of dirty; it must not present ignored-file safety
  as a new guarantee.
- Archive and delete currently share cleanup summary helpers. The new copy
  must be archive-specific so deletion still warns about its own outcome.
- Files can change externally between Git inspection and deletion. A final
  check inside the manager lock narrows this interval but cannot make external
  writers use Kandev's lock; preserve fail-closed ownership and path checks.
- Follow-up jobs must not be classified as failed archive jobs or race a task
  unarchive. The same lifecycle claim/cancellation path is required, including
  cascade-wide fencing, atomic backfill/unarchive serialization, and restoration
  of all cancelled jobs after a failed unarchive mutation. A late dirty refusal
  is a retained worktree outcome; other cleanup errors must still reach retry
  handling.
