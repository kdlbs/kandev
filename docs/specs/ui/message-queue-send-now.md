---
status: shipped
created: 2026-08-05
owner: kandev
---

# Send Queued Messages Now

Decision:
[ADR-2026-08-05-queue-send-now-replaces-turn](../../decisions/2026-08-05-queue-send-now-replaces-turn.md)

## Why

When an agent is in a long-running turn, an urgent correction can sit in the
queue until that turn finishes. Users need to replace the current turn with one
specific queued instruction, or with the whole pending queue as one coherent
follow-up, without completing or advancing the task's workflow step.

## What

- Every visible pending queue row offers **Send Now**. On a fine-pointer
  desktop it appears with the row actions on hover or keyboard focus; on a
  coarse-pointer surface it is always visible and touch-sized.
- Per-row **Send Now** interrupts the active agent turn and dispatches that
  exact entry as the first prompt of a replacement turn. Other queued entries
  remain pending in their existing FIFO order.
- The queue header offers **Send Now** immediately to the left of **Clear all**.
  It dispatches the click-time snapshot of all visible pending entries as one
  replacement turn.
- Bulk content is concatenated in FIFO order with one blank line between
  non-empty message bodies. Attachment-only entries add no empty separators.
  Attachments retain FIFO order, and entity references are combined in first-
  occurrence order with canonical duplicates removed.
- The bulk turn uses the oldest selected entry's model and plan-mode snapshot.
  Its transcript envelope uses that entry's sender attribution while retaining
  source entry identities and provenance in metadata for restoration and
  diagnostics.
- If the aggregate would exceed the existing per-message attachment count,
  attachment byte, or entity-reference limits, the request is rejected before
  cancellation. No content is truncated and the queue is unchanged.
- A promptable session dispatches immediately without cancellation. A busy
  session uses the existing backend-owned cancellation progress, cancels only
  the turn observed when the action began, and starts the replacement turn
  after cancellation settles.
- If ordinary FIFO delivery has reserved a queued entry but has not yet
  accepted its prompt, Send Now wins that same-session handoff. The reserved
  source is restored before the requested selection is claimed, so an
  all-scope replacement can include it in one aggregate prompt. Once FIFO has
  accepted its prompt, Send Now fails closed with `send_now_conflict`; it does
  not duplicate or cancel that successor turn.
- Send Now is a replacement/steering cancellation. It does not create the
  ordinary **Turn cancelled** message, move the task to review, evaluate
  `cancel_triggers_turn_complete`, or run the cancelled turn's
  `on_turn_complete` actions.
- The action is disabled while its request or any backend cancellation for the
  session is pending. The backend also rejects overlapping Send Now or explicit
  cancellation operations so rapid clicks and multiple clients cannot create
  successor turns.
- Successful dispatch publishes the existing queue-status and session
  cancellation updates. The initiating client refetches the authoritative queue
  after success or failure.

## API Surface

New WebSocket action:

```text
message.queue.send_now
```

Request:

```json
{
  "session_id": "session-id",
  "scope": "entry",
  "entry_id": "queue-entry-id"
}
```

or:

```json
{
  "session_id": "session-id",
  "scope": "all"
}
```

`scope` is required and is exactly `entry` or `all`. `entry_id` is required
only for `entry`; malformed combinations are rejected with `validation`.

Successful response:

```json
{
  "session_id": "session-id",
  "dispatched": true,
  "sent_count": 3
}
```

`sent_count` is the number of source queue entries represented by the new
turn. The action returns success only after the selection is claimed and handed
to the replacement-turn dispatch path.

Errors:

- `entry_not_found`: the selected entry is no longer visible and pending.
- `queue_empty`: the `all` snapshot contains no visible pending entries.
- `queue_changed`: one or more bulk-snapshot entries changed eligibility before
  the atomic claim; no partial selection is dispatched.
- `send_now_conflict`: another cancellation or Send Now operation already owns
  the session.
- `turn_changed`: the active turn changed while the operation waited; the
  successor is left untouched.
- `send_now_attachment_overflow`: the aggregate exceeds an attachment limit.
- `send_now_reference_overflow`: the aggregate exceeds the entity-reference
  limit.
- `session_not_promptable`: the session has neither an interruptible active
  turn nor a promptable state.

All requests require access to `session_id`. Authorization happens before any
queue read, cancellation, or mutation and uses the existing non-enumerating
session-not-found response.

## State and Concurrency

The orchestrator snapshots the active turn and, for `scope=all`, the ordered
visible entry IDs before beginning cancellation. All validation and aggregation
limits are checked before the cancellation signal is sent.

The cancel-and-dispatch decision uses the shared per-session cancellation and
queue-take serialization point. An ordinary FIFO handoff has an explicit
pre-acceptance phase: Send Now may supersede that reservation while the worker
has not claimed prompt ownership, but the worker must claim ownership before
creating a visible user message, running turn-start workflow effects, or
accepting the agent prompt. After that claim, the handoff is terminal for Send
Now and the existing conflict response applies.

When Send Now supersedes a pre-acceptance FIFO reservation, the backend
restores that exact source (including clearing a durable lifecycle
reservation), then atomically claims exactly the selected ID or the complete
bulk snapshot. New messages accepted after the snapshot remain queued. A
missing or newly ineligible bulk member fails the whole claim; the backend
never sends a partial bulk result or substitutes the FIFO head for a missing
selected entry.

