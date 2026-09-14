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
