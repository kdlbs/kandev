---
id: "01-live-runs-agent-profile"
title: "Read live runs from session agent profiles"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-DASHBOARD-001
acceptance_criteria:
  - AC-OFFICE-DASHBOARD-001.8
  - AC-OFFICE-DASHBOARD-001.9
system_design:
  - ../../specs/office/system-design/run-observation.md
---

# Task 01: Read live runs from session agent profiles

## Summary

Make the live-runs endpoint read the run's agent from
`task_sessions.agent_profile_id` instead of the dropped
`task_sessions.agent_execution_id` column, so the endpoint works on migrated
databases and renders agent names.

## In scope

- `QueryRecentSessions` selects `COALESCE(ts.agent_profile_id,'')` into
  `LiveSessionRow.AgentProfileID`.
- `GetLiveRuns` batches agent lookup by `AgentProfileID` and returns it as the
  run's `agentId` with the resolved `agentName`.
- Office dashboard and repository test fixtures no longer declare
  `task_sessions.agent_execution_id`.

## Out of scope

Agent summaries, frontend rendering, schema migrations.

## Acceptance

1. `GET /workspaces/:wsId/live-runs` returns 200 against the migrated
   `task_sessions` column set.
2. A run whose session carries an Office agent ID in `agent_profile_id` returns
   that ID and the agent's current name.

## Verification

- `go test ./internal/office/dashboard/ ./internal/office/repository/sqlite/`
  with `-run 'TestLiveRuns|TestQueryRecentSessions'`: four live-runs tests
  return 500 `no such column: ts.agent_execution_id` on the previous source and
  pass with the change.

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/dashboard.go`
- `apps/backend/internal/office/repository/sqlite/dashboard_test.go`
- `apps/backend/internal/office/dashboard/service_agents.go`
- `apps/backend/internal/office/dashboard/run_activity_test.go`
- `apps/backend/internal/office/dashboard/handler_test.go`
- `apps/backend/internal/office/dashboard/agent_summaries_test.go`
- `docs/specs/office/requirements/dashboard.md`
- `docs/specs/office/system-design/run-observation.md`

## Dependencies

None.

## Inputs

- [Owning requirement](../../specs/office/requirements/dashboard.md)
- [Owning design](../../specs/office/system-design/run-observation.md)
- [Plan](plan.md)

## Risks

Sessions without an Office agent keep an empty agent ID and name.

## Parallelism

`sequential`
