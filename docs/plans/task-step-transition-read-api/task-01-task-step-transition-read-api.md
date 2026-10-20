---
id: "01-task-step-transition-read-api"
title: "Read a task's attributed step transitions over HTTP"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-STEP-TRANSITION-READ-001
acceptance_criteria:
  - AC-TASKS-STEP-TRANSITION-READ-001.1
  - AC-TASKS-STEP-TRANSITION-READ-001.2
  - AC-TASKS-STEP-TRANSITION-READ-001.3
  - AC-TASKS-STEP-TRANSITION-READ-001.4
  - AC-TASKS-STEP-TRANSITION-READ-001.5
system_design:
  - ../../specs/tasks/system-design/task-step-transition-read-api.md
---

# Task 01: Read a Task's Attributed Step Transitions over HTTP

## Summary

Add `GET /api/v1/tasks/:id/step-transitions`, returning the task's
`task_step_transitions` rows oldest first with trigger, actor kind, actor ID,
step IDs and names, and occurrence time.

## In scope

- `models.StepTransitionRecord`.
- `Repository.ListTaskStepTransitionRecords` in
  `apps/backend/internal/task/repository/sqlite/step_transitions.go`.
- `Service.ListTaskStepTransitionRecords` in
  `apps/backend/internal/task/service/step_transition_reads.go`.
- `httpListTaskStepTransitions` in
  `apps/backend/internal/task/handlers/step_transition_handlers.go` and its
  route.
- Public docs in `docs/public/tasks-and-workflows.md`.

## Out of scope

- Ledger schema or writer changes, and changes to the session workflow history
  or the plugin trail.

## Verification

- `go test ./internal/task/repository/sqlite/ -run 'TaskStepTransitionRecords'`
- `go test ./internal/task/handlers/ -run 'TestHTTPListTaskStepTransitions'`
