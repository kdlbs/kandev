---
status: current
system: ui
created: 2026-09-15
requirements:
  - REQ-UI-MESSAGE-QUEUE-SEND-NOW-001
owners:
  - kandev
---

# Send Now and queued turn ownership

## Boundary

This design accompanies the existing [Send Now requirement](../requirements/message-queue-send-now.md).
It retains that capability's current specification owner during migration.
The task orchestrator owns runtime arbitration; agent profile selection remains
with the agent runtime. No second requirement is introduced in either system.

The [replacement-turn decision](../../../decisions/2026-08-05-queue-send-now-replaces-turn.md)
and the [queue automation design](message-queue-automation-controls.md) remain
applicable. Session identity, policy transactions, restoration generations, and
explicit Cancel semantics are unchanged.

## Components and phases

`internal/orchestrator/queued_dispatch.go` owns `queuedDispatchReservation`.
`queue_send_now.go` captures the active turn, arbitrates cancellation, claims
an exact queue selection, and starts the replacement worker.
`task_operations.go` owns the guarded prompt claim and its visible side effects.

| Phase | Send Now arbitration | Settlement ownership |
| --- | --- | --- |
| `queuedDispatchPending` | May restore and supersede the unaccepted FIFO source | Exact reservation |
| `queuedDispatchAccepted` | Conflict while the prompt handoff is incomplete | Exact reservation |
| `queuedDispatchLive` | A new request may cancel its captured active turn | Exact reservation and bound successor turn |
| Superseded or settled | Cannot dispatch, restore, or clear a newer owner | Existing identity and generation checks |

The accepted record has two responsibilities: short handoff exclusion and
longer settlement ownership. Keep the record after it becomes live. Deleting
it to enable interruption would also discard protection against late events.

## Transition to live

Use the existing guarded boundary at the end of `claimSessionRunningForPrompt`:
revalidate the session incarnation and promptability, claim the session and
turn, perform `acknowledgePromptClaim`, then promote the exact current accepted
reservation through `markAcceptedDispatchLiveLocked`. Release the pending
marker only after these operations succeed.

Apply that transition to ordinary FIFO and Send Now reservations. The former
`liveEligible` distinction restricted the live phase to Send Now reservations;
the implementation removes that origin-specific gate and its redundant
field/setter. Do not promote on a timer, the browser's RUNNING projection, an
unrelated stream event, or a provider name.

Here, live means ownership of the guarded prompt claim, including its visible
side effects. It does not mean the long-running executor call has returned.
Keep the existing dispatch/cancellation serialization around provider admission.
A cancellation between claim and provider I/O must not let a superseded worker
send after its replacement. Exercise that ordering in a channel-controlled test.

The transition must compare reservation identity. A late worker cannot promote
or release a newer reservation. Continue binding successor turns through the
existing prompt ownership and acceptance paths. An unavailable or changed
session/turn identity fails closed through existing validation.

## Interruption and recovery

`sendQueuedNowAfterCapture` still rejects an active cancellation or an accepted
handoff. A live FIFO reservation proceeds to captured-turn validation and
`dispatchSendNowSelection`, just as a live Send Now reservation does today.

Cancellation uses `cancellationKindQueueSendNow`. The exact selected pending
entry, or all-scope snapshot, is claimed only after cancellation settles.
An already accepted input is never restored merely because its turn was
interrupted. Ordinary and durable entries retain their existing admission,
acknowledgement, generation, and retry rules.

Keep `acceptedDispatchInFlight`, successor-turn binding, and exact-reservation
cleanup effective for live FIFO turns. Completion from an interrupted
predecessor cannot clear the replacement, start a competing FIFO drain, or
complete the workflow step. A request captured before a successor appeared
fails with the existing conflict or turn-changed response, as appropriate to
its arbitration point.

No schema, wire payload, feature toggle, provider selection, or new cancellation
path is required. A dynamic session uses its existing concrete execution for
cancellation and follows the existing runtime contract for the replacement.

## Presentation and compatibility

The queue hook, existing row control, cancellation projection, and queue refetch
remain shared across desktop and phone. Reuse the inline queue panel and its
single scroll region. Desktop retains hover/focus actions; coarse pointers
retain visible touch targets. No new control or layout is needed.

`queue-api.ts` already maps a real `WebSocketRequestError` into a typed queue
error on the current branch. Add a Send Now-specific regression using that
class, rather than reproducing the reporter's older `instanceof Error` bypass.
The existing `chat:sendNowConflict` translation remains the feedback for a real
handoff conflict. No new localized copy is planned.

Clients that previously received a conflict for the entire FIFO turn can now
successfully interrupt it after the guarded claim. The response shape, genuine
conflict handling, Auto-run policy, and queue ordering remain compatible.

## Rationale

Protecting the whole FIFO turn prevents users from steering long-running work
solely because its input came from the queue. Removing all exclusion would
risk duplicate admission during handoff. The shared accepted-to-live boundary
supports interruption while preserving the existing ownership controls.
This refines the existing replacement-turn decision; no independent ADR is
needed because this design records the narrow alternatives and constraint.

## Verification mapping

| Acceptance criteria | Evidence |
| --- | --- |
| .2, .7, .9 | Real-service FIFO drain followed by exact-entry/all-scope Send Now; cancellation and exactly one replacement |
| .8, .10 | Barriers during accepted handoff; stale capture, stale completion, overlapping cancel, and superseded-worker tests |
| .9 | Fixed-profile and persisted dynamic-to-Cursor identity cases; no profile re-selection added by queue code |
| .1, .11 | Desktop click and phone tap through the existing queue panel, plus real WebSocket error mapping |

Implementation and exact commands live in the
[repair package](../../../plans/queue-send-now-live-fifo/plan.md).
