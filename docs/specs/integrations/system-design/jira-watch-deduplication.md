---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-JIRA-WATCH-DEDUP-001
created: 2026-11-10
owners:
  - kandev
---

# Jira watch deduplication design

## Purpose and boundaries

Jira owns `jira_issue_watch_tasks` and its `(issue_watch_id, issue_key)` claim identity. The task system remains authoritative for task existence.

This design uses the shared task database without adding integration knowledge to native task deletion. It covers future deletions and existing orphaned mappings.

The [watcher prompt design](jira-watcher-task-prompts.md) owns provider search and prompt data. This design defines which stored mappings suppress dispatch.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-INTEGRATIONS-JIRA-WATCH-DEDUP-001` | Mapping states, Polling, Atomic claim recovery, Failure and compatibility, Verification |

## Components and responsibilities

- `jira.Service.CheckIssueWatch` searches Jira and calls `Store.ListSeenIssueKeys` before returning new issues.
- `jira.Poller.checkIssueWatches` publishes those issues after its existing enabled and interval gates.
- `JiraWatcherSource.Reserve` delegates to `ReserveIssueWatchTask` before `WatcherDispatchCoordinator.Dispatch` creates a task.
- `AssignIssueWatchTaskID` completes the reservation after task creation. Existing release and reset flows retain their ownership.

Move only `ListSeenIssueKeys`, `ReserveIssueWatchTask`, and their new private helpers into `internal/jira/store_issue_watch_dedup.go`.
Their exported signatures remain unchanged. Other store methods remain in `store.go`.

## Mapping states

Evaluate every row independently within its exact watch. The state predicate is:

```sql
d.task_id = '' OR EXISTS (
  SELECT 1 FROM tasks t WHERE t.id = d.task_id
)
```

| Stored state | Suppress issue | Reservation result |
| --- | --- | --- |
| No mapping | No | One caller can claim |
| Empty `task_id` | Yes | Existing claim remains protected |
| Assigned task exists | Yes | Existing claim remains protected |
| Assigned task is archived or on a completed step | Yes | Existing claim remains protected |
| Nonempty `task_id` references no task | No | One caller can reclaim |

Task existence is existential; one row with the referenced ID is sufficient. Archival and workflow status do not affect this predicate.
An empty task ID represents unfinished creation, not evidence of deletion.

## Polling

`ListSeenIssueKeys` keeps its one-query bulk lookup and watch filter. Add the mapping-state predicate inside that watch's scope.
The query reads through `s.ro`. It performs no cleanup write.

`CheckIssueWatch` retains its existing error propagation and last-polled stamp. A failed existence query must not become an empty seen set.
No extra Jira request is needed. The existing first-page limit remains in effect.

## Atomic claim recovery

`ReserveIssueWatchTask` owns the authoritative recovery check. A filtered poll alone cannot bypass the existing unique reservation.

Use one native transaction on `s.db` for these operations:

1. Acquire the database's claim serialization before the first claim read or mutation.
2. Delete a row only for the supplied watch and issue, with a nonempty task ID and no matching task.
3. Insert a fresh empty reservation through the existing `ON CONFLICT(issue_watch_id, issue_key) DO NOTHING` statement.
4. Read `RowsAffected`, then commit. Return success only after a successful commit.

Reclaimed rows receive the normal new reservation ID, URL, and creation time. Keep all deletion and insertion statements on the transaction handle.
On any error, roll back both operations. Keep task creation, events, and provider calls outside the transaction.

SQLite uses the existing `db.OpenSQLite` writer transaction, which acquires the writer at BEGIN.
Tests of independent writers must use that factory, not separate default deferred pools.

PostgreSQL uses READ COMMITTED and a transaction-scoped advisory lock before the first mutation.
Use `pg_advisory_xact_lock(hashtextextended(..., 0))` with a Jira-specific, unambiguous watch-and-issue key.
All calls to this reservation method acquire the same lock for the same identity.
The uniqueness constraint remains the final insert guard.

The recovery predicate runs again at dispatch, using writer state. A stale poll snapshot therefore cannot authorize replacement of an existing task or unfinished claim.
If deletion commits after a poll still sees the task, a later eligible poll recovers it.

## Multiple watches

Recovery changes only the exact watch-and-issue row. It must not delete by issue key alone or by task ID across watches.

With the same issue in several watches, each watch retains its own claim. A valid mapping or unfinished reservation in watch B survives recovery in watch A.
If several watches reference one missing task, each mapping becomes independently reclaimable under its existing identity.
This does not introduce a global Jira issue claim.

## Failure and compatibility

Lookup, lock, deletion, insertion, row-count, and commit errors stop that claim. SQL errors never prove task absence.
The transaction preserves the old mapping on failure and permits a later retry after the database recovers.

Production startup supplies the task schema before watcher polling. Standalone Jira test fixtures must supply a minimal task table and actual task rows for assigned live mappings.
Do not create task tables in the Jira production constructor or hide a missing table with a permissive fallback.

No schema migration, new index, setting, API field, or event payload is required. The existence lookup uses the task primary key.
Existing orphaned rows remain stored until a matching issue reaches reservation recovery. No bulk sweep or manual database edit is required.

The store behavior applies to every Jira client through the existing service, including Cloud, Server/Data Center, MCP, and mocks.
Client authentication and provider search remain unchanged.

Reset inventory remains unfiltered and retains unfinished and archived mappings. Watch deletion still removes that watch's mappings and retains its created tasks.
Other integrations and task lifecycle cleanup remain unchanged.

Existing poll and dispatch error logs expose failed recovery through their current error paths. This correction adds no metric or diagnostic endpoint.

## Verification

Store tests cover absent mappings, unfinished reservations, active tasks, archived tasks, completed tasks, missing tasks, and mixed states across watches.
Include several watches sharing a missing task and separate workspaces using the same issue key.

Concurrency tests use independent database connections and an observable barrier. A simultaneous burst proves one claimant wins; all later claimants retain the winner's unfinished or assigned mapping.
A waiting-claimant test holds the writer or advisory lock in one transaction, observes the claimant's SQLITE_BUSY or PostgreSQL advisory-lock wait, installs a replacement task and mapping in the holder transaction, then commits. The waiting claim must lose and preserve the replacement reservation identity.
An insert failure after stale-row removal must roll back the original row unchanged.

A poller test publishes a recovered issue after native repository deletion. Force the watch due again and await events through bounded channels.
A native HTTP test obtains deletion preflight consent, deletes the task, and invokes the existing immediate Jira watch trigger.
It verifies eligibility, a successful claim, replacement task attachment, and suppression on the following trigger.
Existing-task and archived-task controls create no replacement.

SQLite and environment-gated PostgreSQL tests exercise both modified store methods. A skipped PostgreSQL test does not establish dialect coverage.

## Related decisions

- [Workspace-scoped integrations](../../../decisions/0030-workspace-scoped-integration-settings.md)
- [SQLite writer transaction admission](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md)

This local correction preserves the existing claim identity and database ownership. It requires no new architecture decision.

## Implementation plans

- [Deleted-task recovery](../../../plans/jira-watch-deleted-task-recovery/plan.md)
