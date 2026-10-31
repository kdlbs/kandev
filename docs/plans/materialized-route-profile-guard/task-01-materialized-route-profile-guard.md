---
id: "01-materialized-route-profile-guard"
title: "Refuse a profile change on a materialized dynamic route"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.23
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---

# Task 01: Refuse a profile change on a materialized dynamic route

## Summary

Before `startCreatedSession` persists a changed logical profile, load the
session's durable route and refuse the start when its materialized generation
belongs to a different logical profile. Protect that check and prepared-session
route materialization with the same session lifecycle lock. Report the expected
refusal as a WebSocket validation error.

## In scope

- `ErrMaterializedRouteProfileChange` and
  `Service.refuseMaterializedRouteProfileChange`.
- The call in `startCreatedSession` after profile validation and before the
  session profile write.
- The prepared-session route read, claim, and projection write.
- The `session.launch` validation response and its focused regression test.

## Out of scope

The workflow profile-switch session handoff, concrete profiles, route actions,
and route state migration.

## Verification

- `go test -trimpath -run 'Test(PreparedRouteMaterializationCannotRaceProfileChange|ProfileChangeBeforePreparedRouteMaterializationKeepsNewRoute|StartCreatedSession_RefusesProfileChangeOnMaterializedDynamicRoute|StartCreatedSession_MaterializedRouteGuardKeepsSameProfileAndFirstSelection)$' -count=1 ./internal/orchestrator`
- `go test -trimpath -run 'TestMaterializedRouteProfileChangeReportsValidation|TestWSRouteActionReportsNonDynamicProfileAsValidation$' -count=1 ./internal/orchestrator/handlers`
- `go test -trimpath -race -run 'Test(PreparedRouteMaterializationCannotRaceProfileChange|ProfileChangeBeforePreparedRouteMaterializationKeepsNewRoute)$' -count=1 ./internal/orchestrator`
- Changed-file Go lint for the orchestrator and handler files.

## Results

- The focused orchestrator tests passed, including both prepared-route race orderings and the existing profile-guard cases.
- The focused handler tests passed, including the validation response mapping.
- The race-enabled tests passed for both prepared-route operation orderings.
- Changed-file Go lint passed with `0 issues.`
- `python3 scripts/list-docs.py validate` passed for 415 decisions and 1,834 specifications.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
