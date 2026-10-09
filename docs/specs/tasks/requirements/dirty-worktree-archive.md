---
status: draft
system: tasks
created: 2026-09-21
updated: 2026-09-24
owners:
  - cfl
---
# Dirty Worktree Task Archive Requirements

## Overview

Archiving a task is routine board maintenance. It removes the card from active
views and releases runtime resources, and it uses the existing branch-preserving
cleanup path so committed work remains recoverable. A dirty checkout stays on
disk with its branch; a clean checkout follows the manager's existing cleanup
policy.

Uncommitted work has no such protection. Task deletion already refuses to
reclaim a checkout holding tracked or untracked changes, and
[dirty worktree deletion](../system-design/dirty-worktree-deletion.md) states
that it does not change archive. This document owns the archive side of that
boundary.

These requirements extend
[task runtime cleanup](runtime-cleanup.md), which owns archive ownership,
durability, and retry. They add no refusal, consent flag, or error shape to
archive.

## Requirements

### REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-001: Archive Preserves Uncommitted Work

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

### REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-002: Eventual Reclamation of Retained Worktrees

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

## System design

[Dirty worktree task archive](../system-design/dirty-worktree-archive.md).

## Implementation plans

- [Preserve dirty worktrees on archive](../../../plans/dirty-worktree-archive/plan.md)
- [Reclaim retained archived worktrees](../../../plans/archived-worktree-reclamation/plan.md)
