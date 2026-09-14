---
created: 2026-08-31
status: done
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
  - ../../specs/agents/system-design/dynamic-agent-routing-02.md
legacy_specs: []
---

# Implementation Plan: Workflow Dynamic Profile Auto-Start

## Overview

Repair issue #3202 at the workflow replacement boundary. Resolve the logical
dynamic profile before the executor attaches the retained task environment.

One TDD work order adds a regression test and uses the existing shared
resolver. The session keeps its logical identity. Lifecycle receives the
selected concrete execution profile.

## Scope

### In scope

- Cover workflow entry that switches an existing task session to a dynamic
  profile before `auto_start_agent` runs.
- Persist the selected `execution_profile_id` and route generation before the
  replacement workspace is attached.
- Preserve the existing concrete-profile workflow-switch behavior and the
  original session until replacement preparation succeeds.

### Out of scope

- Changes to dynamic candidate selection, fallback policy, or route recovery.
- Changes to workflow-step validation or the profile picker.
- Changes to manual session creation, initial task start, Office routing, or
  utility-agent routing.
- Browser UI changes and new public documentation.

## Technical approach

In `Service.createNewSessionForStep`, validate the destination before creating
the replacement session, then load the new session before workspace attachment.
This function is in `apps/backend/internal/orchestrator/event_handlers_workflow.go`.

Resolve the logical profile with `resolveDynamicLaunchExecution`. Pass the
returned concrete profile ID to `Executor.LaunchPreparedSession`.

Keep `TaskSession.AgentProfileID` unchanged. The resolver persists
`ExecutionProfileID`, route generation, route reason, and the concrete profile
snapshot before lifecycle receives the launch request.

If route selection fails after session preparation, delete the replacement row.
If deletion fails, mark the replacement terminal so it cannot be reused.

The later `StartCreatedSession` call must reuse the persisted route through
`ResolveExisting`. It must not claim a second generation or select another
candidate. Concrete profiles keep the same effective profile ID.

## Tests

- `AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1` and
  `AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.3`: add
  `TestCreateNewSessionForStep_ResolvesDynamicProfileBeforeWorkspaceAttach` in
  `apps/backend/internal/orchestrator/event_handlers_workflow_dynamic_profile_test.go`.
  The test uses real SQLite stores and the real dynamic resolver. It rejects a
  lifecycle launch that receives the virtual profile, starts the created
  session through `StartCreatedSession`, and reloads persisted attribution. A
  second test verifies that route-resolution failure removes the prepared row
  and keeps the current session reusable.
- Existing concrete workflow-switch tests in
  `apps/backend/internal/orchestrator/event_handlers_workflow_profile_test.go`
  remain the control for unchanged concrete behavior.

## E2E tests

No browser E2E test is added. The error is below the UI. The focused Go test
drives the service, resolver, executor, and real SQLite persistence. The UI and
API contracts do not change.

## Work orders

- [x] [Task 01: Resolve dynamic workflow replacement profiles](task-01-resolve-dynamic-workflow-replacement-profiles.md) (`done`)

## Verification results

The focused orchestrator test command passed all seven tests. It covers the
dynamic-profile regression, resolution-failure cleanup, three terminal-session
states, and the existing workspace-attachment failure behavior.

## Risks

- Resolving during workspace attachment must not advance the route again when
  `auto_start_agent` starts the created session.
- A resolution or persistence failure must leave the current session active and
  must not promote the replacement session.
