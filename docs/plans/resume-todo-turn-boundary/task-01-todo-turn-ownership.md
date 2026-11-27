---
id: "01-todo-turn-ownership"
title: "Preserve prompt ownership during todo recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-001
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
acceptance_criteria:
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.10
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.11
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-001.12
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.1
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003.3
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
---

# Task 01: Preserve prompt ownership during todo recovery

## Summary

Prevent todo snapshots from opening promptless conversational turns while
keeping changed and cleared snapshots durable. Prove that a returning workflow
prompt receives the current step stamp.

## In scope

- Change `persistTodoMessage` to resolve an existing turn without creating an
  open prompt turn.
- Preserve todo snapshots with no prompt owner through a completed
  lifecycle-only turn; preserve reserved and active turn identity otherwise.
- Retain todo broadcasts, existing status handling, and historical messages.
- Add regression tests through `/tdd`, including durable reload coverage for
  changed and empty out-of-turn snapshots.

## Out of scope

Automatic-resume policy, task routing, old-turn repair, stamp mutation, schema
changes, UI changes, provider protocol changes, and generalized event filtering.

## Acceptance

1. Todo/status recovery without a prompt creates no open turn, user prompt,
   or turn-start action. Todo broadcasts still occur. Confirmed out-of-turn
   snapshots persist in completed lifecycle-only turns for reload. Lookup errors
   create no turn and skip persistence.
2. Active and reserved prompt turns receive todo messages with their explicit
   IDs. Empty lists persist during real turns. Historical messages remain intact.
3. Work -> Review -> recovery -> Work creates a new Work-stamped turn and
   delivers its prompt once. Genuine mid-turn step changes still reject completion.

## Regression cases

Add `resume_todo_turn_boundary_test.go` rather than extending the large existing
streaming test file. Use the real task service for step-stamp assertions.

- `TestResumeTodosPersistOutOfTurnWithoutOpenTurn`: seed a completed Work turn
  and idle worker session while the task is in Review. Deliver a changed
  nonempty plan followed by resumed status. Assert zero open turns, a durable
  completed lifecycle-only message, a todo broadcast, and preservation of the
  prior todo message.
- `TestResumeTodosPersistOutOfTurnClear`: persist and read back an empty todo
  snapshot in a completed lifecycle-only turn without opening a prompt turn.
- `TestWorkflowAutoStartAfterResumeTodos`: use the same event sequence, return
  the task to Work, and dispatch the workflow prompt. Assert a distinct turn,
  Work stamp, one user message, and one fake-runtime prompt delivery.
  Include warm-cache and reconstructed-service cases.
- `TestResumeTodosPreservePromptTurn`: table-test active, reserved, empty-list,
  completed-only, and failed-lookup cases. Include a live reviewer sibling and
  an idle worker so task activity cannot substitute for session turn ownership.
  A failed lookup must not call `StartTurn` or message creation with an empty ID.
- Coordinate a completion during persistence. Keep the resolved explicit ID,
  do not adopt the successor, and do not create another turn.

The initial regression failed on the original implementation because
`getActiveTurnID` created a turn. Do not use session state alone as evidence of
a prompt. Use reserved identity first and the existing authoritative
non-creating lookup for durable active turns. After a successful lookup proves
absence, use the completed lifecycle-only write path. Log lookup errors without
prompt content.

Retain `TestResumedSessionStatusDoesNotCreateTurnForWaitingSession` and
`TestResumedSessionStatusUsesExistingActiveTurn` as controls.
Retain MCP stale-turn rejection and fresh-turn recovery tests unchanged.
Do not replace the adapter's restored-plan event with silence.

## Verification

Run from the repository root. The first command includes the new regressions
and the adjacent workflow tests after the focused red/green cycle.

```bash
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -count=1)
(cd apps/backend && go test -tags fts5 ./internal/task/service -run 'TestStartTurn.*Stamp|TestCreateCompletedTurn' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/mcp/handlers -run 'TestHandleStepComplete_' -count=1)
(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp -run 'TestLoadSuppression_|TestLoadSession' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the repository documentation coverage preflight against changed work orders
and their referenced documents. Record its result with the commands above.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming_test.go` status adapter
- `apps/backend/internal/orchestrator/task_operations_test.go` mock adapter
- `apps/backend/internal/orchestrator/service.go` message creator contract
- `apps/backend/internal/backendapp/adapters.go` production adapter
- `apps/backend/internal/backendapp/adapters_test.go` lifecycle adapter test
- `apps/backend/internal/integration/test_server_test.go` integration adapter
- `apps/backend/internal/orchestrator/resume_todo_turn_boundary_test.go` (new)
- This work order and `plan.md` for status and verification results.

## Dependencies

None.

## Risks

An empty turn ID activates the task-service creating fallback. Cached IDs can
outlive completion. Reserved turns can receive output before dispatch returns.
Do not infer prompt ownership from the task's current step or a sibling's state.

## Parallelism

`sequential`

## Inputs

- [Recovery requirements](../../specs/tasks/requirements/queued-session-ownership.md), criteria 001.10-.12.
- [Recovery design](../../specs/tasks/system-design/queued-session-ownership.md#recovery-metadata-and-prompt-turns).
- [Completion requirements](../../specs/tasks/requirements/workflow-explicit-completion-signal.md), criteria 003.1 and 003.3.
- `event_handlers_streaming_test.go` status tests and service-backed message adapter.
- `turn_lifecycle_test.go` reserved-turn fixtures.
- `step_handoff_carry_dispatch_test.go` workflow dispatch harness.
- ACP `load_suppression_test.go` and `adapter_session_resume_test.go`.

## Results

Completed. The new orchestrator regressions verify changed and empty out-of-turn
snapshots persist in completed lifecycle-only turns; active and reserved turns
retain explicit ownership; lookup errors skip persistence; and a completion
during persistence cannot redirect a todo to its successor. The warm and
reconstructed workflow dispatch cases each create one Work-stamped turn, one
user message, and one runtime prompt delivery. A production adapter test
verifies lifecycle-only persistence leaves no active turn.

Verification passed:

- `go test -tags fts5 ./internal/orchestrator -count=1`
- `go test -tags fts5 ./internal/task/service -run 'TestStartTurn.*Stamp|TestCreateCompletedTurn' -count=1`
- `go test -tags fts5 ./internal/mcp/handlers -run 'TestHandleStepComplete_' -count=1`
- `go test ./internal/agentctl/server/adapter/transport/acp -run 'TestLoadSuppression_|TestLoadSession' -count=1`
- `go test ./internal/backendapp -run '^TestMessageCreatorAdapter_CreateLifecycleSessionMessage$' -count=1`
- `go test ./internal/integration -run '^$'` (test adapter compilation)
- `go build ./internal/orchestrator ./internal/backendapp`
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- Documentation coverage preflight: `covered` with no errors.
- `git diff --check`
