---
status: active
system: tasks
created: 2026-09-21
updated: 2026-10-18
owners:
  - cfl
---
# Dirty Worktree Task Archive Requirements

## Overview

Archive releases managed workspaces as part of the archive operation, including
tracked, untracked, and ignored files. The user explicitly requests removing
dirty-worktree preservation. Tasks owns this authorization and the durable
cleanup lifecycle; workspace primitives implement filesystem removal.

Requirement 004 defines the current archive contract. It
supersedes requirement 001, criteria 002.1 through 002.4 and 002.6, and the
dirtiness/cleanliness conditions in 003.2. Those earlier criteria remain below
as historical references for completed delivery packages; they do not override
004. Storage protection in 002.5 and the remaining task-directory requirements
continue to apply.

[Task runtime cleanup](runtime-cleanup.md) continues to own stop ordering,
ownership, durability, and retry. Archive adds no discard flag or new prompt.

## Requirements

### REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-001: Archive Preserves Uncommitted Work (superseded by 004)

**Intent:** Archiving a task shall never destroy uncommitted or untracked local
changes. Archive keeps admitting the task and preserves the affected checkout
instead of refusing the operation, because a refusal would block board hygiene
behind a checkout the user may not care about, and a discard-consent flag would
train users to pass discard reflexively.

#### Acceptance criteria

- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.1:** When archive cleanup reclaims a Git worktree whose checkout holds tracked or untracked local changes, the system shall preserve that checkout on disk.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.2:** The task shall still become archived and leave active task views, whatever the checkout state.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.3:** A preserved worktree shall retain its branch and its active worktree record, and shall remain eligible for a later cleanup operation once it is clean.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.4:** When the change inspection itself fails, the system shall preserve every worktree in the cleanup set and shall not fall through to removal.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.5:** A clean worktree shall still be removed. In a multi-repository task the decision shall be made for each recorded worktree, so a clean repository is reclaimed while a dirty sibling is preserved.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.6:** The rule shall hold for cascade archive and for single-task archive, including scheduled auto-archive, because all archive cleanup shares one path.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-001.7:** Archive shall continue to use the existing branch-preserving cleanup policy. A dirty worktree skipped by archive shall retain its branch. A clean worktree shall follow the manager's existing branch-compaction policy, including compaction of a fully integrated managed branch. The existing task-delete dirty-checkout guard shall be unchanged.

### REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-002: Eventual Reclamation of Retained Worktrees (partially superseded by 004)

**Intent:** A retained checkout shall be reclaimed after it becomes safe to
remove, without requiring the operator to enable install-wide storage
maintenance or risking the loss of local changes.

#### Acceptance criteria

- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.1:** When an archived task retains a
  dirty Git worktree, the system shall revisit that checkout by default while
  the task remains archived, including after a backend restart and while
  scheduled storage maintenance is disabled.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.2:** When a retained checkout becomes
  clean and no active task uses it, a later successful lifecycle pass shall
  remove that worktree and apply the normal archive branch policy. Other
  repositories in the same task shall be decided independently.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.3:** When a checkout remains dirty,
  an ownership or cleanliness check fails, or an active task uses it, the
  recheck shall leave its files, branch, and active worktree record intact.
  A busy or dirty checkout shall not prevent other eligible checkouts from
  being reconsidered.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.4:** When a task is unarchived or its
  worktree ownership changes before removal, a delayed recheck shall not
  remove the active or newly owned checkout.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.5:** Storage maintenance shall not
  quarantine or permanently delete a workspace that contains a recorded,
  active Git worktree, including one owned by an archived task. This shall
  also protect an entry that was already quarantined before the rule changed;
  a force-clear action shall not bypass the ownership check.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.6:** When archive confirmation is
  shown on desktop or phone, its worktree summary shall explain that local
  changes are retained and that a retained checkout is removed after it
  becomes safe. The user's choice to bypass archive confirmation and
  programmatic archive behavior shall remain unchanged.

### REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-003: Archived Task Directory Cleanup

**Intent:** Once archive has reclaimed every task-owned worktree, the enclosing
managed task directory shall be removed with any files and nested folders the
agent created outside those worktrees.

