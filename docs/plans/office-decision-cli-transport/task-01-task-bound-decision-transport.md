---
id: "01-task-bound-decision-transport"
title: "Establish the task-bound decision transport"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUORUM-RECORDING-001
acceptance_criteria:
  - AC-TASKS-QUORUM-RECORDING-001.2
  - AC-TASKS-QUORUM-RECORDING-001.3
  - AC-TASKS-QUORUM-RECORDING-001.4
  - AC-TASKS-QUORUM-RECORDING-001.5
  - AC-TASKS-QUORUM-RECORDING-001.6
  - AC-TASKS-QUORUM-RECORDING-001.7
  - AC-TASKS-QUORUM-RECORDING-001.8
  - AC-TASKS-QUORUM-RECORDING-001.10
  - AC-TASKS-QUORUM-RECORDING-001.11
system_design:
  - ../../specs/tasks/system-design/workflow-quorum-decision-recording.md
---

# Task 01: Establish the Task-Bound Decision Transport

## Summary

Add the singular `task decision` CLI command and its signed Office runtime API.
Keep the handler thin and route the trusted run identity into the existing
dashboard and workflow-engine decision path.

## In scope

- Add CLI flag validation and JSON request handling.
- Add the closed runtime endpoint with no caller-selectable identity fields.
- Add an acyclic Office composition adapter for `DashboardService`.
- Cover trusted identity forwarding, cross-task input rejection,
  non-participant denial, structured results, and no-write validation failures.

## Out of scope

- Prompt, skill, or MCP registry changes.
- Any change to dashboard or engine quorum semantics.

## Acceptance

- The CLI accepts only `approved|rejected` plus a non-empty reason and posts no
  task, step, role, participant, session, or agent identifier.
- The API derives all identity from the signed run and rejects forged identity
  fields and callers without a live decision seat.
- Reviewer and approver decisions still persist under the canonical live seat,
  return all seven fields, and can advance quorum through the existing service.

## Verification

```bash
cd apps/backend && rtk go test ./cmd/agentctl ./internal/office/runtime ./internal/office/dashboard -run 'Test(TaskDecision|RuntimeHandler_RecordAgentDecision|RecordAgentDecision)' -count=1
```

## Files likely touched

- `apps/backend/cmd/agentctl/kandev_task.go`
- `apps/backend/cmd/agentctl/kandev_test.go`
- `apps/backend/internal/office/runtime/handler.go`
- `apps/backend/internal/office/runtime/handler_test.go`
- `apps/backend/internal/office/runtime/errors.go`
- `apps/backend/internal/office/routes.go`
- `apps/backend/internal/office/decision_adapter.go`
- `apps/backend/internal/office/dashboard/agent_decisions_test.go`

## Dependencies

None.

## Risks

- The runtime/dashboard package cycle requires DTO translation at the Office
  composition root.
- Unknown JSON fields must fail closed for cross-task regression coverage.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-QUORUM-RECORDING-001` and the agent command contract.
- Existing runtime handler, CLI client, and dashboard decision tests.

## Results

- Added the signed `kandev task decision` CLI command with local verdict/reason validation.
- Added the closed `POST /api/v1/office/runtime/task/decision` endpoint.
- Added the Office composition adapter to `DashboardService.RecordAgentDecision`.
- Verified signed task/session/agent identity forwarding, forged identity rejection, structured results, validation and permission errors, and dashboard decision regressions.
- Verification passed: `cd apps/backend && rtk go test ./cmd/agentctl ./internal/office/runtime ./internal/office/dashboard -run 'Test(TaskDecision|RuntimeHandler_RecordAgentDecision|RecordAgentDecision)' -count=1` (18 tests).
