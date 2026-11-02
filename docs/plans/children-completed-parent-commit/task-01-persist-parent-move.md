---
id: "01-persist-parent-move"
title: "Persist the parent move after an in-turn child completion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SUBTASK-COMPLETION-TRIGGER-002
acceptance_criteria:
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-002.1
  - AC-TASKS-SUBTASK-COMPLETION-TRIGGER-002.2
system_design:
  - ../../specs/tasks/system-design/subtask-completion-parent-commit.md
---

# Task 01: Persist the Parent Move After an In-Turn Child Completion

## Summary

Drop the child's workflow delivery effect before the parent's
`on_children_completed` evaluation, and stop the transition lifecycle when the
commit wrote nothing.

## In scope

- `withoutWorkflowEffect` and its call in `processOnChildrenCompleted`.
- `workflowStore.applyTransitionReportingApplied` and its use in
  `applyEngineTransitionWithMode`.

## Out of scope

- The engine `Store.ApplyTransition` signature and deferred-move commits.

## Acceptance

1. A last child moved to a completion-enabled step by its own turn leaves the
   parent on the `on_children_completed` target step.
2. A commit refused for a consumed effect returns not applied, with no engine
   ledger row and no `on_enter`.

## Verification

```bash
(cd apps/backend && go test -count=1 ./internal/orchestrator/ -run 'ChildrenCompleted|TerminalStep|NotAppliedCommit')
```

## Files likely touched

- `apps/backend/internal/orchestrator/{agent_delivery_effect,event_handlers_children_completed,event_handlers_workflow,workflow_store}.go`
- `apps/backend/internal/orchestrator/event_handlers_children_completed_turn_test.go`

## Dependencies

None.

## Parallelism

`sequential`

## Results

Author-reported: both new tests fail on `main` at `b544f15d8` and pass with
the change; the existing children-completed and terminal-step tests pass.