#### Acceptance criteria

- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.1:** When archive cleanup has removed
  every owned worktree and the task environment is no longer in use, the system
  shall recursively remove the managed task directory, including loose files,
  nested directories, and its ownership marker. The tasks base directory,
  other tasks, and paths outside this task directory shall remain intact.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.2:** When at least one worktree remains
  dirty, active, shared, failed, or of uncertain ownership, or an owned runtime
  has not stopped, the system shall preserve the enclosing directory. Cleaning
  and reclaiming the last retained worktree shall complete directory cleanup.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.3:** When directory deletion fails after
  worktree removal, cleanup shall retain retryable intent across restart and
  shall not report that required directory cleanup succeeded. A proven prior
  removal shall complete idempotently. Unarchive or ownership change shall
  prevent delayed cleanup from deleting an active or newly owned directory.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.4:** The same behavior shall apply to
  direct, cascade, and scheduled archive and later archived-worktree reclamation.
  Removing an individual session or worktree shall not authorize recursive
  removal of the enclosing task directory.

### REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-004: Immediate Managed Worktree Removal

**Intent:** Archiving a task shall delete its managed worktrees without waiting
for local changes to be committed or discarded separately. Uncommitted files
are intentionally lost; committed branch recovery retains its existing policy.

#### Acceptance criteria

- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.1:** When archive cleanup has stopped
  the relevant runtimes and established exclusive ownership, it shall remove
  managed Git worktrees containing staged, unstaged, untracked, or ignored work
  during that cleanup attempt. Dirtiness alone shall not retain a checkout,
  fail archive, or schedule a cleanliness recheck.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.2:** The same policy shall apply to
  single-task, bulk, cascade, scheduled, and programmatic archive. In a task
  with clean and dirty repositories, every otherwise eligible worktree shall
  be removed independently of its local changes.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.3:** When all recorded worktrees have
  been removed, cleanup shall remove the admitted managed task folder and
  its contents in the same attempt. At least one shared, failed, or
  uncertain-ownership checkout, or an unstopped owned runtime, shall continue
  to prevent root removal. Unarchive and ownership changes shall fence delayed
  destructive work; filesystem failures shall remain retryable across restart.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.4:** When a checkout gains local
  changes during archive cleanup, those changes shall not veto removal after
  the other cleanup checks succeed. Archive shall neither create an automatic
  commit nor push local work before deleting it. Existing branch-compaction
  and committed-head recovery behavior shall continue to apply.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.5:** Existing archived worktrees
  retained under the previous dirtiness policy shall be reconsidered by the
  normal cleanup worker after upgrade, without waiting for the old 24-hour
  cleanliness delay or enabling storage maintenance. Reconsideration shall
  preserve the original archive identity, cancellation, and ownership fences;
  unarchived or reassigned work shall not be deleted.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.6:** When archive confirmation is
  enabled, desktop, compact inline, bulk, and phone confirmations shall state
  that managed worktrees and local changes are deleted. The existing
  confirmation preference, cascade choices, and programmatic API shall retain
  their interaction contracts. No extra discard confirmation shall be added.
- **AC-TASKS-DIRTY-WORKTREE-ARCHIVE-004.7:** Archive shall not use this policy
  to delete an ordinary local repository folder, another task's resources, a
  borrowed workspace, or a path without proven ownership. Task deletion shall
  retain its separate dirty-worktree consent behavior.

## Exclusions

Task-delete discard consent, session deletion, ordinary local repository
folders, remote executor policies, independent-checkout adoption consent, and
unrelated orphan-directory removal are outside this change.

## System design

[Dirty worktree task archive](../system-design/dirty-worktree-archive.md).

## Implementation plans

- [Preserve dirty worktrees on archive](../../../plans/dirty-worktree-archive/plan.md)
- [Reclaim retained archived worktrees](../../../plans/archived-worktree-reclamation/plan.md)
- [Remove archived task directories](../../../plans/archive-task-directory-cleanup/plan.md)
- [Immediate archive removal](../../../plans/immediate-archive-worktree-removal/plan.md)
