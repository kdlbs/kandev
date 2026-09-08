---
id: "01-resolve-dynamic-workflow-replacement-profiles"
title: "Resolve dynamic workflow replacement profiles"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.3
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
---

# Task 01: Resolve Dynamic Workflow Replacement Profiles

## Summary

Resolve a dynamic destination before workflow profile switching attaches the
retained task environment. Add a regression test for the lifecycle profile.
The Kandev session must keep its logical profile and route attribution.

## In scope

- Add a focused orchestrator regression test using real profile routing and
  task-session persistence.
- Resolve the newly prepared workflow replacement session through
  `resolveDynamicLaunchExecution` before `LaunchPreparedSession`.
- Pass the concrete execution profile to workspace attachment without changing
  the logical session profile.
- Remove or terminalize the prepared replacement when route resolution fails.
- Verify the selected route remains sticky for the later created-session start.

## Out of scope

- Dynamic routing policy or candidate-order changes.
- Workflow validation, frontend, API, schema, migration, or documentation
  changes.
- Refactoring other direct `LaunchPreparedSession` callers.

## Acceptance

- A workflow switch does not pass the virtual profile to lifecycle during
  replacement workspace attachment.
- The replacement session stores the dynamic logical profile and selected
  concrete execution profile before it becomes primary.
- A resolution or attachment error leaves the prior session active and does not
  leave a reusable partial replacement.
- Concrete-profile switching remains unchanged.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestCreateNewSessionForStep(_(ResolvesDynamicProfileBeforeWorkspaceAttach|RemovesPreparedSessionWhenDynamicResolutionFails|TerminalPrimaryReusesCanonicalEnvironment)|KeepsCurrentSessionWhenWorkspaceAttachFails)' -count=1
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_dynamic_profile_test.go`

## Dependencies

None.

## Risks

- A second route claim during `StartCreatedSession` could select a different
  candidate or increment the generation. The test must assert sticky persisted
  attribution across preparation and start boundaries.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001`, especially the transparent profile
  execution and workflow-use sections.
- `docs/specs/agents/system-design/dynamic-agent-routing-01.md` profile
  execution and route-selection boundaries.
- `docs/specs/agents/system-design/dynamic-agent-routing-02.md` persistence and
  failure guarantees.
- `docs/decisions/2026-08-13-dynamic-agent-profile-routing.md`.
- `Service.createNewSessionForStep`, `Service.resolveDynamicLaunchExecution`,
  and the existing workflow profile-switch tests.

## Results

RED: the dynamic-profile regression failed because lifecycle received
`profile-dynamic`. Lifecycle rejected that virtual profile before it attached
the retained task environment. The resolution-failure cleanup test also found
a prepared replacement row left behind.

The workflow replacement path now resolves the prepared session before the
workspace attachment. It stores the concrete execution profile and route
generation, keeps the logical dynamic profile, and passes the concrete profile
to lifecycle. A second resolution reuses the stored route.

The path validates the destination before preparation and removes a prepared
replacement when route selection fails. If removal fails, it terminalizes the
row so later workflow switches cannot reuse it. The regression now starts the
created session through `StartCreatedSession` and reloads the persisted row.

Verification:

- `cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'TestCreateNewSessionForStep(_(ResolvesDynamicProfileBeforeWorkspaceAttach|RemovesPreparedSessionWhenDynamicResolutionFails|TerminalPrimaryReusesCanonicalEnvironment)|KeepsCurrentSessionWhenWorkspaceAttachFails)' -count=1`: seven tests passed.

PR fixup: addressed two exact-head review findings. The test now proves
persisted sticky attribution across the replacement and created-session start
boundaries. Resolution failures no longer leave a partial replacement that a
later workflow switch could promote.
