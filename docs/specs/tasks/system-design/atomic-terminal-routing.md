---
status: current
system: tasks
requirements:
  - REQ-TASKS-ATOMIC-TERMINAL-ROUTING-001
  - REQ-TASKS-ATOMIC-TERMINAL-ROUTING-002
  - REQ-TASKS-ATOMIC-TERMINAL-ROUTING-003
---

# Atomic terminal routing System Design

## Purpose and boundaries

This design serializes workflow-route producers on the task's persisted source
step and gives each request one durable operation identity. It covers MCP moves,
explicit completion, turn-end deferred application, workflow auto-advance, and
moves issued from a persisted merged-PR lifecycle prompt. Exact administrative
pending-row cancellation and general stale-session cleanup remain separate.

## Transaction and lock order

The task repository is the physical lane arbiter. A producer carries a
server-derived `routing.Operation` on context into the repository transaction.
The transaction locks and validates the task source generation, performs WIP
admission, updates lane and state, inserts the task-step transition, records the
route operation, allocates one route effect, and settles every deferred row for
the task when the resulting state is terminal.

The first task read stamps the request's expected source step when a caller did
not supply one. The repository rechecks that generation under the task lock.
Operation IDs are global immutable identities: an existing ID may advance only
the same task, producer, source, target, session/turn, actor, and external
cause. A different identity receives a conflict and its task mutation rolls
back in the same transaction.

Deferred admission uses the same order:

1. task row and expected source step;
2. queue-session lock rows in sorted order;
3. exact pending row generation;
4. destination capacity lock during apply.

On PostgreSQL the task no-op update is the row lock. On SQLite the same update
acquires the single-writer lock. A terminal transaction therefore either waits
for and deletes an earlier deferred admission, or commits first and causes the
later admission to fail its terminal/source predicate.

## Persistent model

- `pending_moves.id` is the immutable row generation; `move_id` is the route
  operation identity. `expected_workflow_step_id` and `initiating_turn_id`
  attest the source generation.
- `workflow_route_operations` stores producer, expected and observed lanes,
  target, session/turn, actor, external cause, stable outcome, supersession,
  transition link, and effect link. A terminal stored outcome is not replaced
  by a retry.
- `task_step_transitions` remains the authoritative physical lane ledger.
- `workflow_route_effects` gives the winning transition one destination-entry
  identity. Engine-owned on-entry actions continue to claim their exact
  `workflow_step_entries` markers, so retrying delivery cannot create another
  logical entry. Delivery first claims the route effect with a token and lease
  while it prepares the session. Immediately before any lifecycle side effect,
  the same token moves the effect from `claimed` to `executing`. An expired
  `claimed` lease is recoverable; `executing` is non-reclaimable because an
  external outcome may already exist. Completion accepts only that execution
  token and same-token retries read back as successful. The route effect has
  typed readback for its operation, transition, target, and status.

## Producer behavior

- Active-session terminal MCP moves commit synchronously. Nonterminal moves
  remain deferred and report a queued outcome.
- Every automatic or manual move carries an expected source step. A mismatch
  records `stale_source` and cannot mutate pending storage.
- Completion signals use the latest turn's immutable step stamp and operation
  identity. The signal operation is linked to the later winning transition;
  stale signals are durable audit rows with no transition or effect.
- If a signal loses its atomic claim, the handler reloads the task. A changed
  source is recorded as `stale_source`; only an unchanged source is treated as
  a duplicate signal.
- Persisted merged-PR prompt metadata supplies the exact repository/PR cause.
  The prompt remains advisory; a move issued from that turn is recorded as a
  `merged_pr` producer and converges with a manual Done route.

## Authorization

The MCP dispatcher derives its principal from the live task/session stream.
Ordinary Kanban and Office agents may route only their bound task. The existing
automation/Coordinator surface is its server-attested grant and may route a
different task only in the principal workspace. Workflow, task, session, turn,
actor, and PR identities from request payloads are not trusted.

## Recovery and compatibility

Turn-end reads without deleting, reloads the pending operation by its immutable
move ID, applies expected-step CAS with that original producer/turn/actor/cause,
then exact-deletes the row. A crash after commit leaves either no row (terminal
settlement) or a row whose retry observes the committed target and cleans up
idempotently. A legacy row without an expected step fails closed for exact
cancellation or TTL.

Route-effect recovery prioritizes duplicate prevention over automatic liveness.
A worker may reclaim an expired `claimed` effect because no lifecycle side
effect has started. The worker atomically changes it to `executing` before
`on_exit` or `on_enter`. Later workers report that state for reconciliation and
do not schedule automatic redelivery. The executing worker retries transient
completion writes with its exact token without rerunning lifecycle actions. If
the process dies after execution begins, the durable `executing` state remains
for explicit reconciliation because the backend cannot safely infer whether an
external action committed.

Schema changes are additive and replay-safe on SQLite and PostgreSQL. Rollback
keeps the added columns/tables; operators must settle new-format rows before an
older backend resumes queue processing. No production row is backfilled from a
mutable current task step.

## Failure modes

| Condition | Outcome |
|---|---|
| Source lane changed | `stale_source`; no task, pending-row, or effect mutation |
| Same operation retried | Stored outcome and links are returned/read back |
| Same operation ID carries another immutable request | Conflict; transaction rolls back |
| Different request after terminal commit | `already_satisfied`; no new transition/effect |
| Newer pending row replaced/restored/transferred by stale snapshot | Generation conflict; newer row survives |
| Crash after task commit before turn-end cleanup | Retry observes target and exact-cleans; no second transition |
| Preparation worker dies with a `claimed` effect | Expired lease is reclaimable because no lifecycle side effect began |
| Worker stalls or dies after effect becomes `executing` | Successors do not reclaim; effect remains attributable for explicit reconciliation |
| Exact-token completion response is lost | Same-token retry reads the completed outcome as success without rerunning effects |
| Legacy row lacks expected source | Preserved and never auto-applied |
