---
status: draft
system: tasks
created: 2026-09-07
owners:
  - kandev
---

# Audited cross-workspace task transfers Requirements

## Overview

A task belongs to one workspace, but work outgrows that boundary: a platform
reorganization moves a board between workspaces, a delivery coordinator needs to
hand an in-flight task to the workspace that now owns its lane, and a human
restructures workspaces without abandoning running sessions. Recreating a task
elsewhere loses its plan history, sessions, and audit trail, while stopping the
agent first wastes an active turn.

The cross-workspace transfer capability moves one exact task identity — the
same UUID, sessions, plan, relations, and history — from an authorized source
workspace to an equivalent lane in a destination workspace owned by the same
owner. The transfer is one atomic operation with an idempotent receipt and a
redacted audit row for every attempt.

The task system owns this contract because task identity, placement, lifecycle,
and movement authorization are durable task behavior. The MCP surface exposes
it; authorization is server-attested, never taken from a request payload.

## Terminology

- **Transfer:** One atomic movement of an existing task's identity from a source
  workspace to a same-owner destination workspace, preserving every durable and
  runtime artifact.
- **Placement predicate:** The exact source workspace, workflow, and lane, plus
  the task's current `updated_at` generation, that a request must bind before
  the transfer is attempted.
- **Equivalent lane:** A destination lane whose configurable semantics match the
  source lane: name, effective prompt, inherited workflow prompt, events,
  effective agent profile, stage type, pull source, manual-move and start flags,
  archive timer, signal-gating, cancel-completion behavior, WIP limit, and the
  durable participant slate.
- **Idempotency key:** A caller-supplied identity for one transfer request. An
  exact retry returns the stored receipt; the same key with a changed request,
  actor, or session conflicts.
- **Preservation policy:** The versioned contract `preserve-task-identity-v1`
  that defines what the transfer must preserve and what the receipt reports.
- **Receipt:** The durable per-operation result containing identifiers, counts,
  and digests — never prompts, message bodies, secrets, or credentials.
- **Server-attested actor:** The transfer caller identity the backend resolves
  from the authenticated session or principal. A request payload can never
  assert it.

## Requirements

### REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001: Atomic identity transfer

**Intent:** An authorized caller can move one exact task to a same-owner
workspace without recreating data, stopping sessions, or exposing a
half-transferred state.

#### Acceptance criteria

- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.1:** When a transfer request names an
  existing task, binds the exact source workspace, workflow, and lane, the
  current task generation, a destination workspace and workflow, one
  destination lane resolved by stable lane ID or a unique exact lane name, a
  unique idempotency key, and the `preserve-task-identity-v1` policy, the system
  shall move that task into the destination placement in one atomic operation
  and return a receipt for the same task UUID.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.2:** When any bound predicate no
  longer matches the task's persisted placement or generation, when the
  destination lane is missing or not uniquely mapped, when the destination lane
  is not equivalent to the source lane, when the destination lane is at WIP
  capacity, or when the source and destination workspaces do not share one
  owner, the system shall reject the transfer with one stable conflict result
  and shall not change any source or destination state.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.3:** When a transfer completes, the
  system shall preserve, without cancellation, duplication, or loss: all
  sessions including a running writer turn, plans and messages, task
  repositories, worktrees, and branches, parent and dependency relations,
  pull-request associations, pending moves, status summaries, terminal Done
  archive timing, and Blocked preservation state.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.4:** When the task carries
  workspace-owned relations that cannot follow it — labels scoped to another
  workspace, workspace groups, project bindings, or other relations without a
  destination mapping — the system shall reject the transfer before moving the
  task rather than silently dropping or re-scoping those relations.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-001.5:** When a transfer commits, the
  system shall rebind workspace-owned projections of the task in the same
  transaction, emit the destination task update through the source notification
  boundary, and reconcile dependent caches and events exactly once, so a
  subsequent read observes the destination placement without a duplicate or
  missing event.

### REQ-TASKS-CROSS-WORKSPACE-TRANSFER-002: Server-attested authorization

**Intent:** Only an authorized human or server-attested coordinator can transfer
across workspaces, and no request payload can grant that authority.

#### Acceptance criteria

- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.1:** When the transfer surface is
  invoked, the system shall resolve the actor from the authenticated server
  context and shall admit only a human caller or a server-attested coordinator
  holding destination-workspace authority; request-supplied actor fields shall
  never influence this resolution.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.2:** When an ordinary task agent, an
  automation coordinator without destination reach, or any unattested caller
  invokes the transfer surface, the system shall deny the attempt without
  revealing whether the named task exists.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-002.3:** When a coordinator-session
  transfer is invoked, the system shall map at most its own runner seat to the
  destination workspace's unique active authority and shall reject the transfer
  when that mapping is missing or ambiguous.

### REQ-TASKS-CROSS-WORKSPACE-TRANSFER-003: Idempotent replay and audit

**Intent:** Retrying a completed transfer is safe and every attempt is
auditable, without leaking task existence or transcript content.

#### Acceptance criteria

- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.1:** When the same bound actor and
  session retries the same request — same idempotency key and unchanged
  request identity — the system shall return the stored receipt without
  re-executing the transfer and without depending on mutable workflow or lane
  configuration.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.2:** When the same idempotency key is
  reused with a changed request, a different actor, or a different session, the
  system shall return one stable conflict result and shall not alter the
  committed transfer.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.3:** When a caller retries after the
  caller's request context is cancelled, the system shall still complete the
  committed transfer's post-commit reconciliation exactly once.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.4:** When any transfer attempt is
  accepted, rejected, or denied, the system shall write one audit record that
  identifies the operation, the bound placement, the idempotency key, and the
  outcome, and that never contains prompts, message bodies, secrets, or
  credentials.
- **AC-TASKS-CROSS-WORKSPACE-TRANSFER-003.5:** When a caller reuses the
  idempotency key of a denied or failed attempt, or attempts a replay without
  current task or destination-workspace access, the system shall reject the
  retry or replay and shall not fabricate a successful receipt.

## Out of scope

- Recreating, copying, or splitting a task; the transfer moves one identity and
  creates no task.
- Cross-owner or cross-trust-boundary movement; both workspaces must share one
  owner, enforced by REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001.2.
- Resolving project membership, workspace groups, or labels across the
  boundary; transfers that carry them fail closed by
  REQ-TASKS-CROSS-WORKSPACE-TRANSFER-001.4 until a mapping contract exists.
- Changing same-workspace move behavior; the existing manual-move contract
  remains unchanged.
