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

- `persistTodoMessage` uses `currentTurnIDForSession` and skips persistence when
  no turn is active.
- Tests with the SQLite-backed task service for both turn states.

## Out of scope

Resume scheduling, turn adoption, stamp rules, UI, and migrations.

## Acceptance

1. The no-active-turn test fails on the old code and passes with the fix.
2. A todo report during an active turn is still persisted on that turn.
3. Existing resumed-status tests keep passing.

## Verification

```bash
(cd apps/backend && go test ./internal/orchestrator -run 'TestReplayedTodosWithoutActiveTurnDoNotCreateTurn|TestTodosAttachToExistingActiveTurn|TestResumedSessionStatus' -count=1)
(cd apps/backend && go test ./internal/orchestrator ./internal/task/... -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go`

## Dependencies

None.

## Risks

Todo lists reported outside a turn are no longer persisted. The live update still
reaches clients, and a list produced during a turn is persisted from that turn.

## Parallelism

`sequential`

## Inputs

- [Requirement 004](../../specs/tasks/requirements/workflow-explicit-completion-signal.md)
- [System design](../../specs/tasks/system-design/workflow-explicit-completion-signal.md)
- [Issue #4047](https://github.com/kdlbs/kandev/issues/4047)

## Results

Completed. The todo path uses the same non-creating turn lookup as the resumed
status message. The regression test failed on the unmodified code with one open
turn and passes with the fix.
