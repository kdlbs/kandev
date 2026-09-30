---
id: "01-turn-ledger"
title: "Turn ledger, stamp and query tool"
status: draft
wave: 1
depends_on:
  - "phase 3 merged"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-TURN-LEDGER-001
  - REQ-COORDINATOR-TURN-LEDGER-002
  - REQ-COORDINATOR-TURN-LEDGER-003
  - REQ-COORDINATOR-TURN-LEDGER-004
  - REQ-COORDINATOR-TURN-LEDGER-005
  - REQ-COORDINATOR-TURN-LEDGER-006
acceptance_criteria:
  - AC-COORDINATOR-TURN-LEDGER-001.1
  - AC-COORDINATOR-TURN-LEDGER-001.2
  - AC-COORDINATOR-TURN-LEDGER-001.3
  - AC-COORDINATOR-TURN-LEDGER-001.4
  - AC-COORDINATOR-TURN-LEDGER-001.5
  - AC-COORDINATOR-TURN-LEDGER-001.6
  - AC-COORDINATOR-TURN-LEDGER-001.7
  - AC-COORDINATOR-TURN-LEDGER-002.1
  - AC-COORDINATOR-TURN-LEDGER-002.2
  - AC-COORDINATOR-TURN-LEDGER-002.3
  - AC-COORDINATOR-TURN-LEDGER-002.4
  - AC-COORDINATOR-TURN-LEDGER-002.5
  - AC-COORDINATOR-TURN-LEDGER-003.1
  - AC-COORDINATOR-TURN-LEDGER-003.2
  - AC-COORDINATOR-TURN-LEDGER-003.3
  - AC-COORDINATOR-TURN-LEDGER-004.1
  - AC-COORDINATOR-TURN-LEDGER-004.2
  - AC-COORDINATOR-TURN-LEDGER-004.3
  - AC-COORDINATOR-TURN-LEDGER-004.4
  - AC-COORDINATOR-TURN-LEDGER-004.5
  - AC-COORDINATOR-TURN-LEDGER-005.1
  - AC-COORDINATOR-TURN-LEDGER-005.2
  - AC-COORDINATOR-TURN-LEDGER-005.3
  - AC-COORDINATOR-TURN-LEDGER-005.4
  - AC-COORDINATOR-TURN-LEDGER-006.1
  - AC-COORDINATOR-TURN-LEDGER-006.2
  - AC-COORDINATOR-TURN-LEDGER-006.3
system_design:
  - ../../specs/coordinator/system-design/turn-ledger.md
---

# Task 01: Turn ledger, stamp and query tool (WP 3.1-1)

## Summary

Records one row per coordinator turn with its stamp, call digest and frozen
board snapshot, links proposals and log rows to the turn, adds the read-only
`list_coordinator_turns_kandev` tool, the phase 3.1 flag and the retention
job. Recording ships ahead of the flag.

## In scope

- `internal/coordinator/ledger/`: `Recorder` (observes `turn.started` and
  `turn.completed`), `Verdict` (pure), `Stamp`, the async `Call` queue, the
  in-memory `ActiveTurnID` map, `Reader`, `safe(stage, fn)`, the server-side
  `board.Snapshot` query (a new query, not a port of the client projection),
  and the daily settle-and-retention job, with tests beside each.
- Store: `coordinator_turns`, `coordinator_turn_calls`,
  `coordinator_turn_snapshots`, nullable `turn_id` on `coordinator_proposals`
  and `coordinator_activity`, `ledger_turn_id` on
  `coordinator_unattended_turns`; additive migrations replayable on SQLite and
  PostgreSQL
  ([Migration](../../specs/coordinator/system-design/turn-ledger.md#migration)).
- The guarded-call layer calls `ledger.Call` at its allow/refuse decision
  point; the proposal and activity inserts read `ActiveTurnID`.
- `features.coordinatorPhase31` in `internal/runtimeflags/registry.go` and
  root `profiles.yaml` (off in `prod`, `dev` and `e2e`), through
  `/runtime-feature-flags`; the tool profile extension that adds
  `list_coordinator_turns_kandev` when the flag is effective; a phase 3.1
  registration function in `internal/backendapp/coordinator.go` that later
  work orders extend.
- Metrics `coordinator_ledger_write_failed_total{stage}`.

## Out of scope

- Outcomes, overrides, replay, dream, Pause and Projects.
- Any screen that browses the ledger.
## Acceptance

- One ledger row per session turn under redelivery and concurrent completion,
  on SQLite and PostgreSQL; first completion wins; the model set once.
- The verdict table covers every precedence pair, including a turn with no
  calls and a turn whose calls were all refused.
- The 101st call is dropped and `calls_truncated` set; a failing write never
  fails or delays a turn, proposal or decision.
- Flag off: rows are still written and the tool, routes and prompt text are
  absent; a phase 3 database upgrades and keeps its rows.
- The tool returns own-coordinator rows only, in the documented order, with
  the validation errors and the watch-set restrictions of `004`.

## Validation

- `make -C apps/backend test` for the touched packages, with `synctest` for any timer and no `time.Sleep`; store conformance on SQLite and PostgreSQL for each new table or column.
- `python3 scripts/list-docs.py validate` if a specification changes.
