# ADR-2026-08-05-queue-send-now-replaces-turn: Queue Send Now Replaces the Active Turn

**Status:** accepted
**Date:** 2026-08-05
**Area:** backend, frontend, protocol, workflow

Related decision:
[Keep Queue Auto-run Server Owned](2026-08-16-server-owned-queue-auto-run.md)
defines an accepted Send Now claim as an explicit queue resume while retaining
this decision's replacement-turn guarantees.

## Context

The queue panel needs a **Send Now** action that interrupts a busy agent and
starts a replacement turn from one selected queued message or an aggregate of
the current queue. Kandev's ordinary user Cancel action may intentionally run a
workflow step's `on_turn_complete` policy, move the task to review, or auto-start
the destination step. Reusing that path would let a request to continue working
change workflow position before its replacement prompt is dispatched.

## Decision

`message.queue.send_now` is a turn-replacement operation, not an explicit
workflow-completing cancellation.

- The orchestrator owns the whole cancel-and-dispatch operation under the
  existing per-session cancellation coordinator and turn-identity guard.
- The operation gets a distinct cancellation kind. It publishes the existing
  backend-owned cancellation progress, but it does not create the ordinary
  **Turn cancelled** message, move the task to review, or evaluate
  `cancel_triggers_turn_complete` / `on_turn_complete`.
- A promptable session dispatches the claimed queue selection without issuing a
  cancel. A busy session cancels only when the active turn is confirmed to be
  the same turn observed when the operation began; a successor turn is never
  cancelled as a stale continuation of the request.
- Queue selection and dispatch remain one orchestrator operation. A selected
  entry or click-time bulk snapshot is claimed only for this replacement turn,
  and failure handling restores the original pending entries rather than
  silently substituting another queue entry.
- An already accepted explicit user cancellation or another send-now operation
  wins the conflict. Send Now fails closed instead of joining a
  workflow-completing cancellation and dispatching after it.

## Consequences

Users can redirect a long-running turn without moving the task out of its
current workflow step. The action shares cancellation progress and race safety
with the existing coordinator while preserving a separate product meaning from
the Cancel button.

The backend needs a new WebSocket action, queue claim/aggregation behavior, and
tests for cancellation-source races, successor-turn protection, restoration,
and mixed-provenance bulk delivery. The frontend must treat Send Now as an
asynchronous cancellation operation and disable duplicate actions while the
backend projection is pending.

## Alternatives Considered

- **Call the existing explicit `CancelAgent` action, then drain the queue.**
  Rejected because configured user cancellation can complete a workflow step,
  move the task, and auto-start unrelated successor work before the queued
  prompt is sent.
- **Queue a copy of the selected content and use the parent-message interrupt
  endpoint.** Rejected because it duplicates a durable row, loses the selected
  entry's identity and attachments, and exposes the wrong authorization model.
- **Cancel first and let the ordinary FIFO drain choose the next message.**
  Rejected because per-row Send Now must dispatch the selected entry, while
  bulk Send Now must use the click-time queue snapshot rather than whichever
  row happens to be at the head after a race.
