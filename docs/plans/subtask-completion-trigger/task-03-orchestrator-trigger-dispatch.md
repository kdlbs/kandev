---
id: "03-orchestrator-trigger-dispatch"
title: "Orchestrator trigger dispatch"
status: done
wave: 2
depends_on: ["01-backend-child-completion-repository", "02-workflow-event-contracts"]
plan: "plan.md"
spec: "../../specs/tasks/subtask-completion-trigger.md"
---

# Task 03: Orchestrator trigger dispatch

## Acceptance

- A child task state transition from non-terminal to terminal evaluates the
  parent only when every active direct child is terminal.
- The parent receives exactly one `engine.TriggerOnChildrenCompleted` for the
  same completed child set.
- Transition actions from `on_children_completed` apply the normal workflow
  lifecycle: `on_exit`, persisted parent step change, data patch persistence,
  and `on_enter`.

## Verification

```bash
go test ./internal/orchestrator/...
```

## Files Likely Touched

- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_children_completed.go`
- `apps/backend/internal/orchestrator/event_handlers_children_completed_test.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/workflow_store.go`

## Dependencies

- `01-backend-child-completion-repository`
- `02-workflow-event-contracts`

## Inputs

- Spec: `docs/specs/tasks/subtask-completion-trigger.md`, sections `State
  machine`, `Failure modes`, `Persistence guarantees`, `Scenarios`.
- Plan: Backend `Workflow Trigger Dispatch`.
- Existing patterns: `processOnTurnCompleteViaEngine`,
  `applyEngineTransition`, `office/engine_dispatcher.Dispatcher`, and watcher
  task event subscription wiring.

## Output Contract

When finished, update this task frontmatter to `status: done`, update the
checkbox in `plan.md`, and report:

- Summary of trigger dispatch and idempotency behavior.
- Files changed.
- Tests run and their result.
- Any blockers or follow-up risks.
