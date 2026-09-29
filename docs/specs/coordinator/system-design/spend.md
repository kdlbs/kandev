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
`SpendReading{WindowSubcents, Mean7dSubcents int64; Measurable, Degraded,
Mean7dKnown bool}`.

**Contract.** `now` is supplied by the caller (production callers pass the
coordinator service clock; tests pass a controlled clock). The result follows
one rule: the error is non-nil exactly when `Measurable` is false because a
read failed, and then every amount is zero (no partial total from chunks that
succeeded is ever returned). `Measurable` is false with a nil error only when
the 24-hour window holds an unpriced row (`Degraded` is then true). The one
exception is a caller scope error: an empty coordinator id or an empty
workspace id returns `ErrSpendScope` with `Measurable` false and every amount
zero, and issues no query, because `ListCoordinatorOriginTasks` reads every
workspace for an empty workspace id. It is not a failed read, so it is not
counted in `coordinator_spend_read_failed_total`; it is logged at warn.
Callers (admission checks 3 and 4, `CheckCeiling`, the autonomy read) treat
"error non-nil or `Measurable` false" as one condition, unmeasurable, which
includes `ErrSpendScope`; they never inspect the amounts of an unmeasurable
reading. Unmeasurable is the fail-closed reading: a failed read, a task list
that cannot be read, or an unpriced row is never treated as zero spend.

