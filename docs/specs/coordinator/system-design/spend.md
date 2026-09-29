---
id: coordinator-spend-design
title: Cost against a declared ceiling design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-SPEND-001
  - REQ-COORDINATOR-SPEND-002
  - REQ-COORDINATOR-SPEND-003
  - REQ-COORDINATOR-SPEND-004
---

# Cost against a declared ceiling System Design

## Purpose and boundaries

This design stores a coordinator's ceiling, measures its spend from the core
task usage ledger, supplies admission checks 3 and 4 of
[wake](wake.md#admission), stops an open unattended turn at the ceiling, and
renders spend. It reads `task_usage_events`, which `internal/task/usage`'s
writer owns and which exists with or without Office; it never reads
`office_cost_events` and is not governed by Office budget policies.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-SPEND-001` | [Ceiling](#ceiling) |
| `REQ-COORDINATOR-SPEND-002` | [Measurement](#measurement), [Per-turn cost](#per-turn-cost) |
| `REQ-COORDINATOR-SPEND-003` | [Stopping](#stopping) |
| `REQ-COORDINATOR-SPEND-004` | [Screens](#screens) |

## Ceiling

`coordinators.cost_ceiling_subcents` (added by [wake](wake.md#flag-and-settings))
stores hundredths of a cent, the unit of `task_usage_events.cost_subcents`
(1 USD = 10,000 subcents). The PATCH field `cost_ceiling_usd` is a JSON string
matching `^[0-9]{1,5}(\.[0-9]{1,2})?$` whose value is from `0.01` to
`10000.00`, or JSON `null` to clear. A JSON number, any other string, or a
value outside the range is 400 naming `cost_ceiling`. Parsing is integer-only:
the whole part times 10,000 plus the fractional digits scaled to subcents, so
no float rounding occurs. The autonomy interlock of
`AC-COORDINATOR-SPEND-001.2` is enforced in the same PATCH transaction
([wake](wake.md#flag-and-settings)). Changing the ceiling calls `Kick` so a
raised ceiling releases held wakes at once. That call, and the
`autonomy_changed` publish, follow the post-commit,
best-effort, change-only rule of
[integration](integration.md#autonomy-patch-and-deletion).

## Measurement

`internal/coordinator/spend.go` exports
`Spend(ctx, coordinator, now) (SpendReading, error)` with
`SpendReading{WindowSubcents, Mean7dSubcents int64; Measurable, Degraded bool}`.

1. **Conversation tasks.** `ListCoordinatorOriginTasks(ctx, workspaceID)`
   (phase 1, [copilot](copilot.md#conversation-cleanup)) filtered in Go to
   rows whose `coordinator_id` is this coordinator: current and archived
   conversation tasks alike, and no other task
   (`AC-COORDINATOR-SPEND-002.1`). A coordinator with none has spend 0 and is
   measurable.
2. **Window sum.** One query on the task repository's read handle:

   ```sql
   SELECT
     COALESCE(SUM(CASE WHEN cost_source = 'unpriced' THEN 0 ELSE cost_subcents END), 0),
     COALESCE(SUM(CASE WHEN cost_source = 'unpriced' THEN 1 ELSE 0 END), 0) > 0
   FROM task_usage_events
   WHERE task_id IN (...) AND occurred_at >= ? AND occurred_at < ?
   ```

   with `[now - 24h, now)`, the half-open bound the Office spend window uses.
   The `IN` list is chunked at 500 ids and the chunks summed.
3. **Seven-day mean.** The same query over `[now - 7d, now)`, divided by 7
   with integer division, for display only.
4. `Measurable` is false when any query errors or the 24-hour window is
   degraded (holds an unpriced row). `Degraded` alone reports the second
   case to the UI.

Spend includes attended and unattended turns, because both are the
coordinator's own sessions. A conversation task deleted with its coordinator
takes its usage rows with it (`ON DELETE CASCADE`), which is correct: the
coordinator no longer exists.

Admission check 3 fails with `spend_unmeasured` when `Measurable` is false;
check 4 fails with `ceiling_reached` when `WindowSubcents >=
cost_ceiling_subcents` (`AC-COORDINATOR-SPEND-003.1`).

## Per-turn cost

When [wake](wake.md#turn-end) settles a turn, it sets `cost_subcents` to the
sum of priced `cost_subcents` over `task_usage_events` where `session_id` is
the turn's session and `occurred_at` is in `[started_at, finished_at]`, with
the unpriced count ignored for this display value. The usage writer is
asynchronous, so a turn's last usage row can land after the settle; the
backstop recomputes `cost_subcents` once for turns settled in the last 10
minutes, and the value is final after that.

## Stopping

`internal/task/usage`'s writer gains an optional post-commit observer,
`OnRecorded(taskID, sessionID string)`, called after each successful insert
in the writer's own goroutine, never blocking on it (the call enqueues onto a
bounded channel of 256 and drops with a metric when full). The coordinator
registers one only while phase 3 is effective. `CheckCeiling(coordinatorID)`
runs for the coordinator whose open unattended turn is on `sessionID`. It
acts only when the session's active turn (`GetActiveTurnBySessionID`) is the
row's `session_turn_id` ([wake turn end](wake.md#turn-end)); a session running
any other turn is left alone. The ceiling it compares against is the
coordinator's current `cost_ceiling_subcents`, so a raised or lowered ceiling
applies to the open turn; when that is null (a manager turned autonomy off
and then cleared the ceiling while the turn was still open), it is the turn
row's `start_ceiling_subcents` ([wake store](wake.md#store)). It runs `Spend`,
and when the result is not measurable or `WindowSubcents` is at or above
that ceiling it:

1. marks the row with `stop_requested_at` in `UPDATE ... SET
   stop_requested_at = ? WHERE id = ? AND outcome IS NULL AND
   stop_requested_at IS NULL`, the retry marker; an unmeasurable reading
   stops the same way, because the ceiling can no longer be shown to hold;
2. cancels the turn through `orchestrator.Service.CancelTurn(ctx, sessionID,
   expectedTurnID)`, a new turn-fenced variant of `CancelAgent` (the path the
   panel's Stop uses), passing the row's `session_turn_id`;
3. when the cancel is confirmed (it returned without error), settles the turn
   `outcome='stopped_at_ceiling'`, `finished_at` and its
   [per-turn cost](#per-turn-cost) in `UPDATE ... WHERE id = ? AND outcome IS
   NULL`, and publishes `coordinator.updated` with `autonomy_changed`.

`CancelTurn` runs `CancelAgent`'s sequence (the explicit-cancellation claim,
then the cancel-in-flight guard) and, inside the guard, compares the captured
`cancellationIdentity.turnID` with `expectedTurnID` before cancelling
anything. When they differ, or no turn is active, it returns
`ErrTurnNotActive` and cancels nothing; the agent cancel it issues is fenced
to the captured prompt generation, so a prompt dispatched after the capture
is never cancelled. The active-turn check before step 1 is therefore a cheap
filter only; the fence is the comparison inside the guard. `CheckCeiling`
treats `ErrTurnNotActive` as "the unattended turn already ended": it does
not settle the row, which [turn end](wake.md#turn-end) settles as
`stopped_at_ceiling`. A test pins the race: a manager's queued message
drained onto the session between the pre-check and the cancel is not
cancelled.

When the cancel fails, the row stays open with `stop_requested_at` set: the
turn stays unattended, so its permissions are still denied, and the next
observer call or [backstop](wake.md#backstop) tick cancels again, since an
open row with `stop_requested_at` set is retried whatever the spend now reads.
When the turn ends by itself first, [turn end](wake.md#turn-end) settles a row
with `stop_requested_at` set as `stopped_at_ceiling`, whatever the session
state. A retried cancel of a turn that has already ended gets `ErrTurnNotActive`
and does nothing.

The backstop runs `CheckCeiling` for every open unattended turn each tick,
whether or not autonomy is still on ([wake backstop](wake.md#backstop)), so
a dropped observer call or a failed cancel delays the stop by at most 60
seconds (`AC-COORDINATOR-SPEND-003.2`). Both acting paths cancel only
through `CancelTurn` fenced to the row's `session_turn_id`, so an attended
turn, including a manager's queued message drained on the same session at any
point before the cancel, is never cancelled (`AC-COORDINATOR-SPEND-003.3`); the phase 1 message send path has no ceiling
check.

The overshoot is bounded by one usage report, or by one backstop period when
an observer call is dropped or one cancel fails: the ceiling is compared after
each recorded usage event, not before a model request, since Kandev does not
mediate the agent CLI's model calls. The bound holds only while a cancel
eventually succeeds. When cancels keep failing (the agent process ignores
them), the turn keeps spending until it ends by itself; Kandev has no second
stop below the agent cancel. That residual is made visible, not hidden: each
failed cancel increments `coordinator_ceiling_cancel_failed_total`, and once
a row has `stop_requested_at` older than five minutes and is still open, the
autonomy read reports `last_turn.stop_state: "stop_failing"` and the autonomy strip
shows "Stop at ceiling not confirmed: the turn is still running" with the
panel's Stop, so a manager can act (`AC-COORDINATOR-SPEND-003.4`). Admission
already holds new unattended turns while any turn is open, so the failing
turn is the only one spending.

## Screens

- **Autonomy strip (UI-01).** From the autonomy read's `spend` block: "Spend
  <window> of <ceiling> USD in 24 h" and a pill "Inside" (window below
  ceiling) or "Over". With no ceiling: "Spend <window> USD in 24 h" and "No
  ceiling". Not measurable: "Spend unknown: some usage is unpriced" when
  `degraded`, else "Spend unavailable".
- **Settings, Autonomy section (UI-04).** A "Cost ceiling (USD per 24 hours)"
  field with the same validation client-side, the same spend line and pill,
  "7-day daily mean <mean> USD" and "Last unattended turn <cost> USD" or
  "No unattended turns yet".
- Amounts render with two decimals through the existing currency formatter in
  `apps/web/lib/i18n/formats`. Copy goes through `t()` in six locales.

## Failure and recovery

| Failure | Result |
| --- | --- |
| Usage query error | Not measurable: admission holds with `spend_unmeasured`; an open unattended turn is stopped |
| Unpriced usage in the window | Same, until the row leaves the window |
| Observer queue full | Metric; the backstop applies the stop within 60 s |
| Cancel fails | Row stays open with `stop_requested_at`; the next observer call or tick cancels again |
| Cancel keeps failing for 5 minutes | Metric per failure; autonomy read and strip show `stop_failing` until the turn ends |
| Unattended turn ended before the cancel | `ErrTurnNotActive`; nothing is cancelled; turn end settles the row |
| Late usage row after settle | Per-turn cost corrected by the backstop within 10 minutes |

## Security

The ceiling is written only by the PATCH route (`workspace.manage`). Spend
reads are `workspace.read` through the autonomy route; they return amounts,
never per-message usage.

## Observability

`coordinator_ceiling_stop_total`, `coordinator_ceiling_cancel_failed_total`,
`coordinator_spend_read_failed_total` and
`coordinator_usage_observer_dropped_total` counters, plus a structured zap log
at info for each ceiling stop with the coordinator id, window and ceiling.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Instance-wide claim lock and budget fail mode asymmetry](../../../decisions/2026-09-16-instance-wide-claim-lock-and-budget-fail-mode-asymmetry.md)
