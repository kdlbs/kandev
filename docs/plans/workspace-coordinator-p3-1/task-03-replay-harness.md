---
id: "03-replay-harness"
title: "Replay harness, regression guard and planted suite"
status: draft
wave: 2
depends_on:
  - "01-turn-ledger"
  - "02-outcomes-overrides"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-REPLAY-001
  - REQ-COORDINATOR-REPLAY-002
  - REQ-COORDINATOR-REPLAY-003
  - REQ-COORDINATOR-REPLAY-004
  - REQ-COORDINATOR-REPLAY-005
acceptance_criteria:
  - AC-COORDINATOR-REPLAY-001.1
  - AC-COORDINATOR-REPLAY-001.2
  - AC-COORDINATOR-REPLAY-001.3
  - AC-COORDINATOR-REPLAY-001.4
  - AC-COORDINATOR-REPLAY-002.1
  - AC-COORDINATOR-REPLAY-002.2
  - AC-COORDINATOR-REPLAY-002.3
  - AC-COORDINATOR-REPLAY-002.4
  - AC-COORDINATOR-REPLAY-002.5
  - AC-COORDINATOR-REPLAY-002.6
  - AC-COORDINATOR-REPLAY-003.1
  - AC-COORDINATOR-REPLAY-003.2
  - AC-COORDINATOR-REPLAY-003.3
  - AC-COORDINATOR-REPLAY-004.1
  - AC-COORDINATOR-REPLAY-004.2
  - AC-COORDINATOR-REPLAY-004.3
  - AC-COORDINATOR-REPLAY-004.4
  - AC-COORDINATOR-REPLAY-005.1
  - AC-COORDINATOR-REPLAY-005.2
  - AC-COORDINATOR-REPLAY-005.3
  - AC-COORDINATOR-REPLAY-005.5
system_design:
  - ../../specs/coordinator/system-design/replay.md
---

# Task 03: Replay harness, regression guard and planted suite (WP 3.1-3)

## Summary

Builds the replay library: case selection, the one-prompt sandbox through
`hostutility` with a structured answer, cost pricing and the spend reader's
additive `ExtraSpend` term, three-run scoring, the regression guard, the
improvement judge, the result row, and the planted-regression suite that runs
in CI.

## In scope

- `internal/coordinator/replay/`: `cases`, `stub` (answer parser), `budget`,
  `Run`, `Key`, the guard and judge, `constants.go`, and `testdata/planted/`
  with the deterministic stub model.
- Store: `coordinator_replay_results` and its retention.
- The import-boundary test that fails when `replay` or `stub` imports a writer
  of proposals, activity, settings or conversation.
- `TestPlantedCandidates` in the ordinary backend test run and a constants
  test.

## Out of scope

- Any gate on an approval, model change, setting or standing order (phase 3.5).
- A candidate picker, a route or a tool for replay.
## Acceptance

- Selection order and ties, skip-reason precedence and skipped cases counted
  in no score.
- Key normalisation, score arithmetic, median of three, and identical results
  for two replays with a deterministic model.
- Guard flips at 1, 2 and 3 of 3 runs; an approved-with-edits proposal never
  blocks; no case that ran gives `unmeasured`.
- Judge boundaries at 0.049, 0.05, 19 and 20 held-out cases, in integer
  thousandths.
- The budget stops a replay `unmeasured` (`budget`) when spend is unmeasurable
  or would reach the ceiling.
- The planted suite fails when the guard is disabled and passes with every bad
  candidate blocked and the good one not blocked.

## Validation

- `make -C apps/backend test` for the touched packages, with `synctest` for any timer and no `time.Sleep`; store conformance on SQLite and PostgreSQL for each new table or column.
- `python3 scripts/list-docs.py validate` if a specification changes.
