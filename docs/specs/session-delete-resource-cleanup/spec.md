---
status: draft
created: 2026-08-08
owner: cfl
---

# Session Delete Preserves Task Workspaces

Decision: [Task-owned worktree lifetime](../../decisions/2026-08-08-task-owned-worktree-lifetime.md)

Runtime contract: [Task runtime cleanup](../tasks/system-design/runtime-cleanup.md)

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
- A later session may reuse the retained workspace only after canonical
  environment and repository inventory validation. A missing, preparing, or
  unsafe retained environment returns a recoverable workspace conflict; a
  replacement session never recreates it implicitly.
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
When no canonical repository row exists, the surviving task environment's flat
worktree fields take precedence over a legacy session reference to the same
physical `worktree_id`. This source precedence applies regardless of session
lifecycle state: path or branch drift on a lower-precedence row does not create
a second owner when the physical identity is unchanged.
A legacy session's worktree is normalized onto the environment the session
resolves to, which is not always an environment owned by the session's own
task: workspace-group members, subtasks inheriting a parent workspace, and
tasks that received an environment through ownership handoff share one
environment across tasks. Such a borrowing task gains no environment of its
own, and the shared physical worktree keeps exactly one owner.
Legacy session rows marked `deleted` (or carrying `deleted_at`) and stale
references from terminal sessions are historical evidence, not additional
owners. A higher-precedence task-owned source for the same repository and
branch slot, either a canonical repository row or the surviving flat
environment row, remains the owner even when a terminal reference carries a
different physical `worktree_id`.
Legacy session-worktree rows can also outlive their referenced session in
databases written by older releases. An orphaned row is historical evidence
when it is marked deleted or when its exact non-empty physical `worktree_id` is
already represented by a non-deleted normalized target built from a canonical
repository row or surviving flat environment. The higher-precedence task-owned
source remains authoritative and the orphan does not prevent cutover. A raw
flat row absorbed into a deleted canonical target is not an active owner. An
active orphan with no matching non-deleted normalized target remains
irreconcilable because the cutover cannot recover its task or environment
owner, so startup fails closed.

A non-deleted canonical repository row also takes precedence over deprecated
flat fields for the same normalized task-owned repository slot: the repository
and empty branch slot within the surviving task environment. This rule applies
when the two sources contain different physical `worktree_id` values, including
when the canonical row was re-homed from a collapsed legacy environment. The
original environment ID is not an ownership condition after re-homing. The
upgrade records the flat worktree as historical evidence and logs its identity,
path, and branch after the transaction commits. The upgrade does not remove its
directory, Git registration, or branch.

The task-owned model holds exactly one physical worktree per (repository,
branch slug) slot, while the legacy per-session model let one task accumulate
several for the same slot through handoffs, re-materialized workspaces, and
additional sessions. When no higher-precedence task-owned source exists, the
upgrade elects one owner per contested slot instead of failing: a live
session's worktree wins over a terminal session's, then the most recently
updated row wins. Every other worktree for that slot becomes historical
evidence and is logged with its identity, repository, path, and branch. No
directory, registration, or branch is removed, so a demoted workspace remains
on disk.

Irreconcilable data, including incompatible path or branch metadata for the
same physical worktree identity with no higher-precedence owner or a worktree
whose task has no resolvable environment, still fails closed with a diagnostic.

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
  sessions do not block migration when a higher-precedence task-owned source
  exists for the same repository and branch slot. The higher-precedence source
  remains authoritative.
- An orphaned legacy session-worktree row does not block migration when it is
  deleted history or its exact non-empty physical `worktree_id` is already
  represented by a non-deleted normalized target built from a canonical
  repository row or the surviving flat environment. The orphan contributes no
  ownership or metadata.
- An active orphaned session-worktree row with no higher-precedence source for
  the same physical `worktree_id` remains irreconcilable and fails closed.
- A raw surviving flat row does not suppress an active orphan when source
  merging leaves that physical identity only on a deleted normalized target.
  Startup fails closed rather than dropping the only active legacy evidence.
- A legacy session row that repeats a higher-precedence physical `worktree_id`
  cannot block migration solely because its path or branch metadata is stale,
  including when the session is resumable. The higher-precedence repository or
  flat environment metadata remains authoritative.
