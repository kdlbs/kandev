---
status: current
system: tasks
requirements:
  - REQ-TASKS-PLAN-COMMENTS-002
---

# Task Plan Comments System Design, Part 2: Undispatched Delivery Recovery

## Purpose and boundaries

This part refines the [receipt dispatch](plan-comments.md#receipt-dispatch)
boundary for every comment-bearing queue entry, which covers both plan
comments and web preview feedback. It changes only how a dispatch failure is
classified after the `delivery_attempted` marker. Admission, consumption,
replay, and the at-most-once rule for possibly-delivered prompts do not
change.

## Requirement mapping

| Criterion | Section |
| --- | --- |
| `AC-TASKS-PLAN-COMMENTS-002.9` | [Failures after the marker but before transport](#failures-after-the-marker-but-before-transport), [Send Now claims](#send-now-claims) |

## Failures after the marker but before transport

In practice the orchestrator sets the marker in the `beforeDispatch` hook
(`planCommentDeliveryBoundary`), and several steps that never touch agentctl
run after it: ceiling admission and `afterDispatchAdmission` in
`validateAndRunDispatchBoundary`, then in lifecycle `sendPrompt`
attachment materialization, `beforeAdmission`, the initial-delivery
persistence, prompt-generation admission, and the durable turn binding. If one
of these failed, the message was treated as possibly delivered, so the receipt
was acknowledged silently and the message was lost (issue #4964).

A failure in any of those steps now carries proof that nothing was sent
(`AC-TASKS-PLAN-COMMENTS-002.9`):

- `lifecycle.ErrPromptNotDispatched` is a sentinel. `sendPrompt` returns
  every error that occurs before `triggerPrompt` through
  `lifecycle.MarkPromptNotDispatched`. That wrapper keeps the cause's message
  text unchanged and still unwraps to the cause, so user-visible errors and
  `errors.Is`/`errors.As` matching do not change. The orchestrator marks its
  own post-boundary, pre-executor failures the same way: ceiling admission and
  `afterDispatchAdmission`, in both the ordinary path and the model-switch
  branch. A `triggerPrompt` error, `ErrUncertainPromptDelivery`, and any error
  after acceptance are never marked. Capacity refusal detection uses
  `errors.As`, so a marked refusal is still recognized.
- Before `finishQueuedMessageExecution` runs,
  `executeQueuedMessageWithReservation` checks for a comment-bearing entry
  with `deliveryAttempted` and a not-dispatched error. In that case it calls
  `requeueUndispatchedDelivery`, which does three things:
  - Calls the new `messagequeue.Service.ClearDeliveryAttemptForSession`, the
    exact reverse of `MarkDeliveryAttemptedForSession`. It runs in one
    transaction under the same task guard, session lock, identity validation,
    comment-bearing check, and reservation-token match. It removes only
    `delivery_attempted` and keeps the reservation, the recorded-transcript
    marker, `client_queue_id`, and the request fingerprint.
  - Returns the receipt to its FIFO position with the existing
    `ReleaseQueuedDeliveryForSession`, which is also used by passthrough
    delivery, and publishes queue status. The ordinary requeue path is not
    used, because comment receipts have no `queue_dispatch_claims` row and
    that path rejects them.
  - Sets `allowFollowupDrain = false`, so a persistent failure waits for the
    next prompt-ready event or the user's Send now instead of spinning.

  A retry does not write a second transcript, because the recorded marker
  survives.
- If clearing the marker fails, the entry keeps its previous at-most-once
  handling and an Error log names the queue entry.
- Every remaining `deliveryAttempted && err != nil` acknowledgement logs a
  Warn entry, `comment-bearing queued message completed after a possibly
  delivered dispatch failure`, with task, session, and queue IDs and the
  error. It never logs prompt or comment content.

If the backend crashes between clearing the marker and the release, the entry
is left as an expired pre-attempt reservation. Startup reconciliation already
resumes those, which is correct because nothing was sent.

## Send Now claims

Send Now marks every comment-bearing source of its combined claim in
`sendNowDeliveryBoundary`. When its prompt fails with a not-dispatched error,
`executeSendNowClaimWithContext` calls `revokeUndispatchedSendNow`. That
method clears all of the claim's comment-bearing receipts in one
`ClearDeliveryAttemptForSession` call, which takes a list and clears every
receipt or none. It then uses the existing `restore` path
(`RestoreSendNowClaim`), the same path a failure before the marker already
takes, and disables the follow-up drain. The combined transcript was written
before the marker, and the restored sources keep their recorded-transcript
markers, so a later delivery adds no second chat message. Possibly delivered
and accepted-dispatch failures still settle through
`settleAttemptedSendNowClaim`.

## Verification

- Lifecycle: every error `sendPrompt` returns before `triggerPrompt` satisfies
  `errors.Is(err, ErrPromptNotDispatched)` and still matches its cause.
  `triggerPrompt` failures and uncertain deliveries do not.
- Messagequeue: `ClearDeliveryAttemptForSession` rejects a stale identity, a
  non-comment entry, and an unreserved entry, on SQLite and on env-gated
  PostgreSQL.
- Orchestrator: an injected pre-transport failure leaves the entry queued
  with its client queue ID, fingerprint, and single transcript. A later drain
  delivers it once. A possibly-delivered failure is acknowledged and logged.
  A not-dispatched Send Now restores every source with one transcript; a
  possibly delivered one settles.

## Implementation plans

- [Web preview feedback and comment surface](../../../plans/web-preview-feedback-comment-surface/plan.md)
