---
status: current
system: tasks
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-001
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-002
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-003
created: 2026-09-21
updated: 2026-10-14
owners:
  - cfl
---

# Dirty Worktree Task Archive System Design

## Purpose and boundaries

This design extends task runtime cleanup so archiving a task preserves
uncommitted local work. It defines how archive cleanup detects local worktree
changes, keeps the affected checkout, and later reclaims it when safe. It also
defines the storage-maintenance boundary for a retained physical checkout and
removal of an archived task container after its last worktree is reclaimed.

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
| AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.1-.4 | Archived task container cleanup below |

## Existing system context

The completed archive-preservation package corrected the original dirty-checkout
defect described below. Current archive cleanup applies its clean-checkout guard
before directory removal and after repository cleanup scripts.

The original defect coupled the worktree manager clean-checkout gate to branch deletion rather
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

`CleanupWorktreesPreservingBranches` passes `removeBranch=false`; before the
archive-preservation correction that skipped the clean gate. `completeOwnedWorktreeCleanup` then force-removes the
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
running. Cascade unarchive applies this fence to every member before the first
task mutation. If a later member cannot be restored, it restores the exact
cleanup operations it cancelled so archived members remain eligible for
cleanup. Candidate insertion and each unarchive mutation also serialize on the
task row.
Candidate insertion requires the exact archived generation while holding that
lock. Unarchive checks for active archive jobs again in its transaction and
aborts if a backfill inserted one after the service's cancellation scan. If
unarchive wins the lock, a late backfill sees the task is active and skips the
stale candidate. A task that transfers ownership leaves the candidate set.
The worktree manager holds its existing path and repository locks and applies a
final cleanliness guard inside the audited archive removal path after the
repository cleanup script runs, close to physical removal. If that check
finds changes, it retains the checkout and active row; this is a retained
outcome for original archive cleanup, with its durable reclaim candidate
preserved. Other cleanup errors still reach retry handling. A successful
removal uses
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

### Archived task container cleanup

The task lifecycle owns this terminal step, as recorded in
[Archived task directory cleanup](../../../decisions/2026-10-13-archived-task-directory-cleanup.md).
`Manager.tryRemoveEmptyTaskDir` intentionally uses `os.Remove` after individual
worktree removal. Keep that empty-only behavior for partial worktree removal;
recursive removal there would bypass dirty sibling and borrower protection.

Introduce a narrow archive-only task-directory cleanup capability on the
worktree manager, consumed by the task service through a local interface. Its
request carries retained task/environment/root identity and the archive
operation identity. Resolve the root from the snapshotted task environment and
recorded `TaskDirName`, under the configured `TasksBasePath`. Require an exact
immediate-child managed root, consistent recorded paths, current owner and
archive identity, successful runtime stops, and no active borrower. A directory
name or lexical containment alone does not authorize deletion. The environment
workspace may name the root or an admitted checkout under it. A workspace path
unrelated to the admitted set contributes no recursive cleanup target.

Run this step after the original archive batch has removed all eligible
worktrees and after successful `archive_reclaim` removal, before completing the
job. Reload the complete owning environment repository inventory, including
siblings excluded from the removal batch. The condition is universal: every
recorded worktree under the root must have confirmed removal; one retained
active, dirty, failed, shared, or unknown row defers root deletion. A mixed
clean/dirty task still reclaims its clean worktree and retains its container.

At the mutation boundary reuse task lifecycle fencing, manager path locking,
and the existing `storage/workspaces` directory-handle primitives. Pin the
exact root using `OpenDirectoryNoFollow`, validate its ownership marker against
retained identity, verify its path again, then use `DirectoryHandle.RemoveDirectory`
for recursive deletion. Internal symlinks are removed without traversing their
targets. Reject replaced roots, symlinked roots or parents, foreign markers,
base-root targets, and inconsistent or out-of-base paths. Never route this
operation through a shell or a bare path-based recursive deletion fallback.

The durable cleanup snapshot must retain the root target and required identity
before removing its final worktree. Preserve existing proof and branch history;
`TaskDirectoryCleanup` persists the root path, ownership marker, native
filesystem identity (Unix device/inode or Windows volume/file index), current
task owner, and
admitted worktree paths in the existing snapshot JSON. The current task owner is
separate from the immutable creation marker: an admitted ownership transfer
preserves the marker and stable task-directory name. Absent fields keep older
jobs readable; capture authoritative proof before their worktree removal. A retry must still execute the root step when its worktree row has
already been marked deleted. In particular, `loadArchiveReclaimWorktree` currently
treats a non-active row as stale; the new pending root obligation must survive
that shortcut. Distinguish a deferred retained sibling from a removal failure.
Removal or inspection failures reach existing job retry handling. A folder
that becomes busy after the last checkout disappears remains a durable retry;
it cannot rely on future active-worktree discovery. A captured root with missing
deleted-worktree evidence also retains retry instead of completing as stale. Unarchive
invalidates the same operation before any root deletion. Absence succeeds only
with retained proof of the exact admitted target; generic not-found is not
ownership evidence. Existing empty-only parent removal must not erase the
identity evidence needed by this later step.

Root capture is optional for supported legacy checkout cleanup. A missing or
unsupported marker, changed tasks base, inventory spanning multiple roots, or
unrelated environment workspace means no recursive root target. Individual
checkout removal still runs through its existing Git/ownership audit. Capture
I/O, malformed ownership, conflicting ownership, and path-identity errors remain
retryable. A previously persisted root target never becomes eligible merely
because admission rules changed; its remover remains fail-closed.

Integrate with existing removed-root recording for orphan-process cleanup;
record a successfully removed task root along with the worktree roots. Do not
replay runtime or attachment cleanup from a per-worktree reclaim job.

The implementation updates the archive explanation in
`docs/public/tasks-and-workflows.md`: files beside managed worktrees disappear
when the last checkout is safely reclaimed. Public wording ships with the fix,
not during this design-only turn. No rendered UI changes are planned.

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
- Deleting files outside an admitted archived task root or reclaiming unrelated
  orphan directories. Those remain subject to separate storage policy.
- Making scheduled storage maintenance run by default.
