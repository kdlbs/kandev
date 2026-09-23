---
id: "01-preserve-dirty-archive-worktrees"
title: "Preserve dirty worktrees on archive"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-001
acceptance_criteria:
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.1
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.2
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.3
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.4
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.5
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.6
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.7
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-archive.md
---

# Task 01: Preserve Dirty Worktrees On Archive

## Summary

Filter worktrees with tracked or untracked local changes out of the archive
cleanup set so archive stops force-removing uncommitted work. Archive still
admits the task.

## In scope

- Add failing tests first, covering an untracked file and a tracked
  modification on both archive entry points.
- Inspect the eligible worktree set on the `envCleanup.preserveBranches` path of
  `Service.cleanupDestructiveTaskResources`, immediately before
  `CleanupWorktreesPreservingBranches`.
- Reuse `WorktreeDirtyInspector.InspectDirtyWorktrees`; do not add a second
  inspection mechanism.
- Drop each dirty worktree from the cleanup set and keep removing the clean
  remainder.
- Preserve the whole set and log a cleanup diagnostic when inspection fails;
  never fall through to removal.
- Archive the task regardless of the inspection outcome.

## Out of scope

- Refusing archive, or adding a discard-consent flag and a dirty-file list to
  the archive API.
- The sibling `removeBranch=false` callers in `handoff_cleaner.go` and
  `event_handlers_automation.go`.
- Any change to the delete-path guard, to `auditCleanupBranchDisposition`, or to
  branch preservation on archive.

## Files

- `apps/backend/internal/task/service/service_tasks.go` - archive admission
  filter on the `preserveBranches` path.
- `apps/backend/internal/task/service/*_test.go` - archive entry-point
  regressions.
- `apps/backend/internal/worktree/manager_cleanup_recovery_test.go` - manager
  regression for `CleanupWorktreesPreservingBranches`.

## Acceptance

- Archiving a task whose worktree holds an untracked file leaves the file and
  the directory on disk, and the task becomes archived.
- Archiving a task whose worktree holds a tracked modification leaves the edit
  in place, and the task becomes archived.
- Both the cascade entry point and single-task `Service.ArchiveTask` behave this
  way.
- A clean worktree is still removed. In a multi-repository task a clean
  repository is still reclaimed while a dirty sibling survives.
- A failed inspection preserves every worktree and still archives the task.
- A preserved worktree keeps its branch and its active record.
- The existing delete-path dirty guard is unchanged and still passes.

## Verification

```bash
cd apps/backend
TMPDIR=/private/tmp/kandev-verify go test ./internal/worktree/... ./internal/task/service/...
make lint
```

`TMPDIR` must not sit under a symlink; on macOS `t.TempDir()` returns a `/var`
path that the no-follow check rejects, which fails these tests before the code
under test runs.
