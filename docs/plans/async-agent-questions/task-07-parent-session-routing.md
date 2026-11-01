---
id: "07-parent-session-routing"
title: "Route parent questions by task"
status: done
wave: 6
depends_on:
  - "06-wait-and-authority-recovery"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-003
  - REQ-TASKS-ASYNC-QUESTIONS-005
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-003.3
  - AC-TASKS-ASYNC-QUESTIONS-003.4
  - AC-TASKS-ASYNC-QUESTIONS-005.3
  - AC-TASKS-ASYNC-QUESTIONS-005.4
  - AC-TASKS-ASYNC-QUESTIONS-005.5
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 07: Route parent questions by task

## Summary

Make the parent task the authority for child questions, so losing or replacing a parent session cannot strand a child.
Parent notifications go to the parent task's current session, while author deliveries stay bound to their exact session.

## In scope

- Removing the session-equality check from `ResolveParent`, and checking the task relationship for answers and forwards.
- Choosing the active parent session at admission for `parent_question` and `parent_outcome` deliveries.
- Keeping an intent pending when the parent has no active session.
- Replacing `ErrDestinationTerminal` abandonment for parent-kind deliveries whose task is still valid.
- Forward idempotency across parent sessions.

## Out of scope

- Redirecting owner outcomes or wakes. They keep their exact session and incarnation.
- Multi-hop forwarding.

## Acceptance

1. A replacement session of the same parent task can answer a child question, and an unrelated task still cannot.
2. A parent delivery recorded against a terminal session is admitted to the active session under the same admission identity, with no duplicate row on retry.
3. With no active parent session, the intent stays pending and is admitted once a session exists.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/questions -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp -run 'TestAsyncQuestion' -count=1)
(cd apps/backend && go test -trimpath ./internal/mcp/handlers -run 'TestAsyncQuestions|TestHandleAskParentQuestion' -count=1)
bash scripts/lint-go-changed <changed go files>
```

Required new tests: `TestAsyncQuestionsReplacementParentSessionAnswers`, `TestAsyncQuestionParentDeliveryTargetsActiveSession`, and `TestAsyncQuestionParentDeliveryWaitsForSession`.

## Files likely touched

- **New:** `apps/backend/internal/task/questions/parent_routing_test.go`
- `apps/backend/internal/task/questions/service.go`, `delivery.go`
- `apps/backend/internal/backendapp/async_questions.go`, `async_questions_test.go`
- `apps/backend/internal/task/repository/sqlite/async_question_resolution.go`
- `apps/backend/internal/mcp/handlers/handlers.go`

## Dependencies

Task 06.

## Risks

- The admission ID includes the recorded destination. Keep it stable when the actual session differs, so a retry cannot create a second queue row.
- `validateParentForwardAuthorityTx` marks a request stale when the parent incarnation changes. That check must follow the same task-level rule.
- A parent with several active sessions needs a deterministic target. Use what `GetActiveTaskSessionByTaskID` selects, and test it.

## Parallelism

`sequential`

## Inputs

- [Parent session routing](../../specs/tasks/system-design/async-agent-questions.md#parent-session-routing-remediation).
- Review reproduction at `226eca1`: a replacement parent session is rejected as unauthorized.

## Results

Implemented task-level parent authorization, replacement-session answers,
active-session routing for parent notifications and outcomes, and retryable
delivery while the parent task has no active session. Parent forwarding replays
remain idempotent across parent sessions.

Passed:

- `go test -trimpath -race ./internal/task/questions -count=1`
- `go test -trimpath ./internal/backendapp -run '^TestAsyncQuestionParentDelivery(TargetsActiveSession|WaitsForSession)$' -count=1`
- `go test -trimpath ./internal/mcp/handlers -run 'TestAsyncQuestions|TestHandleAskParentQuestion' -count=1`
- `go test -trimpath ./internal/task/repository/sqlite -run '^TestAsyncQuestions' -count=1`
- Changed-file Go lint: `0 issues`
- `git diff --check`
