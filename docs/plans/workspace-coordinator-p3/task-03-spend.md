---
id: "03-spend"
title: "Spend measurement and the ceiling stop"
status: pending
wave: 2
depends_on:
  - "01-flag-schema-settings"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-SPEND-002
  - REQ-COORDINATOR-SPEND-003
acceptance_criteria:
  - AC-COORDINATOR-SPEND-002.1
  - AC-COORDINATOR-SPEND-002.2
  - AC-COORDINATOR-SPEND-002.3
  - AC-COORDINATOR-SPEND-003.1
  - AC-COORDINATOR-SPEND-003.2
  - AC-COORDINATOR-SPEND-003.3
system_design:
  - ../../specs/coordinator/system-design/spend.md
---

# Task 03: Spend Measurement And The Ceiling Stop (WP-12)

## Summary

Measures coordinator spend from `task_usage_events`, supplies admission checks
3 and 4 as functions task 05 calls, computes per-turn cost, and stops an open
unattended turn at the ceiling through a post-commit usage observer and a
backstop re-check.

## In scope

- `internal/coordinator/spend.go`: `Spend(ctx, coordinator, now)` over every
  conversation task of the coordinator (`ListCoordinatorOriginTasks`
  filtered), the window SQL with chunked `IN` lists, the 7-day mean, and
  `Measurable`/`Degraded`
  ([Measurement](../../specs/coordinator/system-design/spend.md#measurement)).
- `TurnCost(ctx, TurnKey) (subcents, known, err)` keyed on the turn's
  `session_turn_id` through a new task repository `SumUsageForTurn`, and the
  idempotent recompute the backstop runs for turns settled in the last 10
  minutes ([Per-turn cost](../../specs/coordinator/system-design/spend.md#per-turn-cost)).
- Task repository `SumUsageForTasks` (at most 500 ids per call) and the
  `Spend` error and unmeasurable contract, `CheckSpendMeasurable` and
  `CheckCeilingNotReached`
  ([Measurement](../../specs/coordinator/system-design/spend.md#measurement)).
- `internal/task/usage`: `Writer.SetRecordedObserver`, one replaceable
  observer fed by a writer-owned bounded channel of 256 and one consumer
  goroutine joined at `Stop`, with the
  `coordinator_usage_observer_dropped_total` metric and panic recovery; no
  behaviour change when nothing registers
  ([Observer](../../specs/coordinator/system-design/spend.md#observer)).
- `CheckCeiling(ctx, coordinatorID) error` and
  `CheckCeilingForSession(ctx, taskID, sessionID) error` (procedure steps 1-8
  of [CheckCeiling](../../specs/coordinator/system-design/spend.md#checkceiling)):
  for an open turn whose
  `session_turn_id` is the session's active turn, when not measurable or at
  or over the ceiling (or already marked), set `stop_requested_at`, cancel
  through the new turn-fenced `orchestrator.Service.CancelTurn(ctx,
  sessionID, expectedTurnID)` (compares the captured
  `cancellationIdentity.turnID` inside the cancel-in-flight guard, returns
  `ErrTurnNotActive` without cancelling on a mismatch, `ErrCancelInFlight`
  when another cancellation of the session holds the claim; built on the
  existing silent turn-fenced cancellation, no authorization, message or
  workflow completion), count each failed
  cancel in `coordinator_ceiling_cancel_failed_total`, and only after a
  confirmed cancel settle
  it `stopped_at_ceiling` with cost in one conditional update and publish
  `coordinator.updated` with `autonomy_changed`; a failed cancel leaves the
  marked row open for the next call
  ([Stopping](../../specs/coordinator/system-design/spend.md#stopping)).
  The observer calls it; task 05 wires it into the backstop tick.

## Out of scope

- Admission ordering (task 05) and every screen (task 06).
- Office budgets and cost events.

## Acceptance

- Spend sums usage of current and archived conversation tasks of this
  coordinator only; a task of another coordinator or an ordinary task with
  usage is excluded; the window is half-open; an unpriced row or a query error
  makes it not measurable.
- A usage row that takes the window to the ceiling during an open unattended
  turn settles it `stopped_at_ceiling` and cancels the session once, even when
  the session's own state change races the stop; with the observer queue full
  the backstop stops it within one tick (`synctest`). A cancel that fails
  leaves the row open with `stop_requested_at` and its permissions still
  denied; the next tick cancels and settles it. A turn that ends by itself
  after the mark settles `stopped_at_ceiling`.
- A session with no open unattended turn row is never cancelled, whatever the
  spend: an attended turn over the ceiling runs to completion. A manager's
  queued message drained on the unattended turn's session, while the turn row
  is still open because its settle event was lost, runs as an attended turn:
  it is not cancelled and its permissions reach the panel.
- Drain race: with the pre-check seeing the unattended turn active and a
  manager's message becoming the active turn before `CancelTurn` enters the
  guard, `CancelTurn` returns `ErrTurnNotActive`, the manager's turn keeps
  running, and the row is not settled by `CheckCeiling`
  (`AC-COORDINATOR-SPEND-003.3`). An orchestrator test covers `CancelTurn`'s
  match, mismatch and no-active-turn cases.
- Each failed cancel, including `ErrCancelInFlight`, increments
  `coordinator_ceiling_cancel_failed_total`; `ErrTurnNotActive` does not.
- Per-turn cost is keyed on the turn id: a usage row recorded after the settle
  and a manager turn drained onto the same session are handled (the first is
  counted by the recompute, the second is not counted). A failed 7-day read
  leaves spend measurable. Two concurrent `CheckCeiling` calls on one
  coordinator cancel once; the stop counter and info log fire once per turn;
  `autonomy_changed` publishes only from the settle that changed a row.
- A row with `session_turn_id` NULL is never marked or cancelled. A failed
  `GetActiveTurnBySessionID` read marks nothing.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Spend|Ceiling|TurnCost' -count=1
cd apps/backend && go test ./internal/task/usage/... -count=1
cd apps/backend && go test ./internal/orchestrator/... -run 'CancelTurn' -count=1
make -C apps/backend lint
```

## Risks

- The observer must never block the usage writer: the enqueue is
  non-blocking and a test fills the queue.
