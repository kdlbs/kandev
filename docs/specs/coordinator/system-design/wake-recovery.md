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
| `REQ-COORDINATOR-WAKE-005` | [Sent test](#sent-test), [Settle rule](#settle-rule), [Orphan turn](#orphan-turn), [Finding the turn's message](#finding-the-turns-message), [Startup pass](#startup-pass) |

## Sent test

A stored wake message is not proof of a send. The entry point stores it before dispatch with the reserved turn's id; a pre-acceptance failure rolls the reserved turn back, but the task repository's `DeleteTurnIfUnreferenced` never deletes a turn a message references, so the message and its turn both survive. The only proof is the binding: the open turn row's `session_turn_id`, written by `onAccepted` at agentctl acceptance ([Delivery step 4](wake.md#delivery)).

| Open turn row | Result |
| --- | --- |
| `session_turn_id` non-null | sent: the turn continues and is settled by [Turn end](wake.md#turn-end); `message_id`, if null, is filled by the lookup below and a lookup that finds nothing changes nothing |
| `session_turn_id` null | not sent: settled by the [settle rule](#settle-rule); an accepted prompt whose binding was lost is treated the same way and is never re-sent |

The message's `TurnID` and the session's active turn are never a source of `session_turn_id`. Recovery never re-sends: a later delivery builds a new turn row and a new message.

## Settle rule

A not-sent row is first cleared of its [orphan turn](#orphan-turn), then settled by one transaction: `UPDATE coordinator_unattended_turns SET outcome=?, finished_at=? WHERE id=? AND outcome IS NULL AND session_turn_id IS NULL`, with `outcome` `send_failed` (Delivery step 5 and the backstop) or `interrupted` (the startup pass). Only when it changed a row do the wakes return to `pending` with `turn_id` null (`UPDATE coordinator_wakes SET status='pending', turn_id=NULL, updated_at=? WHERE turn_id=?`), and after commit it counts `coordinator_unattended_turn_total{outcome}`, publishes `coordinator.updated` with `autonomy_changed: true`, and marks the orphan message. A settle that changed no row lost to another settle or to a late binding and does nothing further. Because the guard is `session_turn_id IS NULL`, a binding that lands first makes the settle a no-op and a settle that lands first makes a later binding a warn-logged no-op (the residual: a prompt accepted with a lost binding may have run, and the next turn re-reports still-current events, which is safe because each turn tells the agent to read current state).

Marking the orphan message is best effort: the message finder's `MarkOrphan(sessionID, messageID)` sets `metadata.coordinator_wake_orphaned` to true so the transcript can show it as not delivered; a failure is logged at warn and changes nothing, because the settled `outcome` is what decides.

The backstop settles `send_failed` only once two minutes have passed since `started_at` and the conversation session is not `RUNNING` or `STARTING` (a session that no longer exists counts as neither; a failed session-state read defers the settle to the next tick). While the session is `RUNNING` the row stays open and unbound and containment keeps denying by session match, so a turn that did start is never treated as attended.

## Orphan turn

When a pre-acceptance failure follows the seam, the orchestrator rolls the reserved turn back, but `DeleteTurnIfUnreferenced` keeps a turn a message references, so the orphan turn stays the session's active turn (`completed_at` null) and admission check 7 would hold `conversation_busy` with nothing to clear it. A manager's next prompt would also adopt it. So every settle by the settle rule whose row has a non-null, non-empty `reserved_turn_id` first calls the message finder interface's `CompleteOrphanTurn(ctx, sessionID, turnID)`, then runs the conditional UPDATE. The adapter calls one new exported orchestrator method, `Service.CompleteUnattendedOrphanTurn(ctx, sessionID, turnID) error` (`internal/orchestrator/service.go`), which is a thin guard over the existing `completeExpectedTurn` (same file), the path the cancellation flow already uses to complete a captured turn. `completeExpectedTurn` completes the turn through the task turn service's `CompleteTurn` (which publishes `turn.completed`; no bound row matches it, so this design's subscriber does nothing), verifies closure, and clears the orchestrator's in-memory `activeTurns` entry with `CompareAndDelete(sessionID, turnID)`. That clearing is required: the orchestrator's pre-acceptance rollback (`rollbackReservedPromptTurn`, `internal/orchestrator/task_operations.go`) stores the surviving turn in `activeTurns` and prompt admission reuses that entry before reading the database, so completing the turn in the database alone would leave a completed id in `activeTurns` that the next manager message and the next wake send would pick up. No second completion path is added and the task service `CompleteTurn` is never called directly for this purpose.

The method first reads the session state and returns the exported error `ErrOrphanTurnSessionBusy` without changing anything when the state is `RUNNING` or `STARTING` (fail closed; a session that no longer exists counts as neither). It then calls `completeExpectedTurn`, whose results the caller keeps unchanged: no active turn returns nil (already completed or missing, nothing to do), and on that path the method itself still calls `activeTurns.CompareAndDelete(sessionID, turnID)` because `completeExpectedTurn` returns before clearing it, so an entry left behind by an earlier tick whose completion succeeded but whose closure re-read failed, or by another path that completed the turn in the database, does not survive; an active turn that is not `turnID` returns its "superseded" error, which also covers a turn of another session because the read is by the row's session; a failed read or completion returns its error. Every non-nil error means the settle leaves the row untouched that tick (the next backstop tick retries; a repeat is a no-op). A null or empty `reserved_turn_id` (the seam never ran, its write failed, or a partial write left an empty string) is treated as null and completes nothing. Until the settle, the conversation holds `conversation_busy`, bounded by the two-minute rule plus one backstop tick; the startup pass settles pre-`t0` rows at once.

The startup pass calls the same guard, so it never bypasses `ErrOrphanTurnSessionBusy`: a pre-`t0` row whose session is persisted `RUNNING` or `STARTING` is left open that pass and settled later by the backstop as `send_failed` once the session is no longer busy, and the row settled by the startup pass is `interrupted` only when the session is not busy. Which of the two outcomes a row gets is decided by who settles it, and both return the wakes to `pending` without a re-send.

Residuals, stated and not designed away: a failed `RollbackReservedTurn` (`internal/orchestrator/task_operations.go`) keeps the session's private reservation, so completing the orphan turn frees nothing and every later wake fails `ErrAgentPromptInProgress` and settles `send_failed`, bounded only by the cooldown, until the backend restarts; the session-state read and the completion are not atomic, so a manager prompt that starts between them can have its adopted turn completed once (its `turn.completed` then ends that turn early; the next message starts a new turn); a manager prompt that adopts the orphan turn makes the session `RUNNING`, which defers the settle for that whole manager turn and leaves unallowed permission requests denied and counted until it ends; and when the `reserved_turn_id` write failed, nothing completes the orphan turn, so `conversation_busy` holds until a manager acts on the conversation.

The reserved id also narrows [containment](containment.md#unattended-permissions): an unbound row denies a request only when `reserved_turn_id` is null or equals the request's active turn id. A manager prompt that adopted the orphan turn before the settle carries that id, so within the bounded window above its unallowed permission requests are denied and counted (the residual; the denial text tells the manager the turn is unattended); after the settle the turn is completed and never adopted.

## Finding the turn's message

The lookup is a coordinator-side message finder: given a session id, a metadata key and value, and `since`, it returns `{ID, TurnID}` or nil. It reads the conversation session's messages created at or after the turn's `started_at` and returns the one whose `metadata.coordinator_wake_turn_id` equals the turn id, oldest first by (`created_at`, `id`); more than one match logs a warning and takes the first. A session that no longer exists returns nil; a returned error is a read error, distinct from not-found, and leaves the row untouched that tick. It serves two callers only: recording `message_id` for a bound row and finding the message to mark an orphan. It never decides whether a send happened.

The backstop examines each open row: a bound row with null `message_id` runs the lookup and records `message_id` conditionally (`WHERE id=? AND outcome IS NULL AND message_id IS NULL`; a changed-no-row result is a no-op); an unbound row past two minutes runs the settle rule.

## Startup pass

`registerCoordinatorDelivery`'s hook runs after the wake hook (`phase3Registrations` order: wake, then delivery), so `StartWakeBackstop` has already started and its first pass is 60 seconds away. The startup pass examines only open rows whose `started_at` is before the pass's own start time `t0` (a live in-flight turn is never touched). An unbound row is settled `interrupted` by the settle rule; a bound row is left to turn end and the backstop's missed-settle. The pass is the sole owner of pre-`t0` rows at startup, and no ordering against a delivery, a `Kick` or the first backstop pass is needed: an open row makes admission check 7 hold `turn_open`, so no delivery starts over it, and every settle is conditional on `outcome IS NULL AND session_turn_id IS NULL`, so whichever runs first wins and the other is a no-op. A failed read leaves the row for the next backstop tick.
