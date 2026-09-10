---
status: active
system: tasks
created: 2026-08-30
owners:
  - kandev
---

# Exact Pending-Move Cancellation Requirements

## Overview

A deferred workflow move can remain live while its target session is waiting for
input. Resuming that session can apply the move before an operator can repair the
task safely. Kandev therefore provides a narrow administrative operation that
removes one reviewed `pending_moves` row only when its complete identity and
workflow state still match.

A Coordinator preparing that repair rarely already holds the row's exact
identity: the seven predicates the cancellation requires are what it needs to
discover safely first, without resuming or messaging the target session. Kandev
therefore also provides a read-only companion that returns that exact identity,
or an authoritative proof that no row is armed, under the same authorization
boundary as the cancellation.

The Tasks system owns this contract because the operation changes persisted task
transition state. The shared Coordinator trust contract owns designation of the
workspace Coordinator; this capability consumes that designation without
creating another role model.

## Terminology

- **Pending-move generation:** One persisted `pending_moves` row, identified by
  a row ID that changes whenever a session's pending move is replaced.
- **Exact predicate:** A caller-supplied pending-row ID, keyed session ID, task
  ID, move ID, workflow ID, expected current workflow-step ID, and expected
  target workflow-step ID.
- **Designated Coordinator:** The live, server-attested Coordinator task and
  execution authorized for the target workspace by the shared Coordinator
  trust contract.
- **Pending-move census:** The read-only companion result: either the exact
  predicate tuple for one task's currently armed row (`found: true`), or an
  authoritative `found: false` proving no row is armed. It is not the same as
  a null or missing-permission response, which this capability never returns
  for an authorized request.

## Requirements

### REQ-TASKS-PENDING-MOVE-CANCELLATION-001: Exact administrative cancellation

**Intent:** Let a reviewed workspace administrator disarm one hazardous deferred
move without resuming its session or changing unrelated task state.

#### Acceptance criteria

- **AC-TASKS-PENDING-MOVE-CANCELLATION-001.1:** When a designated Coordinator submits
  all seven exact predicates and the live row and task relations match, the
  system shall delete exactly that pending-move generation and return an exact
  readback of the removed row, move, task, session, workflow, prior current
  step, prior target step, and queue timestamp.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-001.2:** When any one exact predicate differs,
  a relation is missing, the row was replaced, or the target is outside the
  Coordinator's workspace, the system shall leave all pending-move and task
  state unchanged and return the same non-leaking not-found-or-changed result.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-001.3:** When two authorized callers race for
  the same generation, exactly one shall succeed and every loser shall receive
  the stable not-found-or-changed result without deleting a replacement.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-001.4:** When a successful request is retried,
  the retry shall be effect-idempotent: no additional state changes occur and
  the result is the stable not-found-or-changed response.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-001.5:** Cancellation shall remove only the
  named pending-move row; the target task's lane, session, tags, queued prompt,
  and other task or workflow state shall remain unchanged.

### REQ-TASKS-PENDING-MOVE-CANCELLATION-002: Least-privilege authorization

**Intent:** Keep this exceptional mutation unavailable to ordinary agents and
make forged or stale caller identity fail closed.

#### Acceptance criteria

- **AC-TASKS-PENDING-MOVE-CANCELLATION-002.1:** The operation shall be advertised only
  on the automation Coordinator MCP surface and shall preserve ordinary-agent
  self-only authorization everywhere else.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-002.2:** The system shall derive actor,
  workspace, caller task, caller session, caller execution, and authenticated
  user from server-owned context; caller-supplied values shall not override
  those identities.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-002.3:** A Coordinator request shall be
  authorized only while the workspace grant, caller task/session/execution,
  reachable target task/session/workflow, and workspace-owner identity are all
  live and consistent in the cancellation transaction. A terminal lifecycle
  transition shall durably revoke execution liveness before releasing the
  execution. Coordinator-grant persistence shall also reject empty identifiers
  and make a cross-workspace task/grant pair unrepresentable.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-002.4:** Ordinary agents, synthetic or forged
  principals, self-targets, revoked or stopped Coordinators, non-owner callers,
  and cross-workspace requests shall make no mutation and shall receive the
  same non-leaking not-found-or-changed response.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-002.5:** Missing or non-canonical public
  identifiers shall be rejected before target lookup with a stable invalid-
  argument result; raw SQL and predicate-light cancellation shall not be
  exposed.

