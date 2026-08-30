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
  target task/session/workflow, and workspace-owner identity are all live and
  consistent in the cancellation transaction. Coordinator-grant persistence
  shall also reject empty identifiers and make a cross-workspace task/grant
  pair unrepresentable.
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

## Out of scope

- General-purpose pending-move deletion, bulk cancellation, or raw SQL access.
- Grant issuance, revocation, or role-management UI for workspace Coordinators.
- Support-principal authorization in the first release.
- Automatic TTL, orphan detection, queued-prompt cleanup, or reaper policy.
- Applying the operation to a production row before reviewed live acceptance.

## System design

- [Exact Pending-Move Cancellation System Design](../system-design/pending-move-exact-cancellation.md)
