---
id: "03-agent-error-session-verdict"
title: "Agent-error session verdict guard"
status: done
wave: 2
depends_on: ["01-parked-session-settlement"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-RUN-SETTLE-003
acceptance_criteria:
  - AC-OFFICE-RUN-SETTLE-003.1
  - AC-OFFICE-RUN-SETTLE-003.2
  - AC-OFFICE-RUN-SETTLE-003.3
  - AC-OFFICE-RUN-SETTLE-003.4
  - AC-OFFICE-RUN-SETTLE-003.5
  - AC-OFFICE-RUN-SETTLE-003.6
system_design:
  - ../../specs/office/system-design/run-settlement-integrity.md
---

# Task 03: Agent-error session verdict guard

## Outcome

With `features.officeAgentErrorSessionVerdict` on, a task run's failure and its
`on_agent_error` replacement wait for the failed session's verdict.

## In scope

- Runtime flag registration, profiles, frontend defaults, and public docs.
- Parked failures resolved by session state, a bounded re-check, or a later
  completion.

## Exclusions

- Dynamic routing's failover decision; restart persistence.

## Acceptance

- A successor launch keeps the run claimed until the successor settles it.
- A recorded failure verdict applies the existing failure path once.
- The flag off keeps the immediate failure path.

## Verification

```bash
cd apps/backend
go test -count=1 -race ./internal/office/service/ -run 'TestAgentErrorSessionVerdict'
go test -count=1 ./internal/runtimeflags/ ./internal/profiles/
```

## Likely files

- `apps/backend/internal/office/service/agent_error_session_verdict.go`
- `apps/backend/internal/runtimeflags/registry.go`
- `apps/backend/internal/profiles/profiles.yaml`

## Results

`TestAgentErrorSessionVerdict_SuccessorKeepsRunClaimed` failed before the guard
(`run status after AgentFailed = "failed", want claimed`) and passes after it,
with the failure-verdict, recorded-verdict, disabled, re-check, and read-error
re-check retention tests.
