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

This design owns how a turn row finds the message its send stored, and the sent test that decides whether the send reached the conversation. Split out of [wake](wake.md) for size; the tables, admission, delivery and turn end stay there.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-WAKE-005` | [Finding the turn's message](#finding-the-turns-message) |

## Finding the turn's message

The lookup is a coordinator-side message finder: given a session id, a metadata
key and value, and `since`, it returns `{ID, TurnID}` or nil. It reads the
conversation session's messages created at or after the turn's `started_at`
and returns the one whose `metadata.coordinator_wake_turn_id` equals the turn
id, oldest first by (`created_at`, `id`); more than one match logs a warning
and takes the first. A session that no longer exists returns nil (nothing can
have been sent to it); a returned error is a read error, distinct from
not-found, and leaves the turn untouched that tick. The finder is the only
test of whether a send reached the conversation; nothing re-sends a turn
whose message exists. Delivery never queues, so stored messages are the only
place to look.

**Sent test.** A found message proves the send only when the turn it names
still exists. The entry point's pre-acceptance dispatch failure rolls the
reserved turn back and leaves the stored message, so the message alone is not
proof. The message's `TurnID` is the only source of `session_turn_id`, never
the session's active turn. The turn read (`GetTurn(TurnID)`) gives:

| Found message and turn read | Result |
| --- | --- |
| `TurnID` non-empty, turn exists, and its session is the turn row's `session_id` | sent: record `message_id` and `session_turn_id` (conditional, as in Delivery) |
| `TurnID` empty, turn not found, or the turn belongs to another session (warn log; never bound) | not sent: the caller settles as for a message that was not found |
| the turn read returns an error | untouched this tick |

The message left behind by a not-sent result stays in the conversation and is
never matched again, because every later turn has a new turn row id and a new
message.

The startup pass, and each backstop tick, examine turn rows with `outcome IS
NULL` and `message_id` null. The startup pass examines only open rows whose
`started_at` is before its own start time, so a live in-flight turn is never
settled `interrupted`. When the lookup finds the message and the sent test
passes, they record `message_id` and `session_turn_id` (conditional, as in
Delivery) and leave the row to [Turn end](wake.md#turn-end).
When it does not (not found, or not sent), the startup pass settles the row
`interrupted`, and a
backstop tick settles it `send_failed` once two minutes have passed since
`started_at` and the session is not `RUNNING` or `STARTING`; a session that no
longer exists counts as neither, and a failed session-state read defers the
settle to the next tick. Either way its
wakes return to `pending` with `turn_id` null. The settle is conditional on
`outcome IS NULL AND message_id IS NULL` and, as in Delivery step 5, returns
the wakes, counts `coordinator_unattended_turn_total{outcome}` and publishes
`coordinator.updated` with `autonomy_changed: true` only when it changed a
row.
