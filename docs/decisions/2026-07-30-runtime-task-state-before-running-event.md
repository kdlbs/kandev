# ADR-2026-07-30-runtime-task-state-before-running-event: Publish Task State Before Running Session State

**Status:** accepted
**Date:** 2026-07-30
**Area:** backend, frontend, protocol, workflow

## Context

Task surfaces intentionally use different lifecycle facts: workflow position,
persisted `tasks.state`, and live `task_sessions.state`. During runtime start,
the orchestrator persisted and published `RUNNING` before reconciling the task
to `IN_PROGRESS`. A client could therefore render a truthful running indicator
inside a stale `Review` state group until the later task event arrived.

## Decision

When an owning non-Office session changes to `RUNNING`, the orchestrator
reconciles `tasks.state` through the existing session-state-guarded task update
before publishing `session.state_changed(RUNNING)`.

The reconciliation runs only after the session compare-and-set succeeds and
before its event publication. If it changes the task, the existing
`task.state_changed` event is therefore published first. The follow-on session
event is appended to the same per-task publication FIFO, so an already-draining
task publication cannot be overtaken and the handler never waits reentrantly on
its own queue. Repeated stream events for an already-`RUNNING` session retain the
existing no-write fast path.
Archive, terminal-session, clarification, cancellation, and Office guards
remain authoritative. The executor-success reconciliation remains as a healing
fallback.

The WebSocket gateway subscribes to both lifecycle subjects through one
ordered NATS-style wildcard subscription and resolves the WebSocket action
from the event type. This preserves the producer's ordering contract across a
remote event bus, where separate subscriptions could otherwise race at the
gateway.

## Consequences

Clients can continue treating persisted task state as authoritative for State
grouping without synthesizing task state from session events. Desktop and
mobile task surfaces converge through their existing shared store and row
rendering.

The task event may now precede the running session event by a small interval,
so a client may briefly show an `IN_PROGRESS` task without a spinner. That is
preferable to claiming active runtime inside `Review`, and the following
session event completes the display. A task-state persistence error cannot hide
a truly running session; the error is logged and the later executor
reconciliation may heal it.

## Alternatives Considered

- **Group the sidebar by live session activity.** Rejected because Group by
  State deliberately exposes persisted task states such as `REVIEW`,
  `COMPLETED`, and `FAILED`; collapsing those into activity buckets changes the
  feature's meaning.
- **Patch `tasks.state` optimistically in the frontend on every running session
  event.** Rejected because the frontend lacks the backend's Office, archive,
  sibling-session, and race guards and would create a second task-state owner.
- **Publish the session event first and rely on eventual task reconciliation.**
  Rejected because it is the observed inconsistency and makes correct UI depend
  on event latency.
- **Keep separate gateway subscriptions for the two lifecycle subjects.**
  Rejected because NATS delivers each subscription independently; their
  callbacks can race and reverse the ordered events before WebSocket delivery.
- **Remove the no-write fast path and reconcile on every stream event.**
  Rejected because long turns generate thousands of duplicate events; the fix
  can preserve deduplication by reconciling only inside the successful
  session-transition hook.
