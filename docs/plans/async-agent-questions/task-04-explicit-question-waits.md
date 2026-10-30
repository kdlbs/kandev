---
id: "04-explicit-question-waits"
title: "Pause only for explicit question dependencies"
status: completed
wave: 4
depends_on:
  - "03-parent-question-flow"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-006
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-006.1
  - AC-TASKS-ASYNC-QUESTIONS-006.2
  - AC-TASKS-ASYNC-QUESTIONS-006.3
  - AC-TASKS-ASYNC-QUESTIONS-006.4
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 04: Pause only for explicit question dependencies

## Summary

Add an explicit wait action for an agent that completes all independent work.
Bind the pause to the captured turn and generation so a racing answer cannot start work that the old pause then cancels.

## In scope

- `wait_for_question_answers_kandev`, dependency validation, and ready-before-pause results.
- Durable requested/parked/released gate transitions and their guarded orchestrator adapter.
- Completion-signal, ready-event, and queue-drain integration for the explicit gate only.
- Any-dependency release, mixed pending/terminal requests, withdrawal release, and newer-turn invalidation.
- Startup reconciliation before prompt admission, cancellation ownership, and bounded worker shutdown.
- Explicit last-action prompt guidance for the wait tool.

## Out of scope

- Automatic waits based on open requests, queue priority changes, or broader workflow-engine redesign.
- Changes to existing blocking clarification watchdogs.

## Acceptance

1. `TestAsyncQuestionsNoImplicitWait` proves that open requests alone do not suppress completion or establish a question gate.
2. Deterministic answer-before, answer-during, and answer-after-pause tests preserve one release and never cancel a successor generation.
3. Restart, explicit cancellation, a newer prompt, and terminal sessions reconcile the exact gate before admitting another prompt. Existing blocking controls remain green.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/questions -run '^TestAsyncQuestionsWait' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^(TestAsyncQuestions(Wait|NoImplicitWait)|TestHandleAgentReady_BlocksOnTurnCompleteWhileClarificationPending|TestHandleAgentCompleted_BlocksOnTurnCompleteWhileClarificationPending)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/mcp/handlers -run '^(TestAsyncQuestionsWait|TestHandleStepComplete_)' -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/server -run '^TestAsyncQuestionsWait' -count=1)
(cd apps/backend && go test -trimpath ./internal/sysprompt -count=1)
```

Run changed-file Go lint for actual touched paths.
Required new methods include `TestAsyncQuestionsWaitAnswerRace`,
`TestAsyncQuestionsWaitMixedDependencies`, `TestAsyncQuestionsWaitNewerTurn`, and `TestAsyncQuestionsWaitRestart`.
Use controlled barriers around pause admission and acknowledgement. A sleep does not prove the race ordering.
Run the restart case by reopening durable storage and reconstructing the service, not by reusing an in-memory gate.

## Files likely touched

- **New:** `apps/backend/internal/task/questions/wait.go`, `wait_test.go`
- **New:** `apps/backend/internal/orchestrator/async_question_wait.go`, `async_question_wait_test.go`
- **New:** `apps/backend/internal/mcp/handlers/async_question_wait_test.go`
- `apps/backend/internal/task/repository/sqlite/async_questions.go`
- `apps/backend/internal/mcp/server/async_questions.go`, `async_questions_test.go`
- `apps/backend/internal/mcp/handlers/async_questions.go`, `step_completion_entry.go`
- `apps/backend/internal/orchestrator/clarification_guard.go`, `event_handlers_clarification.go`
- `apps/backend/internal/backendapp/async_questions.go`
- `apps/backend/internal/sysprompt/sysprompt.go`, `sysprompt_test.go`

Follow the actual ready/queue-drain caller from `clarification_guard.go` before editing it.
Keep the async gate check separate from the existing current-turn clarification predicate.

## Dependencies

Task 03. Both operator and parent outcomes must already use the canonical service and queued delivery intents.

## Risks

- Publishing an answer before pause acknowledgement can lose the resumed turn to a stale cancellation.
- Waiting for all dependencies instead of any terminal dependency can strand work that is ready to resume.
- A mutex that serializes events is not proof of cancellation intent.
- Startup must reconcile the durable gate before any queue drain or completion trigger can observe it.

## Parallelism

`sequential`

## Inputs

- [Explicit wait requirement](../../specs/tasks/requirements/async-agent-questions.md#req-tasks-async-questions-006-explicit-dependency-wait).
- [Explicit wait design](../../specs/tasks/system-design/async-agent-questions.md#explicit-wait).
- Existing guarded clarification pause and turn-authority tests.
- Backend goroutine ownership and cancellation guidance.

## Results

Completed and remediated after review. The wait tool captures the active turn
before registering the gate and binds the forced pause to that turn. A successor
turn returns a stale-pause outcome without cancelling it; pause failures
propagate instead of being parked; a `requested` gate is fenced to
`release_requested` when an answer races the pause and is only released after
pause acknowledgement. Startup reconciliation closes requested gates that did
not survive restart and releases fenced gates with a wake.

Verification passed: questions wait/race/reconciliation tests, orchestrator
captured-turn and completion/drain tests, and MCP wait handler tests. These use
the real forced pause path for the successor case rather than a fake gate.
