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
- `TurnCost(ctx, turn)` ([Per-turn cost](../../specs/coordinator/system-design/spend.md#per-turn-cost)).
- `internal/task/usage`: an optional `OnRecorded(taskID, sessionID)` observer
  on a bounded channel of 256 with a drop metric; no behaviour change when
  nothing registers.
- `CheckCeiling(ctx, coordinatorID)`: for an open turn, when not measurable
  or at or over the ceiling, settle it `stopped_at_ceiling` with cost in one
  conditional update, cancel through the orchestrator's cancel, publish
  `coordinator.updated` with `autonomy_changed`
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
  the backstop stops it within one tick (`synctest`).
- A session with no open unattended turn row is never cancelled, whatever the
  spend: an attended turn over the ceiling runs to completion.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Spend|Ceiling|TurnCost' -count=1
cd apps/backend && go test ./internal/task/usage/... -count=1
make -C apps/backend lint
```

## Risks

- The observer must never block the usage writer: the enqueue is
  non-blocking and a test fills the queue.
