# ADR-2026-08-30-compact-integrated-managed-branches: Compact Fully Integrated Managed Branches

**Status:** accepted
**Date:** 2026-08-30
**Area:** backend, operations

## Context

Task lifecycle cleanup deliberately removed Git worktree registrations while
retaining every local branch ref. That protected unpublished commits and made
archive recovery possible, but no bounded automatic consumer reclaimed managed
branches after their commits were fully integrated. The result was permanent
growth of redundant local refs.

A branch name or task-shaped suffix is not proof of ownership. A branch can be
user-selected, shared with another environment, checked out in another Git
worktree, or contain commits absent from the intended base. Archive recovery also
cannot depend on a remote branch: unpublished state must restore exactly, and a
safely removed integrated branch must remain locally recoverable.

## Decision

Capture three internal fields on each `task_environment_repos` worktree row:

- branch owner: `kandev`, `external`, or `unknown`;
- intended integration ref, resolved from the task branch policy target before
  falling back to the task repository base branch;
- exact recovery head SHA, initially empty.

Legacy and incomplete rows default to `unknown` and empty refs. They are retained
without inference.

At terminal worktree cleanup, after removing the worktree registration and while
holding the existing repository lock, consider exactly one explicit local branch.
Delete it only when all of these conditions hold:

1. The persisted owner is `kandev` and exactly one durable row claims the
   repository/branch pair.
2. No live Git worktree registration uses `refs/heads/<branch>`.
3. The branch is not its base or integration ref and both exact commit refs
   resolve locally without fetching.
4. `git merge-base --is-ancestor <branch-head> <integration-head>` succeeds.
5. The exact branch head is persisted with compare-and-set and is unchanged when
   re-read immediately before deletion.
6. `git branch -d -- <branch>` accepts the single explicit ref.

Any missing metadata, ambiguous owner, active/shared reference, protected ref,
failed probe, unique commit, compare-and-set loss, head race, or refused
non-force deletion retains the branch. Remote refs are never deletion targets.
Permanent task deletion keeps its separate explicit force-delete disposition.

Unarchive and recreate use a retained local branch directly. When safe
compaction removed the local ref, they recreate it from the persisted exact head
before attempting remote or pull-request recovery. Because compaction requires
the head to be reachable from the integration ref, the recovery commit remains
reachable after the branch ref is removed.

All terminal paths share the manager policy: single-repository cleanup,
multi-repository cleanup, task archive, task-environment reset, handoff cleanup,
and aged automation cleanup. The task cleanup service delegates worktree removal
to its batch cleaner exactly once. Aggregate attempted/deleted/retained receipts,
fixed retained-reason counts, and matching expvar counters provide bounded
observability without branch lists or repository data.

## Consequences

- Fully integrated Kandev-created local branches no longer accumulate without
  bound after terminal cleanup.
- Unpublished, external, legacy, protected, shared, inherited, and ambiguous
  branches remain fail-closed.
- Archive/unarchive preserves exact state whether the branch ref was retained or
  safely compacted.
- New additive columns must survive SQLite and PostgreSQL ownership migrations
  and environment-repository CRUD.
- Non-force deletion may conservatively retain a branch even after ancestry was
  independently proven. Safety takes precedence over reclamation rate.

## Alternatives Considered

1. **Set every cleanup call to `removeBranch=true`.** Rejected because the
   existing force deletion discards unpublished work and makes some archived
   branches unrecoverable.
2. **Delete branches matching the generated naming pattern.** Rejected because
   names do not prove ownership, ancestry, liveness, or task-environment identity.
3. **Run an install-wide periodic branch glob collector.** Rejected because it
   loses the lifecycle metadata and repository lock already available at terminal
   cleanup, broadens the deletion surface, and creates duplicate attempts.
4. **Keep every local branch indefinitely.** Rejected because redundant managed
   refs accumulate permanently after their commits are integrated.
