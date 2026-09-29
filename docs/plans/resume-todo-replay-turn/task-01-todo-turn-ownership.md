---
id: "01-todo-turn-ownership"
title: "Resolve the todo turn without creating one"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-004
acceptance_criteria:
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-004.1
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-004.2
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-004.3
system_design:
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
---

# Task 01: resolve the todo turn without creating one

## Summary

Stop a replayed todo list from opening a prompt-less turn that the next workflow
prompt adopts with a stale launch stamp.

## In scope

- `persistTodoMessage` uses `currentTurnIDForSession`. Without an active turn it
  persists the message through `CreateLifecycleSessionMessage` in a completed
  lifecycle-only turn.
- `MessageCreator.CreateLifecycleSessionMessage` and its adapters, backed by the
  task service's `CompletedTurn` request field.
- Tests with the SQLite-backed task service for both turn states and for the
  replay-then-step-change prompt stamp.

## Out of scope

Resume scheduling, turn adoption, stamp rules, UI, and migrations.

## Acceptance

1. On the old code, a todo report without an active turn leaves an open turn;
   with the fix it leaves none and the message is still persisted.
2. A todo report during an active turn is still persisted on that turn.
3. After a replay and a step change, the next prompt starts a new turn stamped
   with the current step.
4. Existing resumed-status tests keep passing.

## Verification

```bash
(cd apps/backend && go test ./internal/orchestrator -run 'TestTodosWithoutActiveTurnPersistInCompletedLifecycleTurn|TestTodoReplayBeforeStepChangeLetsNextPromptStartCurrentStepTurn|TestTodosAttachToExistingActiveTurn|TestResumedSessionStatus' -count=1)
(cd apps/backend && go test ./internal/orchestrator ./internal/task/... -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/backendapp/adapters.go`
- `apps/backend/internal/integration/test_server_test.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`

## Dependencies

None.

## Risks

Every out-of-turn todo report adds one completed lifecycle-only turn. The UI
keeps the latest todo message per turn, so the latest list still wins.

## Parallelism

`sequential`

## Inputs

- [Requirement 004](../../specs/tasks/requirements/workflow-explicit-completion-signal.md)
- [System design](../../specs/tasks/system-design/workflow-explicit-completion-signal.md)
- [Issue #4047](https://github.com/kdlbs/kandev/issues/4047)

## Results

Completed. The todo path uses the same non-creating turn lookup as the resumed
status message and stores out-of-turn reports in a completed lifecycle-only
turn. The replay-then-step-change test proves the next prompt starts a
current-step turn.
