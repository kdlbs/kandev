---
status: draft
created: 2026-08-08
owner: cfl
---

# Session Delete Preserves Task Workspaces

Decision: [Task-owned worktree lifetime](../../decisions/2026-08-08-task-owned-worktree-lifetime.md)

Runtime contract: [Task runtime cleanup](../tasks/runtime-cleanup.md)

## Why

A session is a conversation and execution reference inside a task; it is not the
owner of the task's materialized workspace. A task may have multiple sessions
sharing one `TaskEnvironment`, may intentionally have zero sessions, and may
create a future session that reuses its existing files and Git state.

PR #2421 proposed reclaiming a worktree when the final active session reference
disappeared. That rule confuses reference count with ownership. Deleting the last
session would destroy uncommitted work and remove a workspace that the still-live
task owns. The persistence model must instead keep task ownership durable until a
task lifecycle operation takes responsibility for cleanup.

## What

- Deleting a session removes its session row and conversation history. Its
  `task_environment_id` reference disappears with the row; the task-owned
  environment is unchanged.
- Deleting a session never removes a physical directory, runs `git worktree
  remove` or `git worktree prune`, or deletes a branch.
- The rule applies when the deleted session is the task's only session and when
  it is one of several sessions sharing the workspace.
- A task with zero sessions retains its `TaskEnvironment`, worktree directory,
  Git registration, branch, and uncommitted files.
- A later session may reuse the retained workspace.
- Task archive/delete, cascade archive/delete, workspace delete, quick-chat task
  expiry, and explicit task-environment reset remain the owners of physical
  cleanup.
- Task lifecycle cleanup discovers task-owned resources without joining through a
  session row and executes asynchronously through the durable cleanup worker.
- Resources borrowed by another task or referenced through a shared environment
  are preserved or transferred according to existing ownership rules.
- Session deletion does not create or activate a `session_delete` cleanup job and
  has no `ReclaimSessionWorktree` physical cleanup path.
- Session-delete confirmations describe the conversation deletion and explicitly
  say that the task workspace and files remain. They do not warn that
  uncommitted/unpushed workspace state will be removed.

## Data Model

### `task_environments`

`task_environments` is the task-owned workspace root:

```text
id                    string       environment identity
task_id               string       owning task and inventory key
executor fields       ...          environment runtime configuration
workspace_path        string       agent-visible workspace root
container/sandbox     ...          non-worktree resource handles
status/timestamps     ...          environment lifecycle
```

The deprecated flat `repository_id`, `worktree_id`, `worktree_path`, and
`worktree_branch` columns are removed. Repository and physical worktree state
lives only in the environment's repository rows.

### `task_environment_repos`

`task_environment_repos` is the canonical physical-worktree table and also
represents repository preparation failures where no worktree was created:

```text
id                    string       environment-repository identity
task_environment_id   string       owning environment
repository_id         string       source repository
branch_slug           string       task repository/branch slot
worktree_id           string|null  physical worktree identity
worktree_path         string|null  absolute materialized path
worktree_branch       string|null  registered Git branch
position              integer      workspace ordering
error_message         string       preparation failure, when any
status                enum         active | merged | deleted
created_at             timestamp
updated_at             timestamp
merged_at/deleted_at   timestamp|null
```

Task-level inventory queries these rows through
`task_environments.task_id`. Session projections query them through
`task_sessions.task_environment_id`. Neither query depends on a
session-to-worktree association.

The canonical environment and repository rows are persisted after physical
materialization and before the workspace is exposed for reuse. A worktree that
was physically created but cannot be persisted because task cleanup won a race
is compensated before it becomes task state.

### Schema normalization

