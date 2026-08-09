# ADR-2026-08-08-task-owned-worktree-lifetime: Keep Worktree Ownership at the Task Lifecycle

**Status:** accepted
**Date:** 2026-08-08
**Area:** backend, frontend

## Context

A `TaskEnvironment` is task-owned and may be reused by multiple sessions or
borrowed by another task. The existing `task_session_worktrees` row combines a
session reference with the only durable physical worktree pointer and cascades
when the session is deleted. PR #2421 attempted to compensate by snapshotting
that row and reclaiming the worktree when no session references remained, but
zero session references does not mean the task has relinquished its workspace.

## Decision

Use the existing task-environment model as the only workspace ownership graph.
`task_environments.task_id` owns the environment, and one
`task_environment_repos` row owns each materialized repository worktree. A
session refers to the workspace only through
`task_sessions.task_environment_id`; no session-to-worktree table is needed.

Normalize the schema in one validated upgrade. Backfill
`task_environment_repos` from its existing rows, the deprecated flat worktree
columns on `task_environments`, and `task_session_worktrees`. Create a normalized
environment for a legacy worktree group only when no valid environment exists,
and update the affected sessions to reference it. After validation proves that
every persisted worktree has exactly one task-environment owner, drop
`task_session_worktrees` and the deprecated `task_environments.repository_id`,
`worktree_id`, `worktree_path`, and `worktree_branch` columns. Fresh databases
are created directly in the final shape. Runtime code has no dual-read or
dual-write compatibility path; only the versioned migration knows the legacy
schema.

Before the durable cleanup worker starts, the upgrade also removes any
preview-build `task_resource_cleanup_jobs` rows whose trigger is
`session_delete`. That trigger is invalid under the ownership model and its code
identity is removed; task-level cleanup jobs and their history remain intact.

The cutover uses a dedicated, error-returning migration rather than
`db.MigrateLogger.Apply`, whose legacy contract logs and swallows unexpected
errors. The migration acquires the database writer/migration lock and performs
the following in one transaction:

1. Read and normalize all legacy ownership rows into shadow tables.
2. Compare the complete legacy and normalized worktree identity/path/branch sets.
3. Verify that every session that previously referenced a worktree resolves to
   one valid environment and that every physical worktree has one canonical
   environment-repository owner.
4. Verify uniqueness, foreign keys, row counts, and the exact final schema.
5. Only then drop/rename the legacy tables and columns and commit.

Any query, copy, validation, DDL, or commit error rolls the transaction back.
SQLite uses the existing fatal pre-upgrade `VACUUM INTO` snapshot in addition to
transaction rollback. PostgreSQL uses transactional DDL under a migration
advisory lock and takes exclusive locks on every affected ownership table; lock
timeout aborts without mutation. PostgreSQL operators retain their existing
external backup policy and must stop mixed-version writers during the upgrade.
The migration never calls filesystem or Git cleanup, and the cleanup worker does
not start until repository initialization succeeds.

`task_environment_repos` remains because it models a real domain concept: the
ordered repositories in an environment, including preparation failures where no
worktree exists. Its worktree identity, path, branch, status, and lifecycle
timestamps become the single source of physical-worktree truth. Environment API
responses expose the repository collection instead of deprecated flat worktree
fields.

Only task lifecycle operations may remove physical workspace resources. Archive,
delete, cascade, workspace delete, quick-chat expiry, and explicit environment
reset use the existing durable `task_resource_cleanup_jobs` worker. Task cleanup
inventories `task_environment_repos` by joining through
`task_environments.task_id`, snapshots handles before destructive task-row
mutation, preserves shared environments, and retries filesystem/Git failures
after restart.

Reserve the prepared task cleanup job before taking its inventory. Session and
worktree creation serialize on the task row and refuse creation while that
prepared lifecycle barrier is active. The barrier transaction commits before
the cleanup worker takes repository or filesystem locks, preserving the existing
Git lock order and avoiding a database/Git lock inversion.

## Consequences

- A task can have zero sessions without losing its workspace, Git registration,
  branch, or uncommitted files.
- Session deletion becomes a pure conversation lifecycle operation;
  no `session_delete` cleanup trigger or `ReclaimSessionWorktree` path exists.
- SQLite and PostgreSQL need transactional, replayable normalization migrations
  that either reach the final schema completely or leave the old database
  untouched.
- Invalid or ambiguous legacy data causes a diagnostic startup refusal. It does
  not trigger a partial repair or destructive best-effort conversion.
- The final schema is intentionally incompatible with older binaries. SQLite
  downgrade requires restoring the automatic pre-upgrade snapshot; PostgreSQL
  downgrade requires restoring the operator's pre-upgrade backup. There is no
  rolling mixed-version deployment window.
- `task_session_worktrees`, its CRUD/model surface, duplicate worktree fields,
  and runtime compatibility branches are removed after backfill.
- Worktree-store reads, session projections, storage inventory, and task cleanup
  all use the task environment and its repository rows.
- Shared-environment ownership transfer moves the environment's `task_id`; its
  worktrees follow automatically without a second ownership update.
- Creation paths must compensate a physical worktree if the canonical owner and
  environment-repository transaction is rejected by a concurrent task lifecycle
  barrier.

## Alternatives Considered

1. **Reclaim when the final session reference disappears.** Rejected because
   reference count is not ownership and a task intentionally remains reusable
   with zero sessions.
2. **Keep only a session-delete cleanup snapshot.** Rejected because it preserves
   a pointer solely to destroy the resource and still cannot represent the task's
   continuing ownership.
3. **Add `task_worktrees` and retain a session reference table.** Rejected because
   it creates a third representation of worktree state and leaves a permanent
   compatibility model. The materialization and branch-add paths can instead be
   reordered to persist the existing task environment owner and compensate on
   failure.
4. **Keep `task_session_worktrees` as a reduced reference table.** Rejected
   because `task_sessions.task_environment_id` already identifies the complete
   shared workspace. A second association is redundant and can drift.
5. **Rely on an in-process task mutex.** Rejected because it does not protect
   PostgreSQL multi-connection races or backend restart boundaries.
6. **Ship an additive release and drop legacy schema later.** Rejected because it
   creates a dual-write period, permits drift, and leaves users indefinitely on
   different ownership models.
7. **Use best-effort migration statements.** Rejected because swallowed errors
   can commit a partially normalized database. This cutover must return every
   unexpected error and roll back as a unit.
8. **Keep downgrade-compatible legacy tables.** Rejected because those tables
   recreate the dual ownership model. Recovery uses the pre-upgrade backup
   instead of permanent compatibility storage.
