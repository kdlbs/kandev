---
status: draft
system: tasks
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-001
created: 2026-09-21
updated: 2026-09-21
owners:
  - cfl
---

# Dirty Worktree Task Archive System Design

## Purpose and boundaries

This design extends task runtime cleanup so archiving a task never destroys
uncommitted local work. It defines how archive cleanup detects local worktree
changes and preserves the affected checkout while still archiving the task.

It does not change task deletion, session deletion, branch-handoff cleanup, the
orphan worktree reaper, workspace reset, automatic commit or push behavior, or
branch redundancy rules. It adds no new refusal, consent flag, or error shape to
the archive API.

## Requirement mapping

| Requirement | Design source |
| --- | --- |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.1-.3 | Admission inspection and preservation below |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.4 | Failure and state handling below |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.5-.6 | Admission inspection below |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.7 | Out of scope below |

## Existing system context

`Dirty Worktree Task Deletion` states that it does not change archive. Nothing
has covered archive since, so archive has no dirty-checkout protection.

The worktree manager couples its clean-checkout gate to branch deletion rather
than to directory removal. `auditCleanupBranchDisposition` returns before
`verifyCleanRedundantCheckout` whenever `removeBranch` is false:

```go
if !removeBranch || branchRef == "" || branchOID == "" {
    return false, nil
}
if pathPresent && !options.DiscardWorktreeChanges {
    return m.verifyCleanRedundantCheckout(ctx, wt, branchRef)
}
```

`CleanupWorktreesPreservingBranches` always passes `removeBranch=false`, so every
archive skips the gate. `completeOwnedWorktreeCleanup` then force-removes the
directory once Git registration ownership is proven. Ownership is checked;
cleanliness is not. Archive preserves the branch ref, so committed work survives
and only uncommitted work is destroyed.

Three archive entry points converge on one site. `HandoffService.archiveTaskTree`
calls `CleanupTaskResources(ctx, taskID, false)`, `Service.ArchiveTask` builds
`taskEnvironmentCleanup{preserveBranches: true}`, and the durable cleanup job
sets `preserveBranches: job.IsArchive()`. All three reach the
`envCleanup.preserveBranches` branch of
`Service.cleanupDestructiveTaskResources`.

## Design

### Preserve rather than refuse

Archive admits the task unconditionally and filters the cleanup set instead of
rejecting the operation. Archive is routine board maintenance; a refusal would
block tidying the board behind a checkout the user may not care about, and a
consent flag would train users to pass discard reflexively, recreating the
defect. Preservation also matches archive's existing posture, which already
retains the branch ref and the `task_environment` row for recovery.

### Admission inspection

`Service.cleanupDestructiveTaskResources` inspects the eligible worktree set
before calling `CleanupWorktreesPreservingBranches`. It reuses the existing
`WorktreeDirtyInspector.InspectDirtyWorktrees`, which runs
`git status --porcelain=v1 --untracked-files=normal -z` read-only over every
recorded worktree and deduplicates by repository and cleaned path.

Worktrees reported dirty are removed from the cleanup set. The remaining clean
worktrees proceed through the unchanged audited removal path. Filtering is
per-worktree rather than all-or-nothing, so a multi-repository task still
reclaims its clean repositories. The inspector already walks every recorded
worktree, which covers each `task_environment_repos` row rather than only the
session workspace path.

### Failure and state handling

Inspection failure preserves every worktree in the set and archives the task.
The failure is logged and counts as a cleanup diagnostic, never as a fall-through
to removal.

A preserved worktree is never passed to cleanup, so its record keeps its active
status and its branch is untouched. It remains eligible for a later cleanup
attempt once the checkout is clean, through the existing retry and unarchive
paths.

## Out of scope

- Refusing archive, or adding a discard-consent flag and a dirty-file list to
  the archive API. Surfacing the dirty set so the interface can offer an
  explicit discard is a separate user-visible contract change.
- The sibling `removeBranch=false` callers in branch-handoff cleanup and the
  orphan worktree reaper, which have different triggers and admission rules.
- Any change to the delete-path guard.