SQLite and PostgreSQL migrations backfill canonical rows from
`task_environment_repos`, legacy flat `task_environments` worktree fields, and
`task_session_worktrees`. Sessions with a valid `task_environment_id` retain it.
Legacy sessions are assigned to the matching existing environment or to a
normalized task-owned environment created for their connected worktree group.
Canonical `task_environment_repos` rows take precedence when the legacy flat
environment fields or session references repeat the same physical worktree.
Legacy session rows marked `deleted` (or carrying `deleted_at`) and stale
references from terminal sessions are historical evidence, not additional
owners, and bypass validation only when a canonical repository owner exists.
A terminal-only reference without a canonical owner, a non-terminal session,
or a worktree that has no canonical owner still requires compatible identity,
path, branch, and repository data; unresolved ownership fails closed with a
diagnostic.

After backfill validation, the same upgrade drops `task_session_worktrees` and
the deprecated flat worktree columns from `task_environments`. It also removes
the matching Go models, repository methods, duplicate schema initializer, API
fields, and dual-read/dual-write code. Fresh databases contain only the final
schema. There is no indefinite compatibility phase.

If a database was opened by a preview build of PR #2421, the migration removes
its `session_delete` cleanup jobs before the cleanup worker can claim them. The
trigger constant and session-specific cleanup implementation are removed from
the codebase. Task-lifecycle cleanup jobs remain unchanged.

The migration runs under an exclusive writer/migration lock in one transaction.
It builds normalized shadow tables, compares the full worktree identity, path,
and branch inventory before and after, validates all ownership and foreign-key
constraints, and swaps schemas only after every check passes. It returns errors
directly; it does not use the best-effort migration logger.

SQLite takes the repository's existing fatal pre-upgrade snapshot before the
transaction. PostgreSQL uses transactional DDL under an advisory migration lock
and exclusive locks on the affected tables; lock timeout aborts the upgrade.
Mixed-version PostgreSQL writers must be stopped during cutover. The migration
performs no filesystem or Git operation, and no resource cleanup worker starts
until schema initialization commits successfully.

### `task_resource_cleanup_jobs`

The existing durable job is reserved in `prepared` state before task lifecycle
inventory is captured. Its snapshot contains canonical worktree handles before
archive/delete cascades can remove database rows. The job remains independent of
the task/session foreign-key lifetime and retains the existing retry schedule.

There is no session-delete trigger value.

## API Surface

The WebSocket action is unchanged:

```text
session.delete request  { "session_id": string }
session.delete response { "success": true }
```

Success means that the session was deleted. It does not mean that task resources
were reclaimed. Existing validation, permission, running-
session refusal, quiescence, and primary-session promotion behavior remains.

No new HTTP or WebSocket action is required for task cleanup.

## State and Concurrency

Session deletion follows:

```text
stopped session -> quiesced runtime -> session/reference deletion -> task retained
```

Task lifecycle cleanup follows:

```text
active task -> prepared cleanup barrier -> complete inventory snapshot
            -> archive/delete mutation -> pending durable worker
            -> running -> succeeded | retry_wait | failed
```

Session creation and canonical environment-repository persistence serialize with
the owning task row and check for an active prepared task cleanup barrier.
PostgreSQL uses row locking and SQLite uses the serialized writer transaction.
Either creation commits before the barrier and is included in inventory, or the
barrier commits first and creation is rejected/compensated.

The database barrier transaction completes before any repository, target-path,
filesystem, or Git lock is acquired. Existing shared-worktree reference checks
run again during destructive cleanup so an ownership transfer or active borrower
cannot be destroyed by a stale decision.

## Permissions

Session deletion keeps its existing task/session authorization requirements.
This feature does not grant a session deleter permission to archive/delete the
task or remove task resources.

Task archive/delete and cascade operations keep their existing permissions and
remain the authorization boundary for physical cleanup.

## Failure Modes

- If session deletion fails before commit, no filesystem or Git cleanup was
  attempted and canonical task ownership is unchanged.
- If session deletion commits and the backend crashes, the canonical owner row
  remains queryable by `task_id`; no recovery job is needed for session deletion.
- If migration cannot determine a single compatible owner/path, startup fails
  closed, the transaction rolls back, and the pre-upgrade database remains
  authoritative instead of authorizing future deletion from ambiguous data.
- Historical deleted session-worktree rows and stale references from terminal
  sessions do not block migration when a canonical task-owned repository row
  exists; the canonical row remains authoritative.
