---
status: current
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-SHARED-WORKTREE-HOLDS-001
created: 2026-10-30
updated: 2026-10-30
owners:
  - cfl
---

# Shared Worktree Holds System Design

## Purpose and boundaries

This design extends [Task Runtime Cleanup](runtime-cleanup.md) and
[Managed Worktree Task Archive](dirty-worktree-archive.md). It defines when a
session of another task stops protecting a physical worktree. Ownership transfer
to a borrower with an active session is unchanged.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| AC-TASKS-SHARED-WORKTREE-HOLDS-001.1, .2 | Hold predicate |
| AC-TASKS-SHARED-WORKTREE-HOLDS-001.3 | Release and reclaim |

## Hold predicate

`SQLiteStore.CountActiveWorktreeReferences` is the single predicate for archive
cleanup, archive reclaim, and `removeWorktreeWithReceipt`. It counts sessions of
other tasks that reference the owning environment. A session is not counted only
when its task is archived, its state is `COMPLETED`, `FAILED`, or `CANCELLED`, and
it has no `executors_running` row.

Every other session counts, including a terminal session of a live task, an
archived `IDLE` session, and a session whose task row cannot be joined. The runtime
check matches reclaim's rule that any retained runtime defers removal, and covers
a borrower whose stop failed.

## Release and reclaim

The hold is derived at read time. Archive does not rewrite
`task_sessions.task_environment_id`, so a session keeps its environment history
and unarchive needs no reference restoration. The single shared
`task_environment_repos` row is never released on behalf of one borrower.

Removal keeps the owner's identity, path, Git registration, and branch checks.
Uncommitted files in the shared checkout are removed under the
[immediate archive decision](../../../decisions/2026-10-17-immediate-archive-worktree-removal.md).
When the borrower is archived after the owner, the owner's deferred reclaim job
observes the release on its next 24-hour recheck or after a backend restart.
Worktrees pinned by earlier versions are reclaimed the same way, with no migration.

A borrower unarchived while its owner stays archived still cannot launch:
`validateInheritedEnvironmentOwner` refuses that environment whether or not the
checkout exists. Unarchiving the owner restores the checkout through normal
preparation, and the borrower then attaches to it.

## Scenarios

- **GIVEN** an archived owner whose only borrower session is terminal, has no
  runtime row, and belongs to an archived task, **WHEN** archive cleanup or
  reclaim runs, **THEN** the worktree is removed and the session and its messages
  remain.
- **GIVEN** an archived owner with one released borrower and one borrower of a
  live task, **WHEN** reclaim runs, **THEN** the worktree is retained and the job
  defers as in use.

## Implementation plan

[Archived borrower worktree release](../../../plans/archived-borrower-worktree-release/plan.md).
