# ADR-2026-08-16-server-owned-queue-auto-run: Keep Queue Auto-run Server Owned

**Status:** accepted
**Date:** 2026-08-16
**Area:** backend, frontend, protocol, workflow

## Context

The pending-message queue automatically takes another FIFO entry whenever an
agent becomes ready. The existing **Run next** control directly dispatches one
head entry, but the same readiness path normally continues through every later
entry. Its one-message label therefore describes an implementation step rather
than the user's outcome.

The only existing way to park a backlog is explicit Cancel. That operation also
stops the active response and may complete a workflow step under
`cancel_triggers_turn_complete`. Users need a separate instruction that lets the
current response finish but prevents the next queued turn from starting.

A browser-local switch cannot enforce that instruction. Readiness, workflow,
lifecycle, CI automation, and other backend paths can reserve queued work even
when the controlling browser is closed, and multiple browser or backend
instances may race over the same session.

## Decision

Make auto-run a durable, backend-owned policy for each task session.

- The message-queue domain stores an `auto_run` boolean in a dedicated
  `queue_session_state` table. A missing row means `true`, preserving existing
  behavior for every current and newly created session. An explicit value
  survives an empty queue, browser reload, navigation, and backend restart.
- Queue status includes `auto_run`. The WebSocket action
  `message.queue.auto_run.set` sets it for an authorized session. Setting it to
  `true` immediately attempts the FIFO head when the session is promptable;
  otherwise it arms the next eligible backend drain. Setting it to `false`
  never cancels or truncates the active turn.
- Every automatic FIFO reservation, regardless of message provenance or
  trigger source, checks `auto_run` in the same repository transaction that
  reserves the head. The policy read and queue mutation serialize through the
  existing `queue_session_locks` row. If OFF commits before a successor is
  reserved, no successor starts. If a reservation commits first, that message
  may become the current turn and OFF applies to every later entry.
- **Send Now** remains an exact-entry turn replacement. A successful atomic
  claim also sets `auto_run=true`, so the selected entry runs first and the
  preserved FIFO remainder continues one turn at a time. Validation,
  cancellation, or claim failures leave the previous policy unchanged. Once a
  claim is accepted, a later asynchronous prompt restoration leaves auto-run
  enabled because the accepted operation was also an explicit resume request.
  The backward-compatible all-entry scope follows the same resume rule.
- A successful legacy `message.queue.drain` request also enables auto-run. Its
  response shape and one-head immediate dispatch remain compatible.
- Explicit user Cancel keeps its existing immediate-stop and workflow
  semantics. When it leaves pending queue entries, it persists `auto_run=false`
  before releasing the cancellation guard. Internal cancellations and the
  special Send Now replacement cancellation do not pause auto-run.
- Workflow session transfer moves the policy with the queue. The destination
  is enabled only when both source and destination were enabled, so a paused
  backlog cannot resume merely because its session identity changed. The
  source policy row is removed after transfer.
- Queue snapshots used only to roll back message mutations do not overwrite
  auto-run. Policy changes are independent user intent, not message content.

The first-party queue panel exposes the policy as one labeled **Auto-run**
switch. It removes header **Run next**, header bulk **Send Now**, and any
proposed **Skip to next** action. Every visible row, including the FIFO head,
retains targeted **Send Now**.

## Consequences

- Users can finish the active response and hold the remaining backlog without
  invoking workflow-completing cancellation.
- Queue behavior is consistent across clients, restarts, automatic producers,
  and workflow session changes.
- Queue reservation gains a policy-aware variant, and every automatic drain
  call site must use it. Targeted internal delivery paths remain explicit and
  cannot accidentally inherit the policy check.
- The queue status projection and frontend session slice gain one boolean, and
  status-change events synchronize the switch across connected views.
- A queue can remain pending while Auto-run is ON when an independent guard,
  such as clarification, workflow transition, cancellation, or prompt
  admission, temporarily blocks delivery. ON means “run when eligible,” not
  “bypass lifecycle safeguards.”

## Alternatives Considered

1. **Keep Run next as a one-shot action.** Rejected because later readiness
   already continues the queue, so the label obscures the actual behavior and
   provides no durable pause instruction.
2. **Add a client-local switch.** Rejected because backend drains continue
   without that client and other clients would display conflicting state.
3. **Store the policy in task-session metadata.** Rejected because automatic
   queue reservation could not atomically read that separate ownership domain
   while holding the queue's cross-process session lock.
4. **Store the policy on `queue_session_locks`.** Rejected because the lock row
   is synchronization infrastructure. A separate state table keeps product
   policy explicit while still using the lock to serialize changes.
5. **Reuse Cancel as pause.** Rejected because Cancel stops the current response
   and can advance the workflow, while Auto-run OFF deliberately does neither.
6. **Reset OFF when the queue becomes empty or the process restarts.** Rejected
   because it would silently discard the user's instruction and let later
   automation restart the queue.