- If migration fails after shadow tables are populated or after legacy DDL has
  begun, the database transaction restores the complete legacy schema and data.
- If SQLite cannot create its pre-upgrade snapshot, startup stops before the
  migration transaction begins.
- If PostgreSQL cannot acquire its migration/table locks, startup stops without
  changing the schema or data.
- An older binary cannot open the normalized schema. Downgrade restores the
  SQLite automatic snapshot or the PostgreSQL operator backup.
- If a new session/worktree races task cleanup, the prepared barrier decides the
  ordering; cleanup cannot snapshot a partial inventory.
- If physical compensation is required, it removes only the resource created by
  the rejected materialization attempt.
- If archive/delete filesystem or Git cleanup fails, the durable task cleanup job
  retries after failure and backend restart using its persisted snapshot.
- If another task/session still borrows the environment, cleanup preserves or
  transfers the resource rather than deleting it.

## Persistence Guarantees

- Task worktree ownership in `task_environment_repos` survives deletion of any
  or all session rows.
- Zero-session workspaces remain protected from storage inventory and orphan GC.
- Backend restart does not change the ownership boundary.
- A failed upgrade leaves no partially normalized schema. SQLite also retains a
  directly restorable pre-upgrade snapshot.
- Archive/delete cleanup can query canonical resources by `task_id` before
  mutation and can continue from the durable snapshot after task-row deletion.
- Historical branch metadata required for unarchive recovery remains preserved
  according to the task runtime cleanup contract.
- No session-delete path invokes filesystem removal, Git worktree removal/prune,
  or branch deletion.

## Scenarios

- **GIVEN** a task with one stopped session, a registered worktree, and an
  uncommitted marker, **WHEN** the session is deleted, **THEN** the task has zero
  sessions and the directory, registration, branch, and marker are unchanged.
- **GIVEN** that zero-session task, **WHEN** a replacement session starts,
  **THEN** it reuses the task workspace and observes the marker.
- **GIVEN** two sessions sharing a task workspace, **WHEN** one is deleted,
  **THEN** the other continues using the same directory without interruption.
- **GIVEN** the final session was deleted and the backend restarted, **WHEN** the
  task is archived, **THEN** the task becomes archived and its canonical
  worktree is scheduled for asynchronous cleanup unless shared.
- **GIVEN** the final session was deleted and the backend restarted, **WHEN** the
  task is deleted, **THEN** the task row is removed and its worktree/Git
  registration are removed by a retryable durable cleanup job.
- **GIVEN** a transient Git/filesystem failure during task cleanup, **WHEN** the
  backend restarts and the retry becomes due, **THEN** cleanup resumes from the
  snapshot and completes.
- **GIVEN** session/worktree creation and task deletion begin concurrently,
  **WHEN** one reserves the task row first, **THEN** the resource is either fully
  included in cleanup or rejected and compensated; it cannot become untracked.
- **GIVEN** a legacy database contains deleted session-worktree history and a
  canonical task-owned repository row for the same workspace, **WHEN** the new
  binary starts, **THEN** the cutover completes, preserves the canonical row,
  and removes the legacy schema without requiring manual database edits.
- **GIVEN** a legacy flat environment path differs from the canonical repository
  row for the same physical worktree, **WHEN** the new binary starts, **THEN**
  the canonical repository path and branch are retained and the cutover
  completes.
- **GIVEN** a non-terminal session references a worktree that conflicts with
  the canonical owner and cannot be reconciled, **WHEN** the new binary starts,
  **THEN** startup fails closed, the transaction rolls back, and the verified
  pre-upgrade backup remains the recovery source.

## Out of Scope

- Reclaiming workspaces merely because their session reference count reaches
  zero.
- A session-delete cleanup job, retry state, or physical reclaimer.
- Per-session uncommitted/unpushed warning fetches.
- New archive/delete UI controls or cleanup-failure UI.
- Changing Quick Chat closure, which deletes its hidden backing task and therefore
  remains a task-lifecycle cleanup.
