---
id: "02-implement-safe-session-parking"
title: "Route sessions from the destination step"
status: done
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
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.9
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.10
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 02: Route Sessions from the Destination Step

## Summary

Keep the execution-safe profile-switch lifecycle, but resolve its policy from
the destination step instead of workflow metadata. Prove mixed policies without
changing the workflow engine's transition or action contracts.

## In scope

- Normalize `destinationStep.ProfileSessionPolicy` in step-entry routing.
- Remove policy lookup from workflow metadata and its request-scoped cache.
- Retain policy-aware destination selection and source-session outcome.
- Retain execution-stamped workflow-switch stop intent and durable consumption.
- Retain completion and stopped event suppression and runtime activity cleanup.
- Cover mixed-step behavior, same-profile continuity, reuse/new identity,
  terminal-candidate races, restart, teardown failure, and queue rollback.

## Out of scope

- Workflow transition graph, action, or event changes.
- Frontend behavior.
- Office and automation session lifecycles.

## Acceptance

- Two destination steps in one workflow can produce different session behavior.
- `A -> B -> A` yields two sessions when the returning A step uses
  `park_reuse` and three when it uses `park_new`.
- Parked sessions are nonprimary, `WAITING_FOR_INPUT`, resumable, and retain
  their provider conversation identity.
- A delayed old-execution completion cannot evaluate the destination step, while
  a later execution's genuine completion still can.
- Policy resolution does not fetch workflow metadata or alter workflow engine
  transition evaluation.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'Test(SwitchSessionForStep|HandleAgentCompleted|HandleAgentStopped).*ProfileSessionPolicy' -count=1 -v
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_session_policy_test.go`
- `apps/backend/internal/orchestrator/event_handlers_test.go`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/session.go`

## Dependencies

Task 01.

## Risks

- Stop callbacks can arrive synchronously, asynchronously, duplicated, or after
  a replacement execution starts.
- Full-row metadata writes can erase unrelated session state. Use atomic key
  updates and stamped consumption.
- A test fixture that stores policy on workflow metadata can mask an incomplete
  ownership move.

## Parallelism

`sequential`

## Inputs

- System-design Control flow and Failure and recovery sections.
- Existing execution-safe parking implementation.
- Existing `completeAndStopSession`, conditional terminal promotion, and
  stamped session metadata patterns.

## Results

Resolved profile-session behavior from the destination step and retained the
execution-stamped parking and durable consumed-intent lifecycle. Added coverage
for mixed policies, terminal and delayed-event races, restart durability,
runtime-stop failure, promotion failure, and queue rollback. The focused
orchestrator suite passed, including the synchronous-stop callback regression
that verifies runtime teardown occurs after the session guard is released.
