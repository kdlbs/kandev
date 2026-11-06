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
  - AC-OFFICE-RUN-SETTLE-001.4
  - AC-OFFICE-RUN-SETTLE-001.5
  - AC-OFFICE-RUN-SETTLE-001.6
  - AC-OFFICE-RUN-SETTLE-001.7
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

- Office classification, primary selection and routing policy are unchanged.
  Teardown ownership and the existing session lock remain authoritative.

## Acceptance

- A clean per-launch `RUNNING -> WAITING_FOR_INPUT` finishes the matching run.
- An Office park retains its claim until runtime settlement. Cancelled and
  no-execution paths still release through completion confirmation; failed
  teardown does not emit success, and a successor claim stays protected.
- Late automatic failures preserve an already completed task and its history.
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

## Runtime settlement follow-up

A real successful turn released its claim at the park before teardown; a
queued comment then configured the same live execution, which correctly
refused startup. Its late failure also overwrote the completed business task.
The follow-up keeps Routine settlement and the original cancellation, absent
execution and stale-teardown contracts, but moves Office release to its actual
runtime completion. Failed bootstrap cleanup is recorded as failure.

The existing park test now asserts claim/checkout retention followed by
exact-once release. Four existing OfficeTurnComplete tests check publication
after stop, no publication on stop failure, and no-execution/cancel completion.
The existing task-state CAS test also checks a late FAILED projection. No new
standing suite or replay of business work is introduced. Focused public-source
race checks passed: Office service 2.278s, orchestrator 5.007s and SQLite
repository 5.705s. The selection includes the original park, cancellation,
missing-execution, stale teardown, duplicate launch, error extraction, retained
failure and task-state ownership controls. Documentation coverage is covered;
publication uses the normal commit hooks.

After the normal upstream merge, the focused race selection passed again on
the updated backend and dependencies: Office service 2.135s, orchestrator
2.506s and SQLite repository 2.900s. The configuration conflict retains both
the existing Office verdict flag and the incoming background-work flag. The
new merged packages also prepared their normal fts5/e2e test exports before
the normal merge hooks; this preparation does not execute test assertions.
