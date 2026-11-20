---
status: current
system: tasks
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-002
  - REQ-TASKS-CROSS-WORKSPACE-TRANSFER-003
---

# Audited cross-workspace task transfers System Design

## Purpose and boundaries

The task system owns the transfer because it owns task identity, placement,
lifecycle, and movement authorization. The transfer is one domain command with
three layers: an MCP surface that attests the actor, a service that owns
boundary authorization, and a repository transaction that preserves identity.

Adjacent contracts this design uses but does not own:

- Workspace ownership and repository/worktree identity belong to the
  [workspace system](../../workspaces/README.md).
- Session lifecycle and turn ownership belong to the session lifecycle design.
- The MCP tool surface and profile gating belong to the MCP server surface.
- Notification and cache rehydration belong to the gateway contract.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001` | Transfer command, Atomic transaction, Destination lane resolution, Preservation and rebinding |
| `REQ-TASKS-CROSS-WORKSPACE-TRANSFER-002` | Actor attestation, Boundary authorization |
| `REQ-TASKS-CROSS-WORKSPACE-TRANSFER-003` | Idempotency and replay, Audit, Post-commit reconciliation |

## Components and responsibilities

- **MCP tool (`transfer_task_kandev`)** — decodes the request, validates the
  schema, attests the actor from the server session (never the payload), and
  forwards to the service. Registered only on configuration, external, and
  Office surfaces with a server-attested CEO or coordinator path; ordinary task
  agents never receive it.
- **Task service** — resolves workspace and workflow reachability, delegates
  actor attestation to the coordinator authorizer, and constructs the typed
  transfer command with authorized owner identity.
- **Repository transfer transaction** — locks the task row, verifies every
  placement predicate, resolves the destination lane, validates equivalence and
  workspace boundary, inventories relations, persists the transfer and audit
  record, and commits atomically.
- **Broadcast and reconciliation path** — after commit, publishes the
  destination-scoped task update through the source boundary and reconciles
  dependent caches and events exactly once.

## Data and contracts

The typed command binds: exact task ID; expected source workspace, workflow,
and lane; expected task `updated_at` generation; destination workspace,
workflow, and lane (stable lane ID or unique exact name); idempotency key; and
the `preserve-task-identity-v1` policy.

The durable receipt contains: operation ID, source and destination placement,
committed task generation, step-transition ID, session census, preservation
counts and digest, idempotency key, and policy. It intentionally contains
identifiers and counts, never prompts, message bodies, secrets, or credentials.

Actor identity is a server-attested structure (human or coordinator) resolved
from the authenticated context; request payloads never populate it.

Configuration authority requires the persisted calling session's boolean
configuration-purpose marker. Task metadata and request fields cannot grant
it. Automation, coordinator, and managed conversation restrictions remain in
force. Office CEO attestation and the transaction both require an enabled,
undeleted source profile with an active status and the exact running session
and runner-seat binding.

Lane equivalence includes session start/end policy, session target, completion
on entry, and the unclassified fallback veto, alongside prompts, events,
participants, completion signals, and WIP policy.

Relation discovery inventories and locks every task-keyed table. An unmapped
workspace-owned relation blocks transfer when it contains rows for the selected
task; unrelated or empty tables do not block other tasks. Such rows retain their
original workspace fence until an explicit mapping contract exists.

The stable error vocabulary maps every stale, ambiguous, incompatible,
unauthorized, or idempotency-mismatched outcome to `task transfer conflict` at
the boundary, without distinguishing task existence.

## Control flow

1. The MCP surface decodes and validates the request; schema rejections are
   audited before answering.
2. The service authorizes task, workspace, and workflow reachability and
   resolves the attested actor. Denied attempts audit and answer without
   existence disclosure.
3. The repository transaction:
   - locks the task and reads its placement;
   - fails closed on any predicate, generation, boundary, lane-equivalence,
     capacity, or relation mismatch;
   - resolves the destination lane by stable ID or unique exact name;
   - persists the transfer with its receipt and audit row.
4. After commit, the broadcast path publishes the destination task update
   through the source notification boundary and reconciles caches and events.
   Reconciliation survives caller cancellation and runs exactly once.

## Failure and recovery

Every failure before commit leaves source and destination state unchanged and
returns the stable conflict result. Committed transfers are durable: a retry
after a caller-visible failure replays the stored receipt under the same actor,
session, and key. Post-commit reconciliation is idempotent under cancellation.

A caller must use a fresh key after its denied or failed attempt. Rejected
attempts from another actor or session cannot poison that caller's key.
Committed-receipt replay takes precedence over rejected-attempt history.
Audit-delivery retries share a server-generated attempt ID; that ID is accepted
only on the trusted internal audit path and never in a transfer request.

## Persistence

Transfer receipts and audit rows live in additive task-store tables owned by
the task repository. Migrations are additive; rolling back the application
binary leaves receipts and audit rows intact for a later upgrade. No rollback
path drops the transfer ledger.

## Security

Authorization is server-attested. The workspace boundary requires one shared
owner for source and destination. Ambiguous lane mapping, unattested callers,
and mismatched runner-seat mapping fail closed. Audit rows are redacted to
identifiers and outcomes, never transcript content.

## Observability

Each attempt writes one audit row with its outcome. Structured logs record
authorization decisions and post-commit reconciliation completion. The
receipt's preservation counts and digest make a transfer's inventory observable
without exposing content.

## Related decisions

- [Separate runtime permissions and advisory actions](../../decisions/2026-09-07-separate-runtime-permissions-and-advisory-actions.md)

## Workspace-first concurrency boundary

This design amendment was independently reviewed and accepted in V6; a later
explicit instruction authorized the local implementation. The candidate takes
workspace-first EXCLUSIVE NOWAIT reservations; the delivered version still uses
blocking SHARE ROW EXCLUSIVE. Delivery requires independent actual-diff review.

For AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.6, acquire source and destination
workspace rows in sorted order before any table or task-row lock. Resolve
non-locking locator snapshots first, then revalidate exact source placement,
generation, workspace ownership and requested destination after those locks.
Do not discover and append a workspace under held locks. A changed authoritative
source predicate returns the existing stable conflict. Preserve only existing
bounded retries that already repeat authorization and the complete lock set.
Low-level locator races without an existing authorized retry seam roll back
and conflict; add no generic retries or reauthorization callbacks.

Next reserve the complete existing discovered relation census in sorted order
with PostgreSQL `EXCLUSIVE NOWAIT`. Unlike SHARE ROW EXCLUSIVE, this also
conflicts with pre-existing SELECT FOR UPDATE owners' ROW SHARE locks.55P03
rolls back the whole transaction, releases partial reservations and workspace
locks, then records the existing conflict audit and returns the stable conflict.
Do not wait on an upgrade or hide a retry loop. If the reservation succeeds,
new row-lock/DML owners wait at the relation boundary until commit. Plain
readers remain allowed. The successful transfer retains current mapping,
preservation digest, idempotency, audit and single-event behavior. SQLite
retains its existing immediate writer and serialization-row reservation.

Add `tasks` and `task_step_transitions` explicitly to the reservation base:
`tasks` has no task_id column and is missed by discovery; transitions are
excluded from the before/after preservation comparison and digest because
transfer appends a ledger row. The receipt separately reports the post-write
transition count. Keep those semantics. The complete set is the existing
discovered projection/count union, current core/conditional tables, plus these
two tables. It covers task/session/workspace/step FK targets; generic projection
updates change only workspace_id. Never acquire a late table after mutation. Unknown relation ownership
refusals remain unchanged.

The necessary tradeoff is stronger contention for the already exceptional
transfer operation: a row-lock owner in another workspace may cause a prompt
conflict instead of a blocking wait. Ordinary Office completion uses workspace
exclusion and never acquires these relation locks. This does not introduce a
global table-lock policy for ordinary writes. Retaining blocking SRE is not a
safe fallback if this tradeoff is rejected: a per-task preservation protocol
covering every discovered relation writer would require a broader design.

The [Office completion boundary](../../office/system-design/human-done-final-step.md#one-acquisition-graph-for-runtime-writers)
owns the eligibility writer order. Minimal real PostgreSQL lock-shape probes
observed row/table cycles for both table-before-hierarchy SRE and
workspace-first SRE NOWAIT, and tested rollback/progress for EXCLUSIVE NOWAIT.
Those probes do not constitute TransferTask product RED/GREEN. Required tests
include opposite transfers, prior row owners, partial reservation failure,
cancellation, exact-source races, complete census and unchanged preservation
receipts/audits. No schema or new lock namespace is proposed.
