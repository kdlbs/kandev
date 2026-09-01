---
status: current
system: tasks
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
created: 2026-08-30
updated: 2026-08-31
owners:
  - cfl
---

# Environment-Owned Git Status System Design

## Purpose and boundaries

A `TaskEnvironment` owns one current workspace. The environment also owns the
current Git status that workspace views show.

A `TaskSession` is capture provenance and a transport route. It is not a scope
for current Git status.

This design covers snapshot persistence, selection, capture, hydration,
WebSocket delivery, task summaries, and frontend storage. It does not change
Git polling inside agentctl or workspace materialization.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5` | [Persistence ownership](#persistence-ownership), [Delivery identity](#delivery-identity), [Workspace views](#workspace-views) |
| `AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9` | [Persistence ownership](#persistence-ownership), [Source precedence](#source-precedence), [Workspace views](#workspace-views) |

## Persistence ownership

`task_session_git_snapshots` keeps its table name for compatibility with
historical analytics and delivery queries. Its scoping key changes to
`task_environment_id`.

The final table has these ownership columns:

- `task_environment_id TEXT NOT NULL` references `task_environments(id)` with
  `ON DELETE CASCADE`.
- `session_id TEXT` is nullable capture provenance. It references
  `task_sessions(id)` with `ON DELETE SET NULL`.

Session deletion cannot remove the current environment status. Environment
deletion removes all snapshots for that environment.

Repository identity remains in snapshot metadata. A missing or empty
`repository_name` identifies the root repository.

Current-status reads partition rows by `task_environment_id` and normalized
repository name. Historical session reads can still filter by the nullable
`session_id` provenance field.

## Authoritative selection

The repository selects one current observation for each environment and
repository pair. It orders observations by `created_at DESC`.

Snapshot type does not outrank observation time. A newer `live_monitor` row
therefore outranks an older `agent_completed` row from any sibling session.

The newest observation defines the current state. For equal timestamps, the
selector prefers file details and then orders by `id DESC`.

The selector never searches before the newest observation for file details.
A sparse live row therefore cannot reuse stale files from an older detailed
row.

Archive rows use the same time rule. An archive row remains current until a
newer non-archive observation exists after a resume.

## Capture and supersession

Each write resolves the capture session to a non-empty task environment before
the repository changes data. A missing environment causes the write to stop.

`UpsertLatestLiveGitSnapshot` keeps one `live_monitor` row for each environment
and repository pair. SQLite uses its serialized writer transaction. Postgres
locks the environment row before the delete and insert steps.

An `agent_completed` capture uses the same environment lock. In one
transaction, it removes earlier `live_monitor` and `agent_completed` rows for
the environment and repository. Then it inserts the detailed completion row.

An archive capture keeps earlier archive history. Current selection still
uses observation time, so an old archive cannot replace a newer workspace
observation.

The live-write throttle uses environment and repository identity. A sibling
session cannot create a second throttle scope for the same workspace state.

## Source precedence

Boot hydration and explicit refresh first resolve the requested session to its
task environment. They then inspect all live executions for that environment.

If any execution is live, a fresh agentctl result is authoritative. If the
live query fails, the backend sends no persisted replacement for that request.

If no execution is live, the backend reads the authoritative persisted rows
for the environment. It emits one status update for each repository.

The requested session remains the WebSocket route. It does not select the
snapshot and does not define the frontend storage key.

## Delivery identity

Every `status_update` payload carries `task_environment_id`. The nested status
object carries `repository_name` for non-root repositories.

The lifecycle publisher copies the environment ID from `AgentExecution`. The
orchestrator resolves the ID from the session when an older recovered execution
does not contain it.

Boot hydration copies the environment ID from the requested session binding.
It does not send a session-only status payload.

Other Git events remain session-routed. Commit history and branch actions still
use session provenance where their contracts require it.

## Frontend storage

The Git-status handler reads `task_environment_id` from each status payload.
It passes that ID directly to the session-runtime store.

The store writes `gitStatus.byEnvironmentRepo[task_environment_id][repository]`.
It does not derive this key from `environmentIdBySessionId`.

The handler ignores a status payload that has no environment ID. This rule
prevents a session ID from becoming an accidental environment key.

Timestamp ordering remains a response-order safeguard. An older payload cannot
replace a newer payload for the same environment and repository.

A sparse live snapshot normalizes `files` to an empty object. It clears stale
file details while it preserves current totals and file-name lists.

## Task summaries and direct consumers

Task-card and status-summary rebuilds collect unique environment IDs from task
sessions. They load one authoritative observation for each environment and
repository pair.

The loaders do not count one shared environment once for every sibling
session. Shared and inherited tasks can read the same environment observation.

Delivery ancestry queries join snapshots through environment ownership and
task environment bindings. They do not require the capture session to remain.

Session code statistics continue to use `session_id` as capture provenance.
Rows with null provenance do not contribute to a deleted session.

## Migration

Fresh databases create the final environment-scoped table directly. Existing
databases use one transactional cutover.

The cutover does these steps:

1. Lock the legacy table and its environment ownership inputs.
2. Join each snapshot to `task_sessions.task_environment_id`.
3. Remove rows that have no resolvable environment.
4. Partition the remaining rows by environment and normalized repository.
5. Keep the authoritative row from each partition with the new time rule.
6. Copy each winner to a final-shape shadow table.
7. Replace the legacy table and create the final indexes.

The winner keeps its session ID as nullable provenance. The migration does not
copy a session ID into the scoping key.

The cutover returns all unexpected errors. It does not use the best-effort
migration logger.

If any cutover step fails, the transaction keeps the complete legacy table.
The migration is a no-op when the final shape already exists.

SQLite and Postgres tests cover fresh creation, legacy cutover, rollback, and
same-database replay. Postgres uses the repository migration lock convention.

## Workspace views

Desktop and mobile Changes surfaces use the same environment-keyed store. This
change does not alter layout, navigation, touch behavior, or user copy.

The user regression uses two sessions in one environment. An older completion
contains files that a newer live observation no longer contains.

After reload and both sibling hydrations, Changes must not restore the removed
files. A focused store test covers the same message order.

No separate mobile browser test is necessary. The mobile surface uses the same
store and has no presentation change.

## Failure and recovery

A status write without a resolvable environment stops and records a bounded
log entry. The log does not contain a workspace path.

A live-query error does not permit a persisted fallback while any environment
execution is live. A later poll, focus refresh, or reconnect can retry.

A migrated row without session provenance remains valid until its environment
is removed. Provenance loss does not change current-status authority.

## Observability

Backend logs include the task environment ID, repository name, source type,
capture session ID when present, and selection reason.

Frontend debug logs include the delivered environment ID and repository name.
They also record timestamp rejection and cache invalidation results.

Logs must not contain workspace paths or file content.

## Related decisions

- [Keep Worktree Ownership at the Task Lifecycle](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md)
- [Persist Current Git Status with the Task Environment](../../../decisions/2026-08-30-environment-owned-git-status.md)