- Deprecated flat fields cannot block migration when a non-deleted canonical
  row owns the same normalized task, repository, and empty branch slot. The
  canonical row remains authoritative even when it was re-homed from a
  collapsed environment, and the upgrade records the flat worktree as
  historical evidence.
- A session bound to another task's environment does not block migration and
  does not create a second owner for the shared worktree.
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
- **GIVEN** a non-deleted canonical repository row and deprecated flat fields
  contain different worktree IDs for the same empty branch slot, **WHEN** the
  new binary starts, **THEN** the canonical row remains the owner, the flat
  worktree is logged as history, and the cutover completes.
- **GIVEN** a surviving flat environment and a canonical row only in a legacy
  environment that collapses into it, **WHEN** the new binary starts, **THEN**
  the re-homed canonical row remains the owner of the normalized slot and the
  flat worktree is logged as history.
- **GIVEN** a resumable legacy session and a canonical task-owned repository row
  carry the same `worktree_id` but different path or branch metadata, **WHEN**
  the new binary starts, **THEN** the canonical metadata is retained and the
  cutover completes without changing the session lifecycle state.
- **GIVEN** no canonical repository row exists and a legacy session reference
  repeats the surviving task environment's `worktree_id` with stale path or
  branch metadata, **WHEN** the new binary starts, **THEN** the flat environment
  metadata is retained and the cutover completes.
- **GIVEN** a terminal legacy session references an older physical worktree for
  the surviving flat environment's repository slot, **WHEN** the new binary
  starts, **THEN** the flat environment remains the owner and the cutover
  completes.
- **GIVEN** a legacy session-worktree row references a missing session and the
  same non-empty `worktree_id` remains on a non-deleted normalized target built
  from a surviving flat environment or canonical repository row, **WHEN** the
  new binary starts, **THEN** the task-owned source remains authoritative, the
  orphan contributes no metadata, and the cutover completes without manual
  database edits.
- **GIVEN** a deleted legacy session-worktree row references a missing session,
  **WHEN** the new binary starts, **THEN** the row is treated as historical
  evidence and does not block the cutover.
- **GIVEN** an active legacy session-worktree row references a missing session
  and no higher-precedence source carries its physical `worktree_id`, **WHEN**
  the new binary starts, **THEN** startup fails closed and the complete legacy
  schema and data remain unchanged.
- **GIVEN** a surviving flat row, a deleted canonical row, and an active orphan
  share one physical `worktree_id`, but source merging leaves only a deleted
  normalized target, **WHEN** the new binary starts, **THEN** startup fails
  closed and preserves the legacy schema rather than dropping the active
  orphan evidence.
- **GIVEN** `task_environments` already has the normalized schema but a stale
  `task_session_worktrees` table remains from an intermediate build, **WHEN**
  the new binary starts, **THEN** the cutover preserves normalized environment
  and repository data, imports any remaining session-worktree inventory, and
  removes the stale legacy table without requiring manual database edits.
- **GIVEN** a legacy session of one task is bound to another task's environment
  and carries a session-worktree row for that shared workspace, **WHEN** the new
  binary starts, **THEN** the worktree is normalized onto the owning task's
  environment, both sessions keep that environment, the borrowing task gains no
  environment of its own, and the cutover completes.
- **GIVEN** several legacy sessions of one task registered different physical
  worktrees for the same repository and branch slot, **WHEN** the new binary
  starts, **THEN** the canonical owner — or the newest live session worktree
  when there is none — keeps the slot, the other worktrees are recorded as
  history and logged with their paths, no directory or branch is removed, and
  the cutover completes.
- **GIVEN** a non-terminal session references a worktree whose path or branch
  metadata contradicts the same physical worktree's owner and cannot be
  reconciled, **WHEN** the new binary starts, **THEN** startup fails closed, the
  transaction rolls back, and the verified pre-upgrade backup remains the
  recovery source.

## Out of Scope

- Reclaiming workspaces merely because their session reference count reaches
  zero.
- A session-delete cleanup job, retry state, or physical reclaimer.
- Per-session uncommitted/unpushed warning fetches.
- New archive/delete UI controls or cleanup-failure UI.
- Changing Quick Chat closure, which deletes its hidden backing task and therefore
  remains a task-lifecycle cleanup.