### REQ-TASKS-PENDING-MOVE-CANCELLATION-003: Atomic evidence and recovery

**Intent:** Make every attempt reviewable without weakening atomicity or leaking
secrets.

#### Acceptance criteria

- **AC-TASKS-PENDING-MOVE-CANCELLATION-003.1:** Every accepted, denied, mismatched, or
  invalid attempt shall emit structured evidence with a correlation ID, actor,
  exact identifiers when safe, prior current and target steps when observed,
  outcome, changed flag, and timestamp, without prompts, credentials, or tool
  payload logging.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-003.2:** The success audit record and exact row
  deletion shall commit in one transaction; failure of either write shall leave
  both the pending row and audit state unchanged.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-003.3:** Schema upgrades shall be additive and
  replay-safe for SQLite and PostgreSQL, and rollback shall not require
  destructive removal of audit evidence or row identities. The shared grant
  relation shall use the database-enforced workspace/task composite key rather
  than relying on an authorization-time join alone.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-003.4:** Automatic pending-move expiration and
  orphan reaping shall remain owned by the existing TTL/reaper lifecycle; this
  operation shall neither duplicate nor weaken that policy.

### REQ-TASKS-PENDING-MOVE-CANCELLATION-004: Exact pending-move read (census)

**Intent:** Let a designated Coordinator safely discover the exact predicate
tuple for one task's currently armed pending move — or an authoritative proof
that none is armed — before deciding whether to call the exact cancellation,
without resuming or messaging the target session and without mutating any
pending-move or task state.

#### Acceptance criteria

- **AC-TASKS-PENDING-MOVE-CANCELLATION-004.1:** When a designated Coordinator submits
  a target task ID and an armed pending-move row exists for a live, reachable,
  same-workspace relation, the system shall return `found: true` with the exact
  row/session/current-lane/target identity: pending-row ID, session ID, task
  ID, move ID, workflow ID, current workflow-step ID, target workflow-step ID,
  and queue timestamp.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-004.2:** When the same Coordinator request is
  authorized but no pending-move row is armed for the target task, the system
  shall return `found: false` as an authoritative, non-error result proving the
  absence of a row. This response shall be distinguishable at the API from any
  authorization or relation denial; a null or omitted projection alone shall
  never stand in for this proof.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-004.3:** When the caller is not a live
  designated Coordinator, the target is unreachable through the Coordinator's
  named task tree, or the relation is cross-workspace or broken, the system
  shall make no distinction based on whether a row exists and shall return the
  same non-leaking not-found-or-changed result used by exact cancellation.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-004.4:** Missing or non-canonical public
  identifiers shall be rejected before target lookup with the same stable
  invalid-argument result used by exact cancellation.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-004.5:** The read shall never mutate
  pending-move, task, session, or queue state; a row read this way shall remain
  eligible for the exact cancellation using the identity the read returned,
  subject to that operation's own freshness and predicate checks.
- **AC-TASKS-PENDING-MOVE-CANCELLATION-004.6:** Every accepted, denied, or invalid
  read attempt shall emit structured evidence in the same evidence store as
  cancellation, distinguished by action, without prompts, credentials, or tool
  payload logging.

## Out of scope

- General-purpose pending-move deletion, bulk cancellation, or raw SQL access.
- Grant issuance, revocation, or role-management UI for workspace Coordinators.
- Support-principal authorization in the first release.
- Automatic TTL, orphan detection, queued-prompt cleanup, or reaper policy.
- Applying the operation to a production row before reviewed live acceptance.
- Listing or paginating pending moves across multiple tasks in one call; the
  census is scoped to exactly one target task per request.

## System design

- [Exact Pending-Move Cancellation System Design](../system-design/pending-move-exact-cancellation.md)
