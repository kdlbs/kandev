---
id: coordinator-wake-recovery-design
title: Wake turn message recovery design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-30
last_updated: 2026-09-30
requirements:
  - REQ-COORDINATOR-WAKE-005
---

# Wake turn message recovery design System Design

## Purpose and boundaries

This design owns how an open turn row learns whether its send reached the conversation (the sent test), the one rule that settles a send that did not, the message lookup, and the startup pass. Split out of [wake](wake.md) for size; the tables, admission, delivery and turn end stay there.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-WAKE-005` | [Sent test](#sent-test), [Settle rule](#settle-rule), [Finding the turn's message](#finding-the-turns-message), [Startup pass](#startup-pass) |

## Sent test

A stored wake message is not proof of a send. The entry point stores it before dispatch with the reserved turn's id; a pre-acceptance failure rolls the reserved turn back, but the task repository's `DeleteTurnIfUnreferenced` never deletes a turn a message references, so the message and its turn both survive. The only proof is the binding: the open turn row's `session_turn_id`, written by `onAccepted` at agentctl acceptance ([Delivery step 4](wake.md#delivery)).

| Open turn row | Result |
| --- | --- |
| `session_turn_id` non-null | sent: the turn continues and is settled by [Turn end](wake.md#turn-end); `message_id`, if null, is filled by the lookup below and a lookup that finds nothing changes nothing |
| `session_turn_id` null | not sent: settled by the [settle rule](#settle-rule); an accepted prompt whose binding was lost is treated the same way and is never re-sent |

The message's `TurnID` and the session's active turn are never a source of `session_turn_id`. Recovery never re-sends: a later delivery builds a new turn row and a new message.

## Settle rule

A not-sent row is settled by one transaction: `UPDATE coordinator_unattended_turns SET outcome=?, finished_at=? WHERE id=? AND outcome IS NULL AND session_turn_id IS NULL`, with `outcome` `send_failed` (Delivery step 5 and the backstop) or `interrupted` (the startup pass). Only when it changed a row do the wakes return to `pending` with `turn_id` null (`UPDATE coordinator_wakes SET status='pending', turn_id=NULL, updated_at=? WHERE turn_id=?`), and after commit it counts `coordinator_unattended_turn_total{outcome}`, publishes `coordinator.updated` with `autonomy_changed: true`, and marks the orphan message. A settle that changed no row lost to another settle or to a late binding and does nothing further. Because the guard is `session_turn_id IS NULL`, a binding that lands first makes the settle a no-op and a settle that lands first makes a later binding a warn-logged no-op (the residual: a prompt accepted with a lost binding may have run, and the next turn re-reports still-current events, which is safe because each turn tells the agent to read current state).

Marking the orphan message is best effort: the message finder's `MarkOrphan(sessionID, messageID)` sets `metadata.coordinator_wake_orphaned` to true so the transcript can show it as not delivered; a failure is logged at warn and changes nothing, because the settled `outcome` is what decides.

The backstop settles `send_failed` only once two minutes have passed since `started_at` and the conversation session is not `RUNNING` or `STARTING` (a session that no longer exists counts as neither; a failed session-state read defers the settle to the next tick). While the session is `RUNNING` the row stays open and unbound and containment keeps denying by session match, so a turn that did start is never treated as attended.

## Finding the turn's message

The lookup is a coordinator-side message finder: given a session id, a metadata key and value, and `since`, it returns `{ID, TurnID}` or nil. It reads the conversation session's messages created at or after the turn's `started_at` and returns the one whose `metadata.coordinator_wake_turn_id` equals the turn id, oldest first by (`created_at`, `id`); more than one match logs a warning and takes the first. A session that no longer exists returns nil; a returned error is a read error, distinct from not-found, and leaves the row untouched that tick. It serves two callers only: recording `message_id` for a bound row and finding the message to mark an orphan. It never decides whether a send happened.

The backstop examines each open row: a bound row with null `message_id` runs the lookup and records `message_id` conditionally (`WHERE id=? AND outcome IS NULL AND message_id IS NULL`; a changed-no-row result is a no-op); an unbound row past two minutes runs the settle rule.

## Startup pass

`registerCoordinatorDelivery`'s hook runs after the wake hook (`phase3Registrations` order: wake, then delivery), so `StartWakeBackstop` has already started and its first pass is 60 seconds away. The startup pass examines only open rows whose `started_at` is before the pass's own start time `t0` (a live in-flight turn is never touched). An unbound row is settled `interrupted` by the settle rule; a bound row is left to turn end and the backstop's missed-settle. The pass is the sole owner of pre-`t0` rows at startup, and no ordering against a delivery, a `Kick` or the first backstop pass is needed: an open row makes admission check 7 hold `turn_open`, so no delivery starts over it, and every settle is conditional on `outcome IS NULL AND session_turn_id IS NULL`, so whichever runs first wins and the other is a no-op. A failed read leaves the row for the next backstop tick.
