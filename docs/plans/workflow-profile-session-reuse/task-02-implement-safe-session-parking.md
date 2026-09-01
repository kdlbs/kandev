---
id: "02-implement-safe-session-parking"
title: "Route source and destination behavior"
status: pending
wave: 2
depends_on:
  - "01-add-portable-workflow-policy"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.2
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.3
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.4
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 02: Route Source and Destination Behavior

## Summary

Use the destination step for session selection and the source step for session
retirement.

## In scope

- Split profile-switch routing into start and end inputs.
- Thread the source step or normalized end setting through every entry path.
- Keep credential preflight, promotion, queue rollback, parking, stop intent,
  guard release, and callback suppression.
- Cover legacy, manual, queued, and direct engine transitions.

## Acceptance

- Tests prove all four start-and-end combinations.
- A destination setting cannot change source retirement.
- The workflow engine core gains no action, event, or state.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'Test(SwitchSessionForStep|PrepareWorkflowStepSession|HandleAgentCompleted|HandleAgentStopped).*ProfileSession' -count=1 -v
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/workflow_callbacks.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_session_policy_test.go`
- `apps/backend/internal/orchestrator/workflow_profile_session_lifecycle.go`

## Dependencies

Task 01.

## Risks

A direct engine entry can lose the source step after the task moves.

## Parallelism

`sequential`

## Results

Pending.
