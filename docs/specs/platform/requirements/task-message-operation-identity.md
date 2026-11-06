---
status: draft
system: platform
created: 2026-10-04
owners:
  - kandev
---

# Task Message Operation Identity Requirements

## Overview

An agent that calls `message_task_kandev` can lose the tool response while the
delivery still commits. Platform owns the contract that makes such a send
determinable: a caller-supplied operation identity that is claimed durably
before dispatch, and an idempotent readback that reports the recorded outcome
without redispatching.

This is the agent-to-agent counterpart of the uncertain-delivery contract the
user composer already owns
([AC-PLATFORM-EXPLICIT-STEERING-002.2](explicit-turn-steering.md)); it
generalizes that principle rather than inventing a second one. It covers the
MCP send path only. It does not change queue ordering, admission limits,
delivery modes, or the transcript.

## Terminology

- **Operation identity:** A caller-generated opaque token naming one logical
  send. Chosen by the caller, unique per (sending task, sending session).
- **Operation record:** The durable row holding the identity, the
  delivery-determining request fields exactly as they were sent, and the send
  outcome.
- **Request comparison:** The replay check compares the replayed request's
  delivery-determining fields (target task, prompt, delivery mode, requested target
  session, parent-question identity) **directly** against the values stored on the claimed operation.
  The stored fields *are* the request; no digest, hash, checksum or signature
  is computed, stored or compared. The comparison answers one question only,
  whether this is the same send, and is not a content-integrity or
  tamper-detection control.
- **Claimed:** The operation record exists with state `pending`, before any
  delivery side effect is attempted.
- **Settled:** The operation record reached a terminal state (`committed` or
  `failed`) and carries the delivery outcome.

## Requirements

### REQ-PLATFORM-TASK-MSG-OPID-001: Identified send

**Intent:** A caller that loses a send response can name the send it lost, so
the outcome is a question with an answer instead of a guess.

**User story:** As an agent supervising another task, I want to name each send
I make, so that a lost tool response does not force me to either duplicate
work or abandon it.

#### Acceptance criteria

- **AC-PLATFORM-TASK-MSG-OPID-001.1:** `message_task_kandev` shall accept an
  optional caller-supplied operation identity, and shall echo it in the
  success response together with the identity of the delivered artifact
  (recorded message ID and/or queue-entry ID).
- **AC-PLATFORM-TASK-MSG-OPID-001.2:** When no operation identity is
  supplied, the send shall behave exactly as it does today, including its
  current non-idempotent retry semantics.
- **AC-PLATFORM-TASK-MSG-OPID-001.3:** The operation identity shall be claimed
  durably before the first delivery side effect, so that a response lost
  after any later point still leaves a readable record.

### REQ-PLATFORM-TASK-MSG-OPID-002: Determinable outcome

**Intent:** After an ambiguous send, the caller learns what happened instead of
retrying blind.

**User story:** As an agent, I want to read back the outcome of a send I timed
out on, so that I deliver the message exactly once.

#### Acceptance criteria

- **AC-PLATFORM-TASK-MSG-OPID-002.1:** The platform shall expose a readback
  that resolves an operation identity to its recorded state and delivery
  outcome, without performing any delivery side effect.
- **AC-PLATFORM-TASK-MSG-OPID-002.2:** A readback of a claimed but unsettled
  operation shall report it as still in flight, and shall explicitly state
  that the send must not be retried.
- **AC-PLATFORM-TASK-MSG-OPID-002.3:** A readback of an operation that has no
  record shall report retained-record absence. Because retention is bounded,
  absence shall not authorize an automatic resend.
- **AC-PLATFORM-TASK-MSG-OPID-002.4:** The recorded outcome shall survive a
  backend restart, and shall distinguish accepted dispatch (including queued or started)
  from an unknown delivery outcome; acceptance does not prove a provider reply.

### REQ-PLATFORM-TASK-MSG-OPID-003: Once-only replay

**Intent:** Retrying after an ambiguous send is safe, because a replay is a
read, not a second delivery.

**User story:** As an agent, I want to retry my send with the same identity
after a timeout, so that the retry is harmless whether or not the first
attempt committed.

#### Acceptance criteria

- **AC-PLATFORM-TASK-MSG-OPID-003.1:** A replay with a known operation
  identity shall return the recorded outcome and shall not create a second
  message, a second queue entry, or a second turn.
- **AC-PLATFORM-TASK-MSG-OPID-003.2:** A replay shall not re-issue an
  interrupt; an operation recorded as interrupt-delivered shall never cancel
  a later turn of the target session.
- **AC-PLATFORM-TASK-MSG-OPID-003.3:** A replay whose delivery-determining
  request fields (target task, prompt, delivery mode, requested target session,
  parent-question identity) differ
  from the values stored on the claimed operation shall be rejected with a
  distinct, stable error code, and shall deliver nothing. The check shall
  compare those field values directly; it shall not compute, store or compare
  a digest, hash or signature of them.
- **AC-PLATFORM-TASK-MSG-OPID-003.4:** Where a pre-existing claim mechanism
  already makes a narrower send idempotent (a parent-question reply), the
  operation identity shall compose with it rather than bypassing or
  double-claiming it.

### REQ-PLATFORM-TASK-MSG-OPID-004: Bounded ownership and reach

**Intent:** The record is readable by its owner, does not leak, and does not
grow without bound.

**User story:** As an operator, I want send records scoped to their sender and
pruned opportunistically, so that the mechanism cannot leak other agents' traffic
or consume storage indefinitely.

#### Acceptance criteria

- **AC-PLATFORM-TASK-MSG-OPID-004.1:** Only the trusted kanban task/session principal that claimed
  an operation may read it or replay it; a readback naming another session's
  operation shall be denied without disclosing whether it exists.
- **AC-PLATFORM-TASK-MSG-OPID-004.2:** Operation records shall be retained for
  a bounded period and removed in bounded batches under existing maintenance
  admission; pruning shall never block or fail a live send.
- **AC-PLATFORM-TASK-MSG-OPID-004.3:** The operation identity shall not be
  usable as a covert channel: a rejected replay shall not echo the stored
  prompt or any other stored content.
- **AC-PLATFORM-TASK-MSG-OPID-004.4:** Observability shall distinguish a first
  claim from a replayed claim and a rejected replay, by closed-set reason
  only, with no task, session, prompt or operation identity used as a metric
  label.

## Out of scope

- Changing queue ordering, admission ceilings, or delivery-mode semantics.
- Removing or shortening the multi-minute launch budget.
- Retrying, cancelling or deduplicating on the caller's behalf.
- The user-composer send path, which already owns its uncertain-delivery
  contract under `explicit-turn-steering`.
- Cross-workspace visibility rules and collaboration grants for agent messaging.
- Agent-native subagent delegation, which does not route through this tool.

## Related contracts

- [System design](../system-design/task-message-operation-identity.md)
- [Explicit Same-Turn Steering](explicit-turn-steering.md)
