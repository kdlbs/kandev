# ADR-2026-08-03-backend-owned-cancellation-progress: Keep Cancellation Progress Backend Owned

**Status:** accepted
**Date:** 2026-08-03
**Area:** backend, frontend, protocol

## Context

An accepted turn cancellation is a backend operation that can outlive the React component,
application route, WebSocket connection, or browser page that initiated it. A frontend-only pending
flag can preserve progress across component remounts, but it disappears on reload, is invisible to
other clients, and can disagree with the orchestrator's existing per-session cancellation guard.
At the same time, the durable `TaskSession.state=RUNNING` value controls prompt admission and
workflow behavior, so cancellation progress cannot safely become a new coarse lifecycle state.

## Decision

The orchestrator owns a session-scoped `cancellation_pending` runtime projection. It is derived from
the existing `cancelOperations` registry and exposed through a narrow
`CancellationPending(sessionID string) bool` provider plus an atomic
`CancellationPendingSnapshot(sessionID string) (bool, uint64)` form for serialization. The first
accepted cancellation reference sets the projection; the last reference clears it. A process-local
per-session revision increments on each 0→1 or 1→0 transition. The count, revision, and ordered
publication queue are mutated in one critical section; event-bus sends drain outside that section.
The existing per-session guard remains the owner of deduplicating lifecycle cancellation work.

After session authorization succeeds, `CancelAgent` continues the accepted operation independently
of cancellation of the initiating WebSocket request, while retaining the lifecycle manager's
existing bounded cancellation and escalation timeouts.

The backend publishes first-begin and last-end transitions as the semantic event
`task_session.cancellation_changed`, mapped to the session-scoped WebSocket action
`session.cancellation_changed`. Each event and every full/summary session DTO, task-detail boot
hydration, REST session read, and initial `session.state_changed` subscription snapshot carries both
the explicit `cancellation_pending` boolean and its `cancellation_revision`. The frontend rejects a
snapshot or live value with a lower revision than the session's current value, so delayed REST/boot
responses cannot restore an older boolean after a newer live transition. Missed live notifications
are therefore repaired by the next authoritative snapshot. In accordance with
[ADR-2026-08-01-separate-task-summary-session-stream-traffic](2026-08-01-separate-task-summary-session-stream-traffic.md),
the operation state stays on the opened session stream and is not copied into task summaries.

The projection remains orthogonal to `TaskSession.state` and is not stored in `task_sessions` or
session metadata. A backend restart clears it; startup session/execution recovery remains
authoritative, and a session still reported as `RUNNING` may be cancelled again. The frontend may
show a bounded optimistic flag before backend acceptance, but backend hydration and live updates
own progress after that boundary and across page lifecycles.

## Consequences

Task switches, reloads, replacement tabs, and second clients render the same active cancellation as
long as the backend operation is alive. Closing the initiating page no longer aborts an accepted
cancellation. The session lifecycle and prompt-admission rules stay unchanged, and no schema
migration or restart reconciliation for a synthetic operation marker is required.

The API and WebSocket session contracts gain one explicit boolean, one process-local revision, and
one semantic notification. Every cancellation exit path must clear the reference count, and
serialization tests must cover both `true` and explicit `false` plus revision ordering so stale
client state cannot survive hydration. A backend restart does not claim that an unrecoverable
process operation is still pending; users may see the cancel control become retryable when the
recovered coarse session remains running.

## Alternatives Considered

- **Keep cancellation progress only in the frontend application store.** Rejected because it fixes
  route remounts but not reloads, replacement tabs, second clients, or disagreement with the real
  backend operation.
- **Persist a cancellation marker in `task_sessions` or session metadata.** Rejected because the
  process cancellation has no durable continuation token. A backend crash could strand a stale
  marker that claims work is pending when no operation can finish it.
- **Add `CANCELLING` to `TaskSession.state`.** Rejected because the coarse state participates in
  prompt admission, task projection, workflow transitions, and recovery. Cancellation is a
  temporary operation overlay, not a new durable lifecycle phase.
- **Persist the flag in browser storage.** Rejected because it would remain client-owned, diverge
  across tabs, and risk showing stale progress after the backend operation settled.
- **Order snapshots with database timestamps or transport arrival time.** Rejected because the
  cancellation projection is runtime-only: session `updated_at` does not change for every runtime
  transition, and independent REST/WebSocket delivery can arrive in either order. A process-local
  per-session revision is the smallest identity that orders the boolean without pretending the
  operation is durable.
