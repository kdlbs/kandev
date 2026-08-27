---
status: active
system: ui
created: 2026-08-05
owners:
  - kandev
---
# Send Queued Messages Now Requirements

## Overview

When an agent is in a long-running turn, an urgent correction can sit in the queue until that turn finishes. Users need to replace the current turn with one specific queued instruction, or with the whole pending queue as one coherent follow-up, without completing or advancing the task's workflow step.

## Requirements

### REQ-UI-MESSAGE-QUEUE-SEND-NOW-001: Send Queued Messages Now

**Intent:** When an agent is in a long-running turn, an urgent correction can sit in the queue until that turn finishes. Users need to replace the current turn with one specific queued instruction, or with the whole pending queue as one coherent follow-up, without completing or advancing the task's workflow step.

#### Acceptance criteria

- **AC-UI-MESSAGE-QUEUE-SEND-NOW-001.1:** Every visible pending queue row, including the FIFO head, offers **Send Now**. On a fine-pointer desktop it appears with row actions on hover or keyboard focus; on a coarse-pointer surface it is always visible and touch-sized. No separate **Skip to next** action duplicates this behavior.
- **AC-UI-MESSAGE-QUEUE-SEND-NOW-001.2:** Per-row **Send Now** interrupts the active agent turn and dispatches that exact entry as the first prompt of a replacement turn. This behaves like a transient promotion without persisting a reorder: other queued entries keep their relative FIFO order. An accepted exact claim also turns Auto-run ON, so automatic FIFO delivery continues through them after the selected turn completes and the session is eligible.
- **AC-UI-MESSAGE-QUEUE-SEND-NOW-001.3:** The first-party queue header does not offer bulk **Send Now**. Protocol clients may still dispatch the click-time snapshot of all visible pending entries as one replacement turn with `scope: "all"`.
- **AC-UI-MESSAGE-QUEUE-SEND-NOW-001.4:** Bulk content is concatenated in FIFO order with one blank line between non-empty message bodies. Attachment-only entries add no empty separators. Attachments retain FIFO order, and entity references are combined in first- occurrence order with canonical duplicates removed.
- **AC-UI-MESSAGE-QUEUE-SEND-NOW-001.5:** The bulk turn uses the oldest selected entry's model and plan-mode snapshot. Its transcript envelope uses that entry's sender attribution while retaining source entry identities and provenance in metadata for restoration and diagnostics.
- **AC-UI-MESSAGE-QUEUE-SEND-NOW-001.6:** If the aggregate would exceed the existing per-message attachment count, attachment byte, or entity-reference limits, the request is rejected before cancellation. No content is truncated and the queue is unchanged.
- **AC-UI-MESSAGE-QUEUE-SEND-NOW-001.7:** A promptable session dispatches immediately without cancellation. A busy session uses the existing backend-owned cancellation progress, cancels only the turn observed when the action began, and starts the replacement turn after cancellation settles.
- **AC-UI-MESSAGE-QUEUE-SEND-NOW-001.8:** If ordinary FIFO delivery has reserved a queued entry but has not yet accepted its prompt, Send Now wins that same-session handoff. The reserved source is restored before the requested selection is claimed, so an all-scope replacement can include it in one aggregate prompt. Once FIFO has accepted its prompt, Send Now fails closed with `send_now_conflict`; it does not duplicate or cancel that successor turn.

## Migrated source detail

Decision:
[ADR-2026-08-05-queue-send-now-replaces-turn](../../../decisions/2026-08-05-queue-send-now-replaces-turn.md)

The first-party header, Auto-run policy, and row placement are governed by
[Control Pending Message Auto-run](message-queue-run.md). This spec remains
authoritative for targeted replacement delivery and the backward-compatible
`message.queue.send_now` protocol, including `scope: "all"`.

## Why

When an agent is in a long-running turn, an urgent correction can sit in the
queue until that turn finishes. Users need to replace the current turn with one
specific queued instruction, or with the whole pending queue as one coherent
follow-up, without completing or advancing the task's workflow step.

## What

- Every visible pending queue row, including the FIFO head, offers **Send
  Now**. On a fine-pointer desktop it appears with row actions on hover or
  keyboard focus; on a coarse-pointer surface it is always visible and
  touch-sized. No separate **Skip to next** action duplicates this behavior.
- Per-row **Send Now** interrupts the active agent turn and dispatches that
  exact entry as the first prompt of a replacement turn. This behaves like a
  transient promotion without persisting a reorder: other queued entries keep
  their relative FIFO order. An accepted exact claim also turns Auto-run ON,
  so automatic FIFO delivery continues through them after the selected turn
  completes and the session is eligible.
