---
status: draft
system: tasks
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-001
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-002
created: 2026-09-21
updated: 2026-09-24
owners:
  - cfl
---

# Dirty Worktree Task Archive System Design

## Purpose and boundaries

This design extends task runtime cleanup so archiving a task preserves
uncommitted local work. It defines how archive cleanup detects local worktree
changes, keeps the affected checkout, and later reclaims it when safe. It also
defines the storage-maintenance boundary for a retained physical checkout.

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
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.1-.4 | Archived worktree recheck below |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.5 | Storage protection and existing quarantine below |
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.6 | Archive confirmation below |

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
cleanliness is not. Archive uses the existing branch-preserving cleanup path.
Clean worktrees are removed, and the manager may compact a managed branch after
proving that it is fully integrated. A dirty worktree is filtered out before
cleanup, so its branch ref and `task_environment` row remain.

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
status and its branch is untouched. The record remains available for a later
cleanup operation after the checkout becomes clean. The archive job does not
schedule another attempt solely because the checkout was dirty, because this
preservation is not a cleanup error.

### Archived worktree recheck

The [reclamation decision](../../../decisions/2026-09-24-archived-worktree-reclamation.md)
assigns this follow-up to task lifecycle, not install-wide storage maintenance.
Extend the existing `Service.runTaskResourceCleanupWorker` loop with a bounded
archived-worktree pass. The worker already starts independently of
`StorageMaintenanceSettings.Enabled` and stops with the backend. It must not
reopen a succeeded archive job or consume its eight-attempt failure budget.

`task_environment_repos` remains the physical worktree authority. Add an
`archive_reclaim` trigger and a `waiting_for_clean` state to the existing
`task_resource_cleanup_jobs` model. For each worktree skipped because it is
dirty, create one follow-up job keyed by task, worktree, and the task's
`archived_at` value. Its snapshot records that archive value and the worktree
ID, not another
copy of the full task cleanup inventory. Persist the follow-up intent before
the original archive job succeeds. The follow-up never re-runs runtime,
attachment, or other task cleanup.

The one-minute cleanup worker already claims due jobs in batches of 100.
Include due `waiting_for_clean` jobs in that query. After a dirty check, set
`next_attempt_at` to 24 hours later without storing a cleanup error or
consuming the ordinary eight-attempt failure budget. Dirty jobs stay out of
the due query until that time, so older dirty jobs cannot starve clean ones.
On startup, a bounded, idempotent reconciliation creates missing follow-up
jobs for active archived worktree rows left by older versions. It excludes
rows with a still-running original archive job and uses the same stable
operation identity, so a restart does not duplicate jobs. An inspection or
database failure is reported and retried; it is not treated as cleanliness.

For each claimed job, reload the task and worktree through their authoritative
repositories. Recheck the exact `archived_at` value, current owner, active
references, path and Git registration, and cleanliness at the mutation
boundary. `CancelArchiveTaskResourceCleanup` includes follow-up jobs: an
unarchive cancels pending and waiting jobs and refuses to race one already
running. A task that transfers ownership leaves the candidate set. The
worktree manager holds its existing path and repository locks and applies a
final cleanliness guard inside the audited archive removal path, close to
physical removal. If the final check sees changes or fails, it leaves the
checkout and active row intact. A successful removal uses
`CleanupWorktreesWithReceipt`/the branch-preserving archive policy and marks
only that repository worktree deleted. Other repositories remain independent.
The manager's audit, path identity, and branch-compaction rules remain in
force. Do not run the whole task resource cleanup sequence again.

The recheck reports bounded counts for considered, reclaimed, dirty, in-use,
and failed worktrees. Inspection and mutation errors are logged with task and
worktree IDs, without file contents. Dirty is a deferred state, not a failed
archive job. Restart resumes waiting jobs from their due timestamp. An absent
worktree with unproven ownership is not treated as clean; the existing
worktree recovery rules govern it.

### Storage protection and existing quarantine

`storageInventory.activeWorktreePaths` currently filters out archived tasks.
Include every active `task_environment_repos` physical worktree path regardless
of the owner's archive marker. A deleted row with a historical branch remains
unprotected. `workspaces.buildProtectedSet` already protects a candidate root
when one descendant is in this inventory, so a multi-repository root is kept
while any sibling checkout remains active. A borrowed environment remains
protected by the existing borrower rule.

Protection must also be checked before `workspaces.Provider.PermanentDelete`
and `PermanentDeleteForce` remove a previously quarantined task root. Compare
the entry's original path with the current active worktree inventory. If an
active recorded path is within that root, leave the entry restorable and
report it as protected, even after retention or with force confirmation. If
inventory cannot be loaded, fail closed. The Storage page's existing Restore
action remains the recovery path for an entry quarantined before this change;
the lifecycle worker does not inspect or delete its relocated payload.
Storage analysis classifies these retained roots as protected, not orphan
candidates. No storage setting or global schedule default changes.

### Archive confirmation

`task-cleanup-summary.ts` also supplies task-delete confirmation. Add an
archive-specific summary selection and update only its archive callers. Keep
the delete warning and discard-consent copy unchanged. The archive summary
used by desktop confirmation, compact inline confirmation, and the phone
confirmation sheet must say
that a checkout with Git changes is retained and that a retained checkout is
removed after it becomes clean. It must not claim that every worktree branch
is deleted: unpublished branches remain recoverable. Keep the existing
`confirm_task_archive` bypass and archive API unchanged. Add translations in
the supported locale catalogs and update public task/storage documentation.
The existing mobile confirmation composition and touch targets stay in place;
only their shared copy changes.

## Out of scope

- Refusing archive, or adding a discard-consent flag and a dirty-file list to
  the archive API. Surfacing the dirty set so the interface can offer an
  explicit discard is a separate user-visible contract change.
- The sibling `removeBranch=false` callers in branch-handoff cleanup and the
  orphan worktree reaper, which have different triggers and admission rules.
- Any change to the delete-path guard.
- Automatically deleting ignored files or unrelated non-Git task files; the
  existing Git change-inspection contract and separate storage policy apply.
- Making scheduled storage maintenance run by default.
