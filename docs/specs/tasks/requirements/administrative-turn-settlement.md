---
status: active
system: tasks
created: 2026-09-14
owners:
  - kandev
---

# Administrative turn settlement Requirements

## Overview

A coordinating agent must be able to release one finished workflow-turn
identity when its provider never sends the terminal lifecycle event, without
cancelling the session, interrupting a real successor, or rolling a task
backward. The task system owns this contract because it owns the durable
turn, completion-intent, and queue-admission records that prove which exact
turn finished and what its successor is.

The unrelated broad halt remains [Parent-Child Task Stop](parent-child-task-stop.md);
this capability adds one narrow, evidence-gated alternative. Durable
cross-task reporting that survives a busy target is owned by
[Recoverable cross-task delivery](recoverable-cross-task-delivery.md).

## Terminology

- **Completion intent:** One durable row per (session, turn, workflow step)
  recording that the agent emitted an accepted explicit completion signal.
- **Stale administrative turn:** A running-session turn whose completion
  intent is eligible, whose quiet grace has passed, and for which no
  successor, activity, reservation, or cancellation evidence exists.
- **Settlement:** Closing one exact durable turn identity as completed
  without touching its session, worktree, history, queues, or siblings.

## Requirements

### REQ-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001: Exact stale-turn settlement

**Intent:** Release one proven-stale administrative turn while preserving the
session and every other durable artifact.

#### Acceptance criteria

- **AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.1:** The task MCP catalog
  shall expose `settle_stale_session_kandev` with the exact target session
  and turn identity, returning `settled`, `already_settled`, or
  `active_turn`/`not_stale`, while `stop_task_kandev` remains the separate
  broad direct-child halt.
- **AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.2:** Settlement authority
  shall be limited to a same-workspace peer session on the target task, the
  target task's direct parent, or the server-recorded spawn supervisor of the
  target session; relation alone shall never be sufficient without terminal
  evidence and absence of active-ownership evidence.
- **AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.3:** An authorized request
  against an active or ambiguous turn shall cause no mutation and shall
  record an audit event whose result is `not_stale`.
- **AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.4:** A pure session
  materialization (a CREATED row, empty conversation, no turn or dispatch
  evidence) shall never satisfy settlement; refusal shall interpret absence
  as an unattributed lifecycle anomaly, never as a launch-authority claim.
- **AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-001.5:** Settlement shall commit
  the turn's terminal transition, its completion-intent state, and the
  authorized audit event atomically, so a settled turn never exists without
  its audit and vice versa.

### REQ-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002: Settlement preserves successor flow

**Intent:** A settled prior completion must not corrupt delivery into the
current workflow transition or resurrect an old step.

#### Acceptance criteria

- **AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002.1:** Later foreground or tool
  activity, clarification interaction, user work, a generation mismatch, or a
  successor turn shall prevent automatic settlement of an older captured
  identity.
- **AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002.2:** When the task moved
  before reconciliation, settlement shall mark the old intent superseded and
  ensure only the current transition's on-entry delivery runs.
- **AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002.3:** Repeated on-entry
  processing for one committed task-step transition shall create at most one
  real successor prompt.
- **AC-TASKS-ADMINISTRATIVE-TURN-SETTLEMENT-002.4:** Settlement shall not
  re-evaluate an old workflow step, modify the task's worktree or history,
  or cancel queued messages.

## Out of scope

- Reviving terminal sessions, resetting context, or broadening
  parent-child stop authority.
- Automatic settlement of general long-running turns without an eligible
  completion intent.
- Cancelling sessions: `stop_task_kandev` keeps that separate contract.
