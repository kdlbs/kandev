---
id: "02-route-explicit-recipients"
title: "Route initial workflow recipients"
status: complete
wave: 2
depends_on:
  - "01-persist-recipient-contracts"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.2
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.3
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.4
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.3
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.5
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.7
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.9
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 02: Route initial workflow recipients

## Summary

Wire initial targeting through every workflow entry path. Deliver exact original
reuse and fresh initial-profile conversations without source-step support.

## In scope

- Record the initial snapshot with first-session creation. Backfill older tasks
  through `originalTaskSession`; reject ambiguous or unrecoverable provenance.
- Typed destination selection shared by preflight and preparation, revalidated
  under the lifecycle guard. The initial start does not create a throwaway session.
- Exact original reuse, same-profile new, terminal/deleted-original fallback,
  logical dynamic-profile launch, ACP and passthrough.
- Manual, queued, automatic, direct-engine and no-session paths. Persist and
  resume prepared/committed route state without duplicate fresh allocations.
- Existing queue transfer, pending moves, source-end policies, stamped stops,
  promotion rollback, authorization, completion signals, and question barriers.

## Out of scope

- Step binding records or earlier-step targets.
- Editor controls and Office routing changes.

## Acceptance

- Sol → Luna → initial returns to the exact parked original; `new` creates a
  distinct conversation even when profiles match, without changing original identity.
- First creation, deletion, terminal state, restart and retries preserve the
  documented target and error behavior. Unknown legacy initial identity fails visibly.
- All entry paths deliver once to the selected session; source lifecycle and
  profile-only regression criteria 001.1-001.6 remain intact.

## Verification

Write failing initial scenarios in `workflow_session_target_test.go` and
`workflow_session_target_integration_test.go` before implementation.
From `apps/backend`:

```bash
rtk go test -race ./internal/orchestrator -run 'TestWorkflowSessionTarget' -count=1
rtk go test ./internal/orchestrator -run 'TestPrepareWorkflowStepSession|TestSwitchSessionForStep|TestResolveStepAgentProfile|TestHandleTaskMoved|TestProcessOnEnter|TestWorkflowStore|TestHandleAgent.*ProfileSessionStopIntent' -count=1
rtk go test ./internal/workflow/engine/... ./internal/workflow/stepentry/... -count=1
```

Use production creation/promotion wiring for provenance and retry tests. Exercise
crashes after prepared and committed persistence, and fail source cleanup after
destination preparation. Regression mapping is in the plan's Tests section.

## Files likely touched

- `apps/backend/internal/orchestrator/workflow_session_target.go` (new)
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/workflow_profile_session_lifecycle.go`
- `apps/backend/internal/orchestrator/{workflow_store.go,workflow_session_config.go,session_ensure.go}`
- First-session creation and promotion paths in orchestrator/executor and task repository
- `apps/backend/internal/orchestrator/workflow_session_target*_test.go` (new)

## Dependencies

Task 01 supplies validated initial targets and transactional metadata APIs.

## Risks

A same-profile early return can ignore `new`. Re-resolving after preflight can
select another executor. Old entry events must not allocate another conversation
or overwrite a newer route. Keep exact execution stop stamps.

## Inputs

- Design: Initial provenance and entry routing; Explicit routing flow.
- Existing original-session helper, profile-switch, rollback, preflight,
  passthrough and lifecycle tests.

## Parallelism

`sequential`

## Results

Implemented exact initial-session reuse, same-profile fresh sessions, terminal
and missing-target fallback, preflight, and prepared/committed route handling
across start, move, and workflow-entry paths. The CREATED-session empty-prompt
regression is covered. Orchestrator race coverage passed 180 tests and workflow
engine/step-entry coverage passed 230 tests.