1. **Conversation tasks.** `ListCoordinatorOriginTasks(ctx, workspaceID)`
   (phase 1, [copilot](copilot.md#conversation-cleanup)) filtered in Go to
   rows whose task metadata `coordinator_id` (read with the same helper the
   phase 1 conversation code uses, `conversationTaskCoordinatorID`) equals
   this coordinator: current and archived conversation tasks alike, and no
   other task (`AC-COORDINATOR-SPEND-002.1`). A task whose metadata has no
   `coordinator_id`, or another value, is not this coordinator's. A
   coordinator with none has spend 0 and is measurable. A list error is a
   failed read.
2. **Window sum.** The task repository (`internal/task/repository/sqlite`,
   next to `usage_totals.go`, both dialects, its read handle) gains
   `SumUsageForTasks(ctx, taskIDs []string, from, to time.Time)
   (UsageSum{CostSubcents int64; HasUnpriced bool}, error)`. It rejects more
   than 500 ids with an error and returns `{0, false}` for none. Its query is:

   ```sql
   SELECT
     COALESCE(SUM(CASE WHEN cost_source = 'unpriced' THEN 0 ELSE cost_subcents END), 0),
     COALESCE(SUM(CASE WHEN cost_source = 'unpriced' THEN 1 ELSE 0 END), 0) > 0
   FROM task_usage_events
   WHERE task_id IN (...) AND occurred_at >= ? AND occurred_at < ?
   ```

   `Spend` calls it with `[now - 24h, now)`, the half-open bound the Office
   spend window uses, once per consecutive chunk of at most 500 ids in list
   order (order does not change a sum), adds the chunk sums, and ORs the
   flags. The chunks are separate reads with no shared snapshot: a usage row
   recorded between two chunk reads is counted or not by the chunk that read
   its task, and a row of a task already read is picked up by the next call.
   Sums saturate at `math.MaxInt64`; they never wrap, so an overflow reads as
   "at or above the ceiling". The failure of any chunk is a failed read of
   the whole window.
3. **Seven-day mean.** The same call over `[now - 7d, now)`, divided by 7
   with integer division, for display only. Unpriced rows in it are counted
   as zero, so the mean can understate; `Degraded` reports the 24-hour window
   only. A failure of this read does not change `Measurable` or the error:
   it leaves `Mean7dSubcents` 0 and `Mean7dKnown` false, so the display can
   say the mean is unavailable, and it never holds admission or stops a turn.
4. `Measurable` is false when the task list read fails, any 24-hour chunk
   read fails, or the 24-hour window is degraded (holds an unpriced row).
   `Degraded` alone reports the last case to the UI.

A usage row's `occurred_at` is stamped by the usage writer when it processes
the event, before the row is inserted and before the observer of
[Stopping](#stopping) is notified, and the stored value is never later than
that instant. A check that runs from that notification and passes its own
clock reading as `now` therefore always finds the triggering row inside
`[now - 24h, now)`; a test that freezes the clock must advance it by at least
one millisecond between the insert and the check.

Each failed read (task list, window chunk, seven-day chunk) increments
`coordinator_spend_read_failed_total` with a `read` label from the closed set
`tasks`, `window`, `mean`, and logs at warn.

Spend includes attended and unattended turns, because both are the
coordinator's own sessions. A conversation task deleted with its coordinator
takes its usage rows with it (`ON DELETE CASCADE`), which is correct: the
coordinator no longer exists.

Admission check 3 fails with `spend_unmeasured` when the reading is
unmeasurable (`AC-COORDINATOR-SPEND-002.2`); check 4 fails with
`ceiling_reached` when the reading is measurable and `WindowSubcents >=
cost_ceiling_subcents` (`AC-COORDINATOR-SPEND-003.1`). Both are exported as
`CheckSpendMeasurable(reading, err) bool` and
`CheckCeilingNotReached(reading, ceilingSubcents int64) bool` for the
admission code of [wake](wake.md#admission), which calls `Spend` once and
applies check 3 before check 4.

## Per-turn cost

`TurnCost(ctx, turn TurnKey) (subcents int64, known bool, err error)`, with
`TurnKey{SessionID, SessionTurnID string}` taken from the turn row, sums
priced `cost_subcents` over `task_usage_events` where `session_id` is the
turn's session and `turn_id` equals the turn's `session_turn_id` and
`cost_source <> 'unpriced'`, through a new task repository method
`SumUsageForTurn(ctx, sessionID, turnID)`. The turn is identified by its
turn id, not by a time window, for two reasons: the usage writer stamps
`occurred_at` when it processes an event, so a row that lands after the turn
settles has an `occurred_at` later than the turn's `finished_at`, which a time
window would exclude; and `finished_at` is the settle time, which can trail
the turn's real end by up to one backstop period, so a time window could
absorb a manager's turn drained onto the same session. A turn id names one
turn, so there is no boundary case between adjacent turns. The unpriced count
is ignored for this display value.

Results: `known` is false, with no query, when `session_turn_id` or `SessionID`
is empty (the send outcome was never learned, or the row has no session), and
the caller stores `cost_subcents` NULL.
A turn with the id and no matching rows is `(0, true, nil)`. Usage rows whose
`turn_id` is empty belong to no turn, so they are counted in
[spend](#measurement) and in no turn's cost. A read error returns
`(0, false, err)`.

When [wake](wake.md#turn-end) or [Stopping](#stopping) settles a turn, a
`TurnCost` error never blocks the settle: the row settles with the outcome
and `cost_subcents` NULL, and the recompute below fills it. The usage writer
is asynchronous, so a turn's last usage row can land after the settle. The
backstop, on every tick for each turn whose `finished_at` is within the last
10 minutes and whose `session_turn_id` is set, recomputes the cost and writes
it with `UPDATE ... SET cost_subcents = ? WHERE id = ? AND outcome IS
NOT NULL AND (cost_subcents IS NULL OR cost_subcents < ?)`. Rows are only
ever added to a turn, so the sum only grows; the conditional update makes a
stale concurrent recompute unable to lower a newer value, and repeating it
is idempotent. The value is final once the turn leaves the 10-minute window,
and no marker is stored: a usage row recorded more than 10 minutes after the
settle is counted in [spend](#measurement) and never in the turn's cost, which
is a display value only. The writer records an event when its single worker dequeues it,
so the window bounds that queue lag, not a routine loss.
A turn whose reads keep failing for those 10 minutes keeps `cost_subcents`
NULL, which the screens show as unknown.

## Stopping

### Observer

`internal/task/usage`'s `Writer` gains `SetRecordedObserver(fn func(ctx
context.Context, taskID, sessionID string))`, callable before or after
`Start`, guarded by the writer's mutex; it replaces any earlier observer and
`nil` clears it (one observer, not a list). The writer owns a bounded channel
of 256 notices and one consumer goroutine, started by the first `Start` (a
repeated `Start` starts nothing more) and joined in `Stop` after the event
worker has drained. After each successful insert the event worker, with an
observer set, offers `(event.TaskID, event.SessionID)` to the channel without
blocking. The pair is the row as the writer built it and submitted to the
repository: the repository may clear a session id it finds owned by another
task, and that cleared value never reaches the writer, so a notice can name a
session the stored row does not carry. That is harmless, because the check it
triggers reads the ledger and decides on spend, never on the notice. When the
channel is full the notice is dropped and
`coordinator_usage_observer_dropped_total` is incremented. With no observer
set nothing is offered and there is no behaviour change.

The consumer reads the current observer under the mutex when it dequeues a
notice, so a notice queued before a replacement runs the new observer, and one
dequeued after a clear is discarded without a metric. It calls `fn` serially in
arrival order, with no coalescing, and a panic in `fn` is recovered and logged
at warn while the consumer continues. Each call receives a context that is a
child of one writer-owned observer context, with a 45-second timeout: longer
than the 30-second cancellation operation TTL, so a cancel in progress is not
cut short by the observer's own deadline (`CancelTurn`'s detached operation
outlives its caller's context in any case). `Stop` closes the channel, cancels
the observer context at once (so a running `fn` returns promptly and the join
cannot hang on it), discards notices still queued, and joins the consumer; the
[backstop](wake.md#backstop) covers what was discarded. A notice offered after
`Stop` began is not offered: the event worker has exited. Registering an
observer after `Stop` has no effect and returns no error. A slow `fn` fills
the channel and drops notices; that is the same bound as any other missed
notice.

The coordinator registers its observer in `registerCoordinatorSpend` at
startup only when phase 3 is effective, and the observer re-checks the
effective flag on each call and returns at once when it is off. It ignores an
empty task id or session id, then reads the task by primary key and ignores
it unless the task's metadata names a coordinator (the one read per usage row
that ordinary tasks cost). A failed task read logs at warn, changes no metric
and ends the call: the backstop's `CheckCeiling` needs no task read and stops
the turn within its period. Otherwise it calls `CheckCeilingForSession`.

### CheckCeiling

`CheckCeiling(ctx, coordinatorID string) error` is the backstop's entry and
`CheckCeilingForSession(ctx, taskID, sessionID string) error` the observer's.
The second resolves the coordinator from the task's metadata, reads the
coordinator's open turn row (the partial unique index makes it at most one),
and returns nil when there is none or when the row's `session_id` is not
`sessionID`; otherwise it runs the same procedure. Both return nil for "nothing
to do", "the turn already ended" and `ErrTurnNotActive`, and return an error
only for a failed read, write or cancel, which the caller logs at warn (the
backstop also counts it in `coordinator_backstop_skipped_total`). The
procedure, with the open turn row and the coordinator re-read at the start
(a coordinator row that is gone by the re-read returns nil: its conversation
and sessions were deleted with it, so nothing is running to stop):

1. **One check per coordinator.** A per-coordinator in-process `TryLock`
   guards the procedure; a call that finds it held returns nil at once, since
   the holder is already acting and the backstop follows. The conditional
   updates below are what protect against another process.
2. **Session turn unknown.** A row whose `session_turn_id` is NULL is never
   acted on: no fence can be applied, so nothing is marked or cancelled
   (wake's message recovery and its `send_failed` settle end such a row).
3. **Marked row.** A row with `stop_requested_at` set skips steps 4 and 5 and
   goes to step 7: it is retried whatever the spend now reads and without
   consulting the ceiling.
4. **Active-turn filter.** Otherwise `GetActiveTurnBySessionID` is read. A read
   error returns that error with nothing marked. No active turn, or an active
   turn whose id is not the row's `session_turn_id`, returns nil: the session
   is running another (attended) turn or the turn ended, and
   [turn end](wake.md#turn-end) settles the row. The check is a cheap filter;
   the fence in step 7 is the guarantee.
5. **Decision.** The ceiling is the coordinator's current
   `cost_ceiling_subcents`, or the row's `start_ceiling_subcents` when that is
   null (a manager turned autonomy off and then cleared the ceiling while the
   turn was open). `start_ceiling_subcents` is `NOT NULL`
   (`store_phase2_schema.go`, [wake](wake.md)), so the two are never both
   absent. It runs `Spend`. An unmeasurable reading, or
   `WindowSubcents` at or above the ceiling, is a stop decision; anything else
   returns nil. The decision is not re-read after step 6: a ceiling raised
   after step 5 still stops this turn, and a ceiling lowered after it is
   caught by the next notice or tick.
6. **Mark.** `UPDATE ... SET stop_requested_at = ? WHERE id = ? AND outcome IS
   NULL AND stop_requested_at IS NULL`, the retry marker. When it affected
   one row it increments `coordinator_ceiling_stop_total{reason}` and logs at
   info, with `reason` `ceiling` (the reading was measurable and at or above
   the ceiling; the log carries the coordinator id, window and ceiling) or
   `unmeasurable` (the log carries the coordinator id and no amounts, because
   an unmeasurable reading's amounts are never inspected). This is the only
   place either happens, so a stop is counted once whichever path later settles
   the row. An
   unmeasurable reading stops the same way, because the ceiling can no longer
   be shown to hold. When it affected no row the row is re-read: settled
   returns nil; open and already marked (another process won the mark)
   continues to step 7.
7. **Cancel.** `CancelTurn(ctx, sessionID, expectedTurnID)` with the row's
   `session_turn_id`. `nil` is a confirmed cancel. `ErrTurnNotActive` returns
   nil without settling: the unattended turn already ended, and
   [turn end](wake.md#turn-end) settles the row `stopped_at_ceiling` because
   it is marked; if that callback was missed, wake's missed-settle
   re-derivation in the same or the next tick settles it, so the delay is at
   most one backstop period. Any other error increments
   `coordinator_ceiling_cancel_failed_total`, leaves the row open and marked
   with its permissions still denied, and is returned; the next notice or tick
   retries from step 3.
8. **Settle.** After a confirmed cancel, in one statement `UPDATE ... SET
   outcome = 'stopped_at_ceiling', finished_at = ?, cost_subcents = ? WHERE id
   = ? AND outcome IS NULL`, with the cost from [TurnCost](#per-turn-cost)
   (NULL when `TurnCost` is unknown or errors; the recompute fills it). Only
   when it affected one row, it increments
   `coordinator_unattended_turn_total{outcome="stopped_at_ceiling"}`, publishes
   `coordinator.updated` with `autonomy_changed`, and, after the update commits,
   calls `Kick(coordinatorID)` best effort as [turn end](wake.md#turn-end)
   does, so a wake recorded during the turn is delivered after the cooldown;
   when it affected none, turn end settled first and owns all three. Exactly one
   of the two settles changes the row, so the outcome is counted once. The
   stop counter of step 6 is separate: it counts the request, this counts the
   settled outcome.

### CancelTurn

`orchestrator.Service.CancelTurn(ctx, sessionID, expectedTurnID string) error`,
with the new exported errors `ErrTurnNotActive` and `ErrCancelInFlight` beside
`ErrSendNowTurnChanged` in the `orchestrator` package, is a new turn-fenced variant of the existing silent cancellation
(`cancelAgentSilentActionWithKindExclusiveConflict` and
`runSilentCancellationOwned` in `orchestrator/event_handlers_clarification.go`),
which already carries an expected turn id and returns `ErrSendNowTurnChanged`
when the captured `cancellationIdentity.turnID` differs. It differs from
`CancelAgent` (the path a manager's Stop uses) in what surrounds the agent
cancel: it does no `authorizeSessionControl` (the caller is internal and has
no principal), posts no cancellation message, evaluates no workflow
completion, and does not invalidate resume attempts. The agent-level cancel
and the reconciliation of the session to `WAITING_FOR_INPUT` are the same,
which is what "as a manager's Stop would" in `AC-COORDINATOR-SPEND-003.2`
means. Its contract:

- An empty `sessionID` or `expectedTurnID` is an error, not
  `ErrTurnNotActive`; `CheckCeiling` never passes one.
- It takes the exclusive cancellation claim for the session with
  `cancellationKindInternal` (the kind the stuck-signal watchdog and context
  reset use). `cancellationKindSilent` is not used, because
  `clarificationRecoveryOwnsStreamEvent` treats that kind as clarification
  recovery and would swallow the cancellation's stream frames. If another
  cancellation of that session is in flight (a manager's Stop, a send-now, a
  reset), the claim is refused and `CancelTurn` returns `ErrCancelInFlight`
  without joining it: joining would skip the fence, and the in-flight
  operation may belong to a different turn. `CheckCeiling` counts it as a
  failed cancel and retries.
- Inside the cancel-in-flight guard it captures the identity and compares
  `identity.turnID` (empty when the session has no active turn) with
  `expectedTurnID`. A difference returns `ErrTurnNotActive` (the existing
  `ErrSendNowTurnChanged` is mapped to it) before the lifecycle cancel, so the
  only effects of a mismatch are the claim and the projection scope, both
  released on return; no persisted change, no runtime call and no state
  transition. A failure to capture the identity is returned as an error.
- On a match it runs the existing agent cancel fenced to the captured prompt
  generation, so a prompt dispatched after the capture is never cancelled,
  then the silent reconciliation. Returning nil means the lifecycle cancel and
  the reconciliation completed, including the existing reconcile-when-no-live-
  execution outcome (nothing is running, so nothing is spending). Any error
  from them, including the caller's context ending while the detached
  operation continues, is an error return and a failed cancel. The operation
  runs on a context detached from the caller with the 30-second
  `cancellationOperationTTL`, so a caller whose context ended still leaves it
  running and it releases its own claim when it finishes. A repeated cancel is
  safe: the next call either finds the claim held (`ErrCancelInFlight`, counted
  and retried) or finds the turn no longer active (`ErrTurnNotActive`, nil), and
  the fence means it can never cancel another turn.

A test pins the drain race: with the pre-check seeing the unattended turn
active and a manager's queued message becoming the active turn before the
guard, `CancelTurn` returns `ErrTurnNotActive` and the manager's turn keeps
running. The phase 1 message send path has no ceiling check.

### Bound

The backstop runs `CheckCeiling` for every open unattended turn each tick,
whether or not autonomy is still on ([wake backstop](wake.md#backstop)), so
a dropped observer notice or a failed cancel delays the stop by at most 60
seconds (`AC-COORDINATOR-SPEND-003.2`). Both acting paths cancel only through
`CancelTurn` fenced to the row's `session_turn_id`, so an attended turn,
including a manager's queued message drained on the same session at any point
before the cancel, is never cancelled (`AC-COORDINATOR-SPEND-003.3`).

The overshoot is bounded by one usage report, or by one backstop period when
an observer notice is dropped or one cancel fails: the ceiling is compared after
each recorded usage event, not before a model request, since Kandev does not
mediate the agent CLI's model calls. The bound holds only while a cancel
eventually succeeds. When cancels keep failing (the agent process ignores
them), the turn keeps spending until it ends by itself; Kandev has no second
stop below the agent cancel. That residual is made visible, not hidden: each
failed cancel increments `coordinator_ceiling_cancel_failed_total`, and once
a row has `stop_requested_at` older than five minutes and is still open, the
autonomy read reports `last_turn.stop_state: "stop_failing"` and the autonomy strip
shows "Stop at ceiling not confirmed: the turn is still running" with the
panel's Stop, so a manager can act (`AC-COORDINATOR-SPEND-003.4`, task 06).
Admission already holds new unattended turns while any turn is open, so the
failing turn is the only one spending.

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
| Task list or 24-hour usage read error | Not measurable: admission holds with `spend_unmeasured`; an open unattended turn is stopped; `coordinator_spend_read_failed_total` |
| Seven-day mean read error | The mean shows unavailable; nothing is held or stopped |
| Unpriced usage in the window | Not measurable: admission holds with `spend_unmeasured`; an open unattended turn is stopped; until the row leaves the window |
| Observer queue full | Metric; the backstop applies the stop within 60 s |
| Cancel fails, or another cancellation of the session is in flight | Row stays open with `stop_requested_at`; each failure is counted; the next observer call or tick cancels again |
| Cancel keeps failing for 5 minutes | Metric per failure; autonomy read and strip show `stop_failing` until the turn ends |
| Unattended turn ended before the cancel | `ErrTurnNotActive`; nothing is cancelled; turn end settles the row |
| Late usage row after settle | Per-turn cost, keyed by turn id, corrected by the backstop each tick for 10 minutes |
| Turn cost read fails at settle | Row settles with `cost_subcents` NULL; the recompute fills it |

## Security

The ceiling is written only by the PATCH route (`workspace.manage`). Spend
reads are `workspace.read` through the autonomy route; they return amounts,
never per-message usage.

## Observability

`coordinator_ceiling_stop_total` (label `reason`: `ceiling`, `unmeasurable`; stops requested, counted at the mark),
`coordinator_ceiling_cancel_failed_total`,
`coordinator_spend_read_failed_total` (label `read`: `tasks`, `window`,
`mean`) and `coordinator_usage_observer_dropped_total` counters, plus a
structured zap log at info for each ceiling stop request: the coordinator id,
window and ceiling for `reason` `ceiling`, the coordinator id alone for
`unmeasurable`. A `stopped_at_ceiling` settle also increments wake's
`coordinator_unattended_turn_total{outcome}`.

## Related decisions

- [Coordinator phase 3: autonomy](../../../decisions/2026-09-29-coordinator-phase-3-autonomy.md)
- [Instance-wide claim lock and budget fail mode asymmetry](../../../decisions/2026-09-16-instance-wide-claim-lock-and-budget-fail-mode-asymmetry.md)
