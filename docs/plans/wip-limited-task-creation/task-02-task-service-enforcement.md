---
id: "02-task-service-enforcement"
title: "Task-service WIP enforcement"
status: done
wave: 2
depends_on: ["01-atomic-repository-admission"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 02: Task-Service WIP Enforcement

## Acceptance

- `Service.CreateTask` uses atomic capacity admission for a positive WIP limit
  after resolving the final workflow step.
- Explicit and automatically resolved start-step creation reject a full step
  with the typed WIP error and persist no task, repository, blocker, event, or
  session side effect.
- Unlimited workflow steps and workflow-less ephemeral tasks preserve current
  behavior.
- A positive WIP limit fails closed if capacity-aware persistence is
  unavailable.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/task/service -run 'TestCreateTask.*WIP|TestCreateTask.*StartStep|TestCreateTask.*Unlimited' -count=1
```

## Files likely touched

- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_tasks_wip_test.go`
- `apps/backend/internal/task/service/service.go` only if a narrow shared
  interface or helper belongs there

## Dependencies

Task 01.

## Parallelism

`sequential`

## Inputs

- Task 01's typed error and capacity-aware repository method.
- `resolveWorkflowStep`, `validateTaskWorkflow`, and `buildTask` in
  `service_tasks.go`.
- Existing task-event test helpers in `service_events_test.go`.

## Output contract

Mark this task `in_progress` before the RED test and `done` after GREEN and
refactor. Update `plan.md` and report explicit/resolved-step coverage,
side-effect assertions, files changed, exact test result, blockers, and risks.

## Evidence

Explicit-step, resolved-start-step, unlimited, and no-event rejection tests
pass in `internal/task/service`.
