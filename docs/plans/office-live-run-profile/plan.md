---
created: 2026-10-20
status: done
requirements:
  - REQ-OFFICE-DASHBOARD-001
system_design:
  - ../../specs/office/system-design/run-observation.md
legacy_specs: []
---

# Implementation Plan: Read Office live runs from session agent profiles

## Overview

`GET /workspaces/:wsId/live-runs` selected `task_sessions.agent_execution_id`,
a column the task repository migration already drops. Against a migrated
database the endpoint returned HTTP 500 (`no such column:
ts.agent_execution_id`), and even where the column survived it never matched an
Office agent ID, so runs had no agent name. One work order switches the read to
`task_sessions.agent_profile_id` and aligns the Office test fixtures with the
migrated schema.

## Scope

In scope: `apps/backend/internal/office/repository/sqlite/dashboard.go`
(`LiveSessionRow`, `QueryRecentSessions`),
`apps/backend/internal/office/dashboard/service_agents.go` (`GetLiveRuns`), the
Office dashboard test fixtures, and the dashboard requirement and
run-observation design.

Excluded: the agent-summaries endpoint (already keyed by `agent_profile_id`),
the frontend, and any schema migration.

## Technical approach

Rename the live-session row field to `AgentProfileID` bound to
`agent_profile_id`, and key the batched agent-name lookup by it. Test fixtures
drop the retired `agent_execution_id` column so the existing live-runs tests
exercise the production column set.

## Work orders

- [x] [Task 01: Read live runs from session agent profiles](task-01-live-runs-agent-profile.md)

## Changed-path coverage

The work order owns the two Office backend files above, the Office dashboard
and repository test fixtures, the extended
`docs/specs/office/requirements/dashboard.md` acceptance criteria, and the
`docs/specs/office/system-design/run-observation.md` section.

## Verification

`TestLiveRuns_ResolvesAgentFromSessionProfile` in
`internal/office/dashboard/run_activity_test.go` and
`TestQueryRecentSessions_ScopesByWorkspaceAndOrders` in
`internal/office/repository/sqlite/dashboard_test.go` fail on the previous
source and pass with the change.

## Risks

- Sessions created without an Office agent keep an empty agent ID and name,
  matching the previous empty-default behavior.
