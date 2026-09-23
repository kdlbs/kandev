---
created: 2026-09-21
status: draft
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-001
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-archive.md
legacy_specs: []
---

# Implementation Plan: Preserve Dirty Worktrees On Archive

## Overview

Stop task archive from force-removing a Git worktree that holds uncommitted or
untracked work. Archive keeps admitting the task; it filters the worktree
cleanup set so a dirty checkout survives.

## Confirmed root cause

- `internal/worktree.Manager.auditCleanupBranchDisposition` returns before the
  clean-checkout gate whenever `removeBranch` is false, so the gate is coupled
  to branch deletion rather than to directory removal.
- `CleanupWorktreesPreservingBranches` always passes `removeBranch=false`.
- `completeOwnedWorktreeCleanup` then force-removes the directory once Git
  registration ownership is proven. Ownership is checked; cleanliness is not.
- All three archive entry points converge on the `envCleanup.preserveBranches`
  branch of `Service.cleanupDestructiveTaskResources`.
- Reproduced directly against `CleanupWorktreesPreservingBranches`: the call
  returned `nil` and removed a worktree holding an untracked file and a tracked
  modification. The delete path refuses the identical input.

## Scope

### In scope

- Inspect the eligible worktree set before archive cleanup and drop every
  worktree reporting tracked or untracked changes.
- Preserve the checkout, its branch, and its active worktree record.
- Archive the task regardless of the inspection outcome.
- Preserve every worktree in the set when inspection itself fails.
- Keep removing clean worktrees, including clean repositories inside a
  multi-repository task.
- Cover the cascade entry point and single-task `Service.ArchiveTask`.

### Out of scope

- Refusing archive, or adding a discard-consent flag and a dirty-file list to
  the archive API.
- The sibling `removeBranch=false` callers in branch-handoff cleanup
  (`handoff_cleaner.go`) and the orphan worktree reaper
  (`event_handlers_automation.go`).
- Any change to the delete-path guard or to branch preservation on archive.
- Making archive delete branches.

## Technical approach

`Service.cleanupDestructiveTaskResources` gains an admission filter on the
`envCleanup.preserveBranches` path, immediately before
`CleanupWorktreesPreservingBranches`. It reuses the existing
`WorktreeDirtyInspector.InspectDirtyWorktrees`, which is already wired into the
service for the delete path and already inspects every recorded worktree
read-only.

Filtering is per-worktree, not all-or-nothing. Preserved worktrees are simply
never passed to cleanup, so their records and branches need no separate
handling. No database, API, or WebSocket schema change is planned.

## Work orders

- [pending] [Task 01: Preserve Dirty Worktrees On Archive](task-01-preserve-dirty-archive-worktrees.md)

## Dependency order

```text
Task 01
```

The package is a single work order.

## Verification strategy

- A worktree-manager regression proves a dirty checkout survives
  `CleanupWorktreesPreservingBranches` for an untracked file and for a tracked
  modification.
- Task-service tests prove both archive entry points preserve a dirty worktree,
  still remove a clean one, and preserve the whole set when inspection fails.
- A multi-repository test proves a clean repository is still reclaimed while a
  dirty sibling survives.
- Backend package tests and lint cover the affected packages.

## Risks

- Dirty checkouts accumulate on disk after archive. This is the accepted
  tradeoff for never destroying uncommitted work, and preserved worktrees stay
  eligible for later cleanup once clean.
- `InspectDirtyWorktrees` runs `git status` per worktree, adding bounded work to
  archive cleanup. It is already used this way on the delete path.
- Backend tests that rely on `t.TempDir()` fail on macOS because `/var` is a
  symlink and the no-follow path check rejects it. Run affected worktree tests
  with a non-symlinked `TMPDIR`.