Ordinary entries are removed when claimed, matching normal queue delivery.
Durable lifecycle entries remain reserved until the combined prompt is
accepted, then all durable source rows are acknowledged. A retryable dispatch
failure restores every ordinary source at its original FIFO position and
releases every durable reservation before publishing queue status.

## Permissions

- Any user who can access the session may invoke either Send Now action on its
  visible pending entries, regardless of entry provenance.
- The action never accepts caller-supplied `task_id`, `user_id`, sender identity,
  content, attachments, or metadata. Those values come from the authorized
  session and persisted queue rows.
- Hidden durable entries already reserved for another delivery are not eligible.

## Failure Modes

- **Cancellation fails:** no queue selection is claimed; the entries remain
  pending and the UI refetches and shows a localized error.
- **Selection changes during cancellation:** the active turn may already be
  cancelled, but no replacement is dispatched from a partial or different
  selection. The UI refetches and asks the user to retry.
- **FIFO handoff race:** if normal FIFO delivery reserved the head but has not
  accepted its prompt, Send Now restores/reclaims that reservation and
  dispatches the requested exact selection. If FIFO already accepted the
  prompt, Send Now returns `send_now_conflict`; it leaves that successor and
  the remaining queue authoritative rather than creating a duplicate turn.
- **Prompt admission fails after claim:** the backend restores the original
  entries and their FIFO positions before publishing status. Existing ordinary
  queue delivery retains its current process-crash window after a destructive
  claim; durable lifecycle rows retain their accepted-until-acknowledged
  guarantee.
- **Successor turn appears:** the operation fails with `turn_changed`, leaves
  the successor running, and does not claim queue entries.
- **Conflicting explicit Cancel:** the already accepted operation wins. Send Now
  never dispatches after a workflow-completing cancellation.

## Responsive and Mobile Behavior

- The existing inline queue panel remains the desktop and phone composition;
  its queue list remains the only internal scroll owner and the composer stays
  visible.
- Desktop row actions keep their current hover/focus disclosure. Phone and
  coarse-pointer layouts expose per-row **Send Now** without hover and give it
  at least a 44 by 44 CSS-pixel hit target.
- The header **Send Now**, **Clear all**, and collapse controls remain reachable
  without horizontal page overflow. The header may wrap its action group on a
  narrow phone rather than shrinking labels below usable touch targets.
- Desktop and mobile share the same queue hook, cancellation state, and backend
  action. Mobile Playwright coverage uses touch input and proves the same
  replacement-turn result.

## Scenarios

- **GIVEN** an agent is busy and three messages are queued, **WHEN** the user
  clicks row **Send Now** on the second message, **THEN** the active turn is
  interrupted, the second message starts the replacement turn, and the first
  and third messages remain queued in FIFO order.
- **GIVEN** an agent is busy and three messages are queued, **WHEN** the user
  clicks header **Send Now**, **THEN** one replacement turn receives the three
  message bodies concatenated in FIFO order and the queue becomes empty.
- **GIVEN** queued messages contain attachments and repeated entity references,
  **WHEN** the user sends all now, **THEN** the replacement prompt contains all
  attachments in FIFO order and one canonical copy of each reference.
- **GIVEN** a bulk aggregate exceeds an existing message limit, **WHEN** the
  user clicks header **Send Now**, **THEN** the active turn continues, the queue
  is unchanged, and a localized limit error is shown.
- **GIVEN** the session is already promptable with queued work, **WHEN** the user
  clicks either Send Now action, **THEN** the requested selection starts without
  issuing a cancellation.
- **GIVEN** a promptable session has two queued messages and normal FIFO
  delivery has reserved the first one but has not accepted its prompt, **WHEN**
  the user clicks header Send Now, **THEN** the FIFO reservation is restored,
  both messages are claimed in FIFO order, and exactly one replacement prompt
  contains both bodies.
- **GIVEN** normal FIFO delivery has already accepted its successor prompt,
  **WHEN** the user clicks Send Now, **THEN** the action fails closed without a
  duplicate user message, duplicate prompt, or cancellation of that successor,
  and the authoritative remaining queue is preserved.
- **GIVEN** the workflow step enables `cancel_triggers_turn_complete`, **WHEN**
  the user sends a queued message now, **THEN** the task stays on the same step
  and only the replacement turn's ordinary lifecycle hooks may change it.
- **GIVEN** a successor turn replaces the observed active turn while Send Now
  waits, **WHEN** the operation revalidates, **THEN** it leaves the successor
  running and the requested queue entries pending.
- **GIVEN** a phone viewport with a busy agent and queued messages, **WHEN** the
  user opens the queue panel, **THEN** both per-row and header Send Now controls
  are visible, touch-sized, and can start the same replacement-turn flow without
  horizontal overflow.

## Out of Scope

- Reordering queued messages before sending; that capability is governed by
  [Reorder Queued Messages](message-queue-reorder.md), and bulk Send Now
  dispatches in the current (reordered) FIFO order.
- Choosing a different model or plan mode for the bulk turn.
- Sending an arbitrary user-selected subset other than one entry or all visible
  pending entries.
- Changing normal FIFO auto-drain selection or **Run next**, **Remove**, **Clear
  all**, edit, or merge semantics. FIFO participates in Send Now handoff
  arbitration only until its prompt is accepted.
- Making ordinary queued-message dispatch crash-durable after its existing
  destructive claim boundary.
