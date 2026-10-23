---
id: "01-parked-session-settlement"
title: "Settle parked launched sessions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-RUN-SETTLE-001
acceptance_criteria:
  - AC-OFFICE-RUN-SETTLE-001.1
  - AC-OFFICE-RUN-SETTLE-001.2
  - AC-OFFICE-RUN-SETTLE-001.3
system_design:
  - ../../specs/office/system-design/run-settlement-integrity.md
---

# Task 01: Settle parked launched sessions

## Outcome

A claimed task run finishes when the session it launched parks after a
successful turn on the per-launch session model.

## In scope

- Subscribe Office to `task_session.state_changed`.
- Share the AgentCompleted completion body with park settlement.

## Exclusions

- Office `IDLE` teardown and process-exit completion paths are unchanged.

## Acceptance

- A clean `RUNNING -> WAITING_FOR_INPUT` of the launched session finishes the
  run and releases its checkout.
- Errored parks, foreign sessions, non-turn transitions, and `IDLE` leave the
  run claimed.

## Verification

```bash
cd apps/backend
go test -count=1 ./internal/office/service/ -run 'TestParked'
```

## Likely files

- `apps/backend/internal/office/service/run_session_park_settlement.go`
- `apps/backend/internal/office/service/event_subscribers.go`

## Results

`TestParkedLaunchedSessionSettlesClaimedRun` failed before the change
(`run status = "claimed", want finished`) and passes after it, with
`TestParkedSessionSettlementIgnoresFailuresAndForeignSessions`.
