# ADR-2026-08-03-separate-message-queue-provenance-cancellation-and-capacity: Separate Message Queue Provenance, Cancellation, and Capacity

**Status:** accepted
**Date:** 2026-08-03
**Area:** backend, frontend, protocol, security

## Context

Queued messages carry a `queued_by` identity that records their provenance:
user, agent, workflow, or server. ADR 0051 also used that provenance as a
client-mutation boundary, allowing clients to remove only user-owned rows.
That coupling made an authorized task owner unable to discard visible work
sent by another task. The queue panel still offered **Clear all**, but the
backend silently retained every agent-, workflow-, and server-owned row. A
queue containing ten inter-task messages therefore stayed full after the
user's explicit clear action.

The per-session capacity is also an operational policy. It currently comes
only from `KANDEV_QUEUE_MAX_PER_SESSION`, so changing it requires access to the
launch environment and a backend restart. Users need an install-wide setting
without weakening environment-controlled deployments or risking already
accepted delivery work when the limit is lowered.

## Decision

Treat provenance, content mutation, cancellation, and capacity as separate
concerns.

- `queued_by` remains immutable provenance. It continues to govern content
  editing and the existing merge rules; it does not grant a pending message an
  exemption from an authorized user's explicit discard action.
- A caller authorized for the target session may remove any **visible pending**
  queue entry, regardless of whether it came from a user, agent, workflow, or
  server. **Clear all** removes every visible pending message in that session.
- A durable lifecycle row marked `lifecycle_reserved_in_flight` is already in
  the delivery protocol, is omitted from queue status, and is not cancellable
  through either user action. Cancellation and reservation serialize per
  session and use persisted compare guards so whichever operation wins does so
  completely. Backend archive/delete purge remains the privileged operation
  that may remove reserved rows and advance lifecycle generation.
- Every user-facing WebSocket queue operation authorizes the named session in
  its handler. Session scoping remains mandatory on every row mutation, so a
  disclosed or guessed entry UUID cannot mutate another session.
- Queue capacity is an install-wide setting exposed under **Settings > General
  > Message Queue**. The effective value uses this precedence:

  ```text
  valid KANDEV_QUEUE_MAX_PER_SESSION > persisted install setting > default 10
  ```

  A positive integer is the maximum number of persisted messages per session;
  `0` means unlimited. For compatibility, a non-positive environment value is
  normalized to `0`. An invalid environment value is logged and ignored so a
  valid persisted value or the default can apply.
- A valid environment value locks the UI control. Otherwise an admin may save
  a non-negative integer to the install-wide `settings` store. The saved value
  applies live through a concurrency-safe queue-service setter; no restart is
  required.
- Changing the limit never deletes existing rows. Lowering it below a current
  queue size blocks new admissions until the count falls below the limit.
  Restoration and retry of work accepted before a dequeue bypass the admission
  cap, so a failed delivery cannot lose work merely because the setting was
  lowered.

This decision supersedes ADR 0051 only where that ADR prohibited an authorized
client from deleting agent-, workflow-, or server-owned **pending** rows. ADR
0051's provenance, durable reservation, acknowledgement, archive-generation,
and delivery guarantees remain in force.

## Consequences

- **Clear all** matches its visible promise and can recover a queue filled by
  inter-task or automation messages.
- Individual removal works consistently for every pending row while editing
  and merging retain stricter provenance rules.
- A reservation that wins the race may still be delivered, but hidden
  in-flight work cannot be silently deleted between executor handoff and
  acknowledgement.
- Operators can tune queue pressure without rebuilding launch configuration;
  managed deployments keep environment authority.
- The queue service's cap becomes runtime mutable and all admission paths must
  read one atomic snapshot per operation.
- Queue-setting reads are available to signed-in users; mutations are
  admin-only. Authentication-disabled installs retain their synthetic-admin
  behavior.

## Alternatives Considered

1. **Keep backend-origin rows undeletable and relabel the button.** Rejected
   because it leaves users unable to recover a queue filled by stale inter-task
   messages.
2. **Clear agent rows but preserve workflow/server rows.** Rejected because
   **Clear all** would still have origin-dependent hidden exceptions and the
   user owns the target session, not the sender's provenance.
3. **Delete reserved in-flight lifecycle rows too.** Rejected because those
   rows participate in at-least-once handoff and acknowledgement; deletion can
   corrupt delivery state after work has already reached the executor.
4. **Require restart after a capacity change.** Rejected because the cap is an
   admission-time integer and can be updated safely without reconstructing the
   orchestrator.
5. **Make capacity per workspace or per session.** Rejected for this change.
   The existing policy is install-wide, and finer scopes add inheritance and
   override semantics without evidence they are needed.
