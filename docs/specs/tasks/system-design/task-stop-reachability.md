---
status: draft
system: tasks
requirements:
  - REQ-TASKS-TASK-STOP-REACHABILITY-001
created: 2026-09-02
owners:
  - Kandev
---

# Task Stop Reachability System Design

## Purpose and boundaries

The task system owns the task-scoped stop: resolving which work a task still
owns and halting it. This design changes only how that set is resolved.

The agent system owns the execution registry and the stop primitive; this
design reads the registry and calls the primitive. Session-scoped stop and the
`agent.cancel` recovery path already resolve an execution directly and are not
changed. Which session becomes terminal, and why, belongs to the owning
behavior, such as
[agent stall recovery](../../agents/system-design/agent-stall-recovery.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-TASK-STOP-REACHABILITY-001` | [Two sources of truth](#two-sources-of-truth), [Resolution](#resolution), [Stopping a recovered session](#stopping-a-recovered-session), [Failure and recovery](#failure-and-recovery) |

## Two sources of truth

Two independent records describe a task's live work:

- The **persisted session state** in the database, queried by an active-state
  set that deliberately excludes terminal states.
- The **execution registry** in the running backend, which holds an entry for
  as long as an agent process is tracked.

They can disagree. Any path that marks a session terminal without tearing down
its execution produces a session that the active-state query cannot see and a
process that is still running. The active-state query alone is therefore the
wrong authority for a stop.

The registry is the authority for what can be stopped. The persisted state
remains the authority for what the session means.

## Components and responsibilities

- **The task-scoped stop** resolves the union of both sources, then stops each
  resolved session.
- **The execution registry** answers which session and execution pairs of a task
  currently have a registered execution. Its existing per-execution task
  identity is enough; no new index is required for a set this small.
- **The session store** supplies the session row for a session that only the
  registry knew about, so the stop has the identity and task binding it needs.

## Resolution

1. Query sessions in an active persisted state for the task, as today.
2. Ask the registry for sessions of the task that have a registered execution.
3. Add any registry-only session by loading its session row. Keep the paired
   registry reference when the row cannot be loaded, log the load error, and
   schedule teardown by the captured execution ID.
4. If the union is empty, report that no execution was found and change
   nothing.

Ordering keeps existing behavior first, so a task with no orphan resolves
exactly the set it resolves today.

## Stopping a recovered session

A session reached through the active-state query keeps today's behavior: it
transitions to the cancellation state and emits the existing events.

A session reached only through the registry is normally terminal. Recovery
holds the per-session lock used by resume, then checks the current row and the
captured execution identity. It skips the state transition for a terminal row,
or when a replacement execution owns the session. If the captured execution
still owns an active row, recovery uses the normal cancellation transition.
Teardown always targets the captured execution ID.

Success is reported when teardown is scheduled for at least one execution. It
does not confirm that detached runtime teardown has finished. A task whose only
live work was an orphan therefore stops successfully, and the caller's existing
post-stop task handling proceeds as it does for any successful stop.

## Data and contracts

The stop entry point keeps its signature and its not-found error. One read-only
capability is added to the agent-manager interface the task system already
depends on: given a task identity, return paired session and execution
identities that currently have a registered execution. It is a snapshot; a
session that changes between the snapshot and the stop is handled by the
per-session lock and exact execution check.

No HTTP, WebSocket, or persisted contract changes.

## Failure and recovery

- A failed stop for one session does not prevent the others; the operation
  reports failure only when every resolved session failed.
- A repeated stop finds the executions already removed and reports not-found per
  session without creating another transition.
- A registry lookup that returns nothing degrades exactly to today's behavior.
- A session row that cannot be loaded is logged, but its captured execution is
  still scheduled for teardown. The load error remains visible to the caller.

## Persistence

No schema change. The only persistence difference is a deliberate omission: an
orphan's terminal state is not overwritten.

## Security

The registry is process-local and already scoped per execution. Resolution is
constrained to the requested task, so a stop cannot reach another task's
execution. No new caller-controlled input is introduced.

## Observability

The stop log distinguishes sessions resolved from the active-state query from
sessions recovered through the registry, and names each recovered session and
execution. An orphan is then visible in the logs at the moment it is reclaimed
rather than only as an unexplained not-found error.

## Related decisions

- [ADR-2026-09-02-terminal-stall-owns-process-teardown](../../../decisions/2026-09-02-terminal-stall-owns-process-teardown.md)
