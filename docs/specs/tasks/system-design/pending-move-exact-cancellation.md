---
status: current
system: tasks
requirements:
  - REQ-TASKS-PENDING-MOVE-CANCELLATION-001
  - REQ-TASKS-PENDING-MOVE-CANCELLATION-002
  - REQ-TASKS-PENDING-MOVE-CANCELLATION-003
created: 2026-08-30
owners:
  - kandev
---

# Exact Pending-Move Cancellation System Design

## Purpose and boundaries

The message-queue repository owns `pending_moves`, including a row-generation
identity that changes on replacement and survives snapshot restore or session
transfer. It exposes one exact compare-and-delete operation to the automation
Coordinator MCP surface. The operation never resumes or messages the target
session.

The shared Coordinator trust control plane owns `workspace_coordinator_grants`
and its owner-authorized lifecycle. This design only consumes that grant. The
pending-move TTL/reaper introduced separately remains the sole owner of expiry,
orphan cleanup, and correlated queued-prompt cleanup.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-PENDING-MOVE-CANCELLATION-001` | Row identity, MCP contract, transaction flow |
| `REQ-TASKS-PENDING-MOVE-CANCELLATION-002` | Trust boundary and authorization |
| `REQ-TASKS-PENDING-MOVE-CANCELLATION-003` | Audit, persistence, failure and recovery |

## Components and responsibilities

- `internal/orchestrator/messagequeue` owns pending-row generation IDs, the
  exact cancellation transaction, stable service errors, and audit persistence.
- `internal/mcp/scope` attaches the live server-owned agent execution ID to the
  principal created for an MCP stream.
- `internal/mcp/server` advertises `cancel_pending_move_kandev` only in the
  automation Coordinator catalog, suppresses its arguments and results from MCP
  logs, and forwards schema-invalid calls to the backend denial-audit path
  instead of terminating them in the transport wrapper.
- `internal/mcp/handlers` derives the caller from the scoped principal, maps
  stable errors, strictly rejects unknown or mistyped fields, and bypasses
  predicate-light generic preflight so the repository transaction performs the
  only target authorization decision.
- The Tasks repository creates the replay-safe
  `workspace_coordinator_grants` relation consumed by this operation. Grant
  management remains outside this capability.

## Data and contracts

`pending_moves.id` is an opaque UUID for one row generation. `SetPendingMove`
always rotates it. Snapshot/restore and session transfer preserve it because
they preserve the same deferred request. Existing `move_id` remains the logical
move identity and is not sufficient on its own to identify a generation.

The shared grant relation enforces designation integrity in the database.
`workspace_id` is a non-empty primary key and cascading foreign key to
`workspaces(id)`; `coordinator_task_id` is non-empty; `tasks(workspace_id, id)`
is unique; and the cascading composite foreign key
`(workspace_id, coordinator_task_id) -> tasks(workspace_id, id)` makes a
missing or cross-workspace Coordinator task unrepresentable. The runtime join
revalidates the same pair during cancellation but is not a substitute for this
persistence constraint.

`cancel_pending_move_kandev` requires exactly these public UUID fields:

1. `pending_move_id`
2. `session_id`
3. `task_id`
4. `move_id`
5. `workflow_id`
6. `expected_current_workflow_step_id`
7. `expected_target_workflow_step_id`

It returns `cancelled: true` plus a correlation ID and exact prior-state
readback only after commit. Missing or non-canonical public identifiers map to
`pending_move_invalid_argument`. Every authorization, relation, absence, or
predicate failure maps to `pending_move_not_found_or_changed`. Storage or audit
failures map to `pending_move_cancel_failed`. None of these errors echoes a
submitted identifier.

## Trust boundary and authorization

The request body carries no actor fields. The MCP stream supplies authenticated
user, surface, workspace, task, session, and agent execution identity. The
transaction accepts only `actor_kind = coordinator` and validates:

- a live grant for the caller task in the target workspace;
- the authenticated user is the workspace owner;
- the caller task and session belong to that workspace;
- the caller session still carries the same active execution ID and a live
  executor state;
- the target task is reachable through the caller Coordinator's named task
  tree (the caller or one of its descendants), rather than merely sharing a
  workspace;
- the caller is not targeting its own task;
- target task, target session, current step, target step, and workflow form one
  same-workspace relation.

This is fail closed: a missing grant, stopped execution, forged principal,
broken relation, or cross-workspace target follows the same zero-mutation path.
Denial evidence omits target identifiers that were not safe to establish.

Lifecycle stop persists the terminal executor status before it removes the
execution from the in-memory registry. The cancellation transaction therefore
cannot accept a stale durable execution row after a stop has linearized.

## Transaction flow

The repository serializes on the keyed queue session and starts one database
transaction. Within it, the repository:

1. validates the server-attested Coordinator and same-workspace relations;
2. reads the pending row and current/target workflow state;
3. compares all seven exact predicates;
4. writes the structured outcome audit;
5. on an exact authorized match, deletes with the row ID and complete tuple in
   the delete predicate and requires exactly one affected row;
6. commits, then returns the immutable success readback.

The row read, authorization checks, audit, and delete share one transactional
snapshot. A replacement cannot be mistaken for the inspected generation. Two
callers serialize so only one can affect the row.

## Audit and observability

`pending_move_cancellation_audit` stores a correlation ID, server-attested
actor and caller identity, workspace, exact target tuple when safe, observed
prior steps, outcome, changed flag, identifier-presence/canonicality flags, and
timestamp. It stores no prompt, credential, tool payload, or error detail.

The service emits a structured success event with the same safe identifiers.
Mismatch and invalid events are redacted, and the backend MCP transport redacts
the whole tool payload before logging. Durable audit time is authoritative.

## Persistence, migration, and rollback

Initialization additively creates the audit table, indexes, coordinator-grant
relation, and pending-row identity where needed. DDL is replay-safe for SQLite
and PostgreSQL. Migration creates the unique task workspace/ID parent key before
the composite Coordinator-grant foreign key. Existing pending rows receive
identities during the compatible pending-move migration owned with the
TTL/reaper work.

Rollback disables/removes the MCP exposure and transaction call while retaining
the additive row ID, grant relation, and audit evidence. No destructive schema
rollback is required. The TTL/reaper can continue using the row identity for
its own compare-and-delete behavior.

## Failure and recovery

Authorization and tuple failures are ordinary stable misses. A successful
request retried after an uncertain client response returns the same stable miss
and makes no additional change. Audit insertion or row deletion failure rolls
back the full transaction and returns only the sanitized failure code.

The operation deliberately does not remove a correlated queued prompt. It is a
reviewed administrative disarm of one transition, not queue cleanup. Automatic
expiry remains responsible for its existing prompt-cleanup policy.

## Verification

- Repository tests cover exact success, every predicate mismatch, broken
  relations, replacement races, two callers, retry, and transaction rollback.
- Authorization tests cover designated same-workspace Coordinator success and
  ordinary, synthetic, forged, stopped, revoked, non-owner, self, and
  cross-workspace denial.
- MCP tests cover the fixed catalog, server-owned caller identity, stable error
  mapping, unknown-field rejection, and full-payload log redaction.
- Migration tests cover replay, non-empty grant identifiers, and database
  rejection of cross-workspace Coordinator pairs.
- A final reviewed live acceptance may name one row and verify that only that
  row disappears while task lane, session, tags, and queue state remain
  unchanged. It must occur only after review, merge, and deployment.

## Related decisions

- [Bind administrative pending-move cancellation to an exact row generation](../../../decisions/2026-08-30-exact-pending-move-cancellation.md)
