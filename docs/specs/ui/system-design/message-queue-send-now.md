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

## Restored claims

A session can become promptable while its Send Now claim is still held: the
resume path publishes readiness before the claimed prompt fails. The readiness
drain finds nothing to deliver, and no later event repeats it. Restoring a claim
is therefore a delivery trigger. After `restoreSendNowClaimWithRetry` succeeds,
`executeSendNowClaimWithContext` calls `markQueuedDispatchDrainPending`, and its
existing deferred `drainQueuedDispatchIfPending` runs after the dispatch marker
clears. A failed restore does not mark a drain.

That drain is ordinary FIFO. It cannot re-enter Send Now, so a persistent failure
follows the existing FIFO disposition instead of looping. Only a Send Now executing
in the running backend marks a drain. A claim restored by startup recovery stays
pending, because a backend restart starts no agent.

## Claim ownership over leases

A durable source, such as a plan-comment or preview-feedback message, stays in
`queued_messages` while a Send Now claim owns it. The claim reserves the row
with a delivery lease of `DeliveryReservationTTL` (30 seconds). The
replacement prompt can wait longer than that: `agentPromptReadyTimeout` is
also 30 seconds and starts after the claim, so a stalled session always
outlives the lease. Before the ownership rule, an expired lease let the head
reservation treat the row as free. A readiness drain then reserved it with a
new token; the claim's later restore failed its token check with
`ErrSendNowClaimChanged`, and the FIFO owner later acknowledged the row without
delivering it.

The open claim, not the lease, owns its sources. `reserveHeadTx`
(`messagequeue/repository_sqlite.go`, which also serves PostgreSQL) reads the
session's `queue_send_now_claims` row in the same transaction. If the head
entry is one of that claim's sources, it returns no reservation, as it does for
a live plan-comment lease. It does not skip to the next entry: Send Now is
replacing the turn, and FIFO must not start a competing prompt. The memory
repository's `reserveHeadLocked` applies the same rule through the existing
`sendNowClaimContainsLocked`. The lease still governs ordinary FIFO
reservations that no claim owns.

The rule cannot strand a row: every claim ends in acknowledgement, restoration,
or the release described below. Startup recovery reconciles claims left by a
crash before any drain runs.

## Unrecoverable claims

A claim whose prompt failed and whose restore still returns
`ErrSendNowClaimChanged` after the retry budget cannot be put back. Three sites
reach that state: the running backend's restore in
`executeSendNowClaimWithContext`, `restorePendingSendNowClaimsForRecovery`, and
startup reconciliation without a recovery-block store. Today the first leaves
the claim row behind, and the other two delete it silently.

All three call one release step. It first verifies that the exact claim still
owns its sources; if a successor owns a surviving source, the step leaves the
message unmarked because that successor may deliver it. Otherwise the step
marks the claim's user message as not delivered while the claim is still
present, then releases the claim and every source row it owns, so FIFO cannot
later deliver work already reported as undelivered. When the message is
already in the history (a durable source recorded by `prepareSendNowTranscript`, or a
message recorded after the prompt claim), the step adds
`delivery_status: not_delivered` to its metadata. When it is not, the step
records it from the claim's dispatch envelope with that status in an
already-completed lifecycle-only turn. That keeps the release from starting or
adopting an active conversation turn, which would corrupt turn timing and
running indicators. The step then publishes the message update and queue
status. It is idempotent per claim, so a retry or a second recovery pass does
not duplicate it. Transient failures are retried with the bounded claim
mutation helper. A failure before the release transaction commits leaves the
claim in place for the next startup reconciliation or recovery action. A
persistent failure while writing the message status keeps the claim available
for retry and leaves the message unmarked, and the error is logged.

Claims invalidated by deleting or replacing the task or session are not
unrecoverable: restore already skips sources whose generation changed and
succeeds. They keep their existing behavior.

The orchestrator reaches messages through `MessageCreator`, which has no update
method. Add two narrow methods rather than passing the task service into queue
code: one sets the delivery status on an exact session-owned user message, and
one records an idempotent user message in a completed lifecycle-only turn.
`models.MessageMetaKeyDeliveryStatus` gains the
`MessageDeliveryStatusNotDelivered` value beside `blocked`.

## Presentation and compatibility

The Send Now transport request has a 35-second deadline: the existing
30-second backend cancellation budget plus the ordinary five-second transport
allowance. Pending cancellation must not produce a premature client timeout
or error toast. A lost acknowledgement still terminates at that deadline.

The queue hook, existing row control, cancellation projection, and queue refetch
remain shared across desktop and phone. Reuse the inline queue panel and its
single scroll region. Desktop retains hover/focus actions; coarse pointers
retain visible touch targets. No new control or layout is needed.

`queue-api.ts` already maps a real `WebSocketRequestError` into a typed queue
error on the current branch. Add a Send Now-specific regression using that
class, rather than reproducing the reporter's older `instanceof Error` bypass.
The existing `chat:sendNowConflict` translation remains the feedback for a real
handoff conflict.

A not-delivered message reuses the user-message status badge that
`chat-message.tsx` already renders for `delivery_status: blocked`, with new
localized copy (`task:deliveryMessageNotDelivered`) in every shipped locale. The
badge belongs to the message bubble, so desktop and phone share it; it adds no
control or layout.

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
| .13 | Repository tests on SQLite, memory, and env-gated PostgreSQL with an expired lease; an orchestrator test with a stalled replacement prompt and a readiness drain |
| .14 | Orchestrator tests for all three release sites; desktop and phone rendering of the not-delivered badge |

Implementation and exact commands live in the
[live FIFO repair package](../../../plans/queue-send-now-live-fifo/plan.md) and
the [claim loss repair package](../../../plans/send-now-claim-loss/plan.md).
