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
  idempotent recompute the backstop runs for turns settled in the last 11
  minutes, `SumUsageForTurn(ctx, sessionID, turnID, notAfter)` counting rows
  whose `occurred_at` is no later than `FinishedAt + 10 minutes` ([Per-turn cost](../../specs/coordinator/system-design/spend.md#per-turn-cost)).
- Task repository `SumUsageForTasks` (at most 500 ids per call) and the
  `Spend` error and unmeasurable contract, `CheckSpendMeasurable` and
  `CheckCeilingNotReached`
  ([Measurement](../../specs/coordinator/system-design/spend.md#measurement)).
- `internal/task/usage`: `Writer.SetRecordedObserver`, one replaceable
  observer fed by a writer-owned bounded channel of 256 and one consumer
  goroutine joined at `Stop`, `fn(ctx, taskID, sessionID)` called with a
  45-second child of a writer-owned context that `Stop` cancels, with the
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
  existing silent reconciliation (`finishSilentCancelledAgentTurn`) with
  `cancellationKindInternal`, no authorization, message or workflow
  completion, the agent-level cancel prompt-fenced through
  `GetPromptActivityForSession` and `cancelAgentWhileUnlockedForPrompt` as the
  stuck-signal watchdog does, a `lifecycle.ErrCancelEscalated` result counted as
  a confirmed cancel, and a pre-claim active-turn check; `ErrTurnNotActive` and
  `ErrCancelInFlight` are exported by the `orchestrator` package), count each failed
  cancel in `coordinator_ceiling_cancel_failed_total`, and only after a
  confirmed cancel settle
  it `stopped_at_ceiling` with cost in one conditional update and, only when it
  changed a row, count `coordinator_unattended_turn_total{outcome}`, publish
  `coordinator.updated` with `autonomy_changed` and call `Kick`; a failed
  cancel leaves the marked row open for the next call
  ([Stopping](../../specs/coordinator/system-design/spend.md#stopping)).
  The observer calls it; task 05 wires it into the backstop tick.

## Out of scope

- Admission ordering (task 05) and every screen (task 06).
- Office budgets and cost events.

## Acceptance

Every check below calls `CheckCeiling`, `CheckCeilingForSession`, `Spend`,
`TurnCost`, `CancelTurn` or the writer directly, with fakes for the orchestrator
cancel and the clock. The usage writer has no clock: tests set `occurred_at`
and `FinishedAt` directly on the rows they insert. The turn-end subscriber, the backstop tick and the
delivery path belong to task 05, and permission containment to task 02, so
their behaviour is asserted there (`synctest` tick tests in task 05's
acceptance), not here.

- Spend sums usage of current and archived conversation tasks of this
  coordinator only; a task of another coordinator or an ordinary task with
  usage is excluded; the window is half-open; an unpriced row or a query error
  makes it not measurable; `ErrSpendScope` is unmeasurable, runs no query and
  is not counted in `coordinator_spend_read_failed_total`.
- A usage row that takes the window to the ceiling during an open unattended
  turn: the observer calls `CheckCeilingForSession`, which marks the row,
  cancels the session once through `CancelTurn` and settles it
  `stopped_at_ceiling`. With the observer queue full the notice is dropped and
  counted, and a direct `CheckCeiling` call (the call task 05's tick makes)
  stops the same turn. A cancel that fails leaves the row open with
  `stop_requested_at`, and the next `CheckCeiling` call cancels and settles it
  whatever the spend then reads. The settle also counts the outcome and calls
  `Kick`, and does so only from the settle that changed the row.
- The concurrency races are named: (1) two concurrent `CheckCeiling` calls on
  one coordinator, one from the observer and one direct, call `CancelTurn`
  once; (2) the `turn.completed` settle of the same turn arriving after the
  confirmed cancel and before the step 8 update: exactly one of the two settles
  changes the row, the outcome is `stopped_at_ceiling` because it is marked, and
  the outcome counter, the publish and the `Kick` happen once; (3) the turn
  ending before step 7: `CancelTurn` returns `ErrTurnNotActive`, nothing is
  cancelled, and `CheckCeiling` returns nil and leaves the marked row open.
  Race 2 is driven by a stand-in that settles the row between the confirmed
  cancel and the step 8 update, since the real turn-end subscriber belongs to
  task 05.
- A session with no open unattended turn row is never cancelled, whatever the
  spend: an attended turn over the ceiling is not cancelled. A manager's queued
  message that became the session's active turn while the turn row is still
  open is not cancelled (`ErrTurnNotActive`).
- Drain race: with the pre-check seeing the unattended turn active and a
  manager's message becoming the active turn before `CancelTurn` enters the
  guard, `CancelTurn` returns `ErrTurnNotActive`, the manager's turn keeps
  running, and the row is not settled by `CheckCeiling`
  (`AC-COORDINATOR-SPEND-003.3`). An orchestrator test covers `CancelTurn`'s
  match, mismatch, no-active-turn and `ErrCancelInFlight` cases and the
  caller's context ending while the detached operation continues, a cancel
  reported as escalated (treated as confirmed), and a new prompt starting
  after the capture (`ErrPromptActivityNotOwned`, returned as `ErrTurnNotActive`:
  nothing cancelled, nothing counted).
- Each failed cancel, including `ErrCancelInFlight`, increments
  `coordinator_ceiling_cancel_failed_total`; `ErrTurnNotActive` does not. The
  stop counter and info log fire once per turn at the mark with `reason`
  `ceiling` or `unmeasurable`, and the unmeasurable log carries no amounts.
- Per-turn cost is keyed on the turn id: `TurnCost` counts a usage row
  recorded after the settle with `occurred_at` up to ten minutes past
  `FinishedAt` inclusive, excludes one past that, and not a manager turn drained onto the same
  session, and is `known=false` for an empty session or turn id. The recompute
  function fills a NULL cost and never lowers it. A failed 7-day read leaves
  spend measurable.
- Observer: `Stop` cancels a running `fn` and joins; a replaced observer runs
  for notices queued before the replacement; a cleared one discards them; a
  panic is recovered; a failed task read logs and ends the call.
- A row with `session_turn_id` NULL is never marked or cancelled. A failed
  `GetActiveTurnBySessionID` read marks nothing. A coordinator row gone at the
  re-read returns nil.

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