- The first-party queue header does not offer bulk **Send Now**. Protocol
  clients may still dispatch the click-time snapshot of all visible pending
  entries as one replacement turn with `scope: "all"`.
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
- Auto-run becomes ON in the same repository transaction that claims the exact
  Send Now selection. Validation, cancellation, conflict, or queue-change
  failures before that claim leave the previous policy unchanged. If the
  accepted asynchronous handoff later restores the claim, Auto-run remains ON
  because Send Now was also an explicit resume request. The all-entry scope
  follows the same rule.
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

The claim transaction also persists `auto_run=true`. This write is part of the
all-or-nothing selection mutation, so a rejected claim cannot partially resume
the queue. Restoration after an accepted claim restores message rows but does
not undo that explicit resume instruction.

## Permissions

- Any user who can access the session may invoke either Send Now action on its
  visible pending entries, regardless of entry provenance.
- The action never accepts caller-supplied `task_id`, `user_id`, sender identity,
  content, attachments, or metadata. Those values come from the authorized
  session and persisted queue rows.
- Hidden durable entries already reserved for another delivery are not eligible.

## Failure Modes

- **Cancellation fails:** no queue selection is claimed; the entries remain
  pending, Auto-run retains its prior value, and the UI refetches and shows a
  localized error.
- **Selection changes during cancellation:** the active turn may already be
  cancelled, but no replacement is dispatched from a partial or different
  selection. The UI refetches and asks the user to retry.
- **FIFO handoff race:** if normal FIFO delivery reserved the head but has not
  accepted its prompt, Send Now restores/reclaims that reservation and
  dispatches the requested exact selection. If FIFO already accepted the
  prompt, Send Now returns `send_now_conflict`; it leaves that successor and
  the remaining queue authoritative rather than creating a duplicate turn.
- **Prompt admission fails after claim:** the backend restores the original
  entries and their FIFO positions before publishing status. Auto-run stays ON
  because the operation was already accepted as a resume. Existing ordinary
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
  coarse-pointer layouts expose every row's **Send Now** without hover and give
  it at least a 44 by 44 CSS-pixel hit target.
- The header **Auto-run** switch, **Clear all**, and collapse controls remain
  reachable without horizontal page overflow. The header may wrap on a narrow
  phone rather than shrinking labels below usable targets.
- Desktop and mobile share the same queue hook, cancellation state, and backend
  action. Mobile Playwright coverage uses touch input and proves the same
  replacement-turn result.

## Scenarios

- **GIVEN** an agent is busy and three messages are queued, **WHEN** the user
  clicks row **Send Now** on the second message, **THEN** the active turn is
  interrupted, Auto-run becomes ON, the second message starts the replacement
  turn, and after that turn completes the first and third messages run as
  separate turns in their original relative order without another
  queue-control click.
- **GIVEN** Auto-run is OFF with queued A, B, and C, **WHEN** the user clicks
  Send Now on B, **THEN** B runs first and the preserved A, C remainder
  continues automatically as separate turns.
- **GIVEN** an authorized protocol client selects all three visible messages,
  **WHEN** it sends `message.queue.send_now` with `scope: "all"`, **THEN** one
  replacement turn receives the three message bodies concatenated in FIFO
  order and the queue becomes empty.
- **GIVEN** queued messages contain attachments and repeated entity references,
  **WHEN** a protocol client requests `scope: "all"`, **THEN** the replacement
  prompt contains all attachments in FIFO order and one canonical copy of each
  reference.
- **GIVEN** a bulk aggregate exceeds an existing message limit, **WHEN** a
  protocol client requests `scope: "all"`, **THEN** the active turn continues,
  the queue is unchanged, and the stable limit error is returned.
- **GIVEN** the session is already promptable with queued work, **WHEN** the
  user invokes row Send Now or a protocol client requests `scope: "all"`,
  **THEN** the requested selection starts without issuing a cancellation.
- **GIVEN** a promptable session has two queued messages and normal FIFO
  delivery has reserved the first one but has not accepted its prompt, **WHEN**
  a protocol client requests `scope: "all"`, **THEN** the FIFO reservation is
  restored, both messages are claimed in FIFO order, and exactly one
  replacement prompt contains both bodies.
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
  user opens the queue panel, **THEN** the Auto-run switch and every row's Send
  Now control are visible, touch-sized, and contained without horizontal
  overflow.

## Out of Scope

- Reordering queued messages before sending; that capability is governed by
  [Reorder Queued Messages](message-queue-reorder.md), and bulk Send Now
  dispatches in the current (reordered) FIFO order.
- Choosing a different model or plan mode for the bulk turn.
- Sending an arbitrary user-selected subset other than one entry or all visible
  pending entries.
- Changing **Remove**, **Clear all**, edit, or merge semantics. FIFO participates
  in Send Now handoff arbitration only until its prompt is accepted. Normal
  FIFO policy and first-party queue controls are governed by
  [Control Pending Message Auto-run](message-queue-run.md).
- Making ordinary queued-message dispatch crash-durable after its existing
  destructive claim boundary.
