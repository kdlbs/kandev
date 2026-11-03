---
status: active
system: tasks
created: 2026-10-30
updated: 2026-10-30
owners:
  - cfl
---
# Shared Worktree Holds Requirements

## Overview

A task can run in a workspace owned by another task. The borrower must protect
that worktree while it can still use it, and must not pin it after it is finished
and archived. This refines "active session" in AC-TASKS-RUNTIME-CLEANUP-001.3.

## Requirements

### REQ-TASKS-SHARED-WORKTREE-HOLDS-001: Archived Borrowers Release Shared Worktrees

**Intent:** Reclaim an owner's worktree once no other task can still be using it,
without removing borrower history.

#### Acceptance criteria

- **AC-TASKS-SHARED-WORKTREE-HOLDS-001.1:** A shared worktree shall stay protected
  from destructive cleanup while at least one session of another task holds it,
  even when other borrower sessions have released it.
- **AC-TASKS-SHARED-WORKTREE-HOLDS-001.2:** A session of another task shall stop
  holding a worktree only when all of these are true: its task is archived, its
  state is `COMPLETED`, `FAILED`, or `CANCELLED`, and no runtime row remains for
  it. If any condition is false or cannot be determined, the session holds it.
- **AC-TASKS-SHARED-WORKTREE-HOLDS-001.3:** When the last hold is released, the
  owner's lifecycle cleanup shall reclaim the worktree, including one pinned before
  this rule shipped. The borrower's session and messages shall remain.

## Out of scope

- Changing when a borrower with an active session takes ownership of an environment.
- Remote task directories, containers, and sandboxes.
