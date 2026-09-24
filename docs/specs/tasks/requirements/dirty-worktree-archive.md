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

## System design

[Dirty worktree task archive](../system-design/dirty-worktree-archive.md).
