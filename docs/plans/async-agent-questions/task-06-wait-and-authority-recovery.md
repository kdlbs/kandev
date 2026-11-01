---
id: "06-wait-and-authority-recovery"
title: "Settle waits and close invalid requests"
status: done
wave: 5
depends_on:
  - "05-inbox-and-mobile"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-002
  - REQ-TASKS-ASYNC-QUESTIONS-004
  - REQ-TASKS-ASYNC-QUESTIONS-006
  - REQ-TASKS-ASYNC-QUESTIONS-007
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-002.3
  - AC-TASKS-ASYNC-QUESTIONS-002.5
  - AC-TASKS-ASYNC-QUESTIONS-004.2
  - AC-TASKS-ASYNC-QUESTIONS-004.4
  - AC-TASKS-ASYNC-QUESTIONS-006.4
  - AC-TASKS-ASYNC-QUESTIONS-006.5
  - AC-TASKS-ASYNC-QUESTIONS-006.6
  - AC-TASKS-ASYNC-QUESTIONS-007.4
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 06: Settle waits and close invalid requests

## Summary

Fix the reproduced failures that leave a session stuck or send a parent a dead question.
A `ready` result never leaves an active gate, every stale transition releases its waits and wakes a live author, and withdrawn or stale requests stop prompting the parent.

## In scope

- `releaseRacedWait` releasing from both `requested` and `release_requested`.
- A shared stale transition: close the request, release dependent gates, insert a wake for a live author, and publish a change event after commit.
- A wake message that states the dependency closed as stale.
- The authority sweep on the existing reconciliation tick, for reparenting, archived or missing parents, and workspace transfer.
- Abandoning unsent parent-request intents in the closing transaction, and removing an undispatched queued parent prompt after commit.
- Change events from `MarkSessionStale`, and from stale transitions reached through an answer attempt.

## Out of scope

- Choosing a parent session for delivery (Task 07).
- Frontend rendering of stale outcomes (Task 10).

## Acceptance

1. With an answer injected between gate creation and the re-check, `Wait` returns `ready` and `HasParkedWait` is false.
2. Reparenting a child parked on a parent request closes it within one sweep, with no answer attempt, and queues exactly one wake to the child.
3. A withdrawn request has no pending parent-request intent, and its undispatched queue row is gone.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/questions -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run '^TestAsyncQuestions' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestAsyncQuestions' -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp -run 'TestAsyncQuestion' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
bash scripts/lint-go-changed <changed go files>
```

PostgreSQL evidence for the sweep requires `KANDEV_TEST_POSTGRES_DSN`. A skipped test is not PostgreSQL validation.

Required new tests: `TestAsyncQuestionsWaitReadyLeavesNoGate`, `TestAsyncQuestionsSweepStalesReparentedRequest`,
`TestAsyncQuestionsSweepStalesArchivedParent`, `TestAsyncQuestionsStaleDependencyWakesAuthor`,
`TestAsyncQuestionsWithdrawAbandonsUnsentParentPrompt`, and `TestAsyncQuestionsWithdrawRemovesQueuedParentPrompt`.
Use a store wrapper or barrier for the race. A sleep does not prove the ordering.

## Files likely touched

- **New:** `apps/backend/internal/task/questions/wait_settlement_test.go`, `authority_sweep_test.go`, `withdraw_delivery_test.go`
- `apps/backend/internal/task/questions/wait.go`, `delivery.go`, `service.go`, `types.go`
- `apps/backend/internal/task/repository/sqlite/async_question_resolution.go`, `async_question_waits.go`, `async_question_delivery.go`, `async_questions_postgres_test.go`
- `apps/backend/internal/backendapp/async_questions.go`, `async_questions_test.go`

## Dependencies

Task 05.

## Risks

- The sweep runs every 15 seconds, so its query must stay bounded and indexed.
- Queue rows have an entry ID, while the outbox stores an admission ID. Confirm that `LookupQueueAdmissionWithClientQueueID` resolves the entry before calling `RemoveEntryForSession`.
- A dispatch can race row removal. Treat a row already claimed for dispatch as history, not an error.

## Parallelism

`sequential`

## Inputs

- [Gate settlement](../../specs/tasks/system-design/async-agent-questions.md#gate-settlement-remediation).
- [Authority sweep](../../specs/tasks/system-design/async-agent-questions.md#authority-sweep-remediation).
- Review reproductions at `226eca1`: wait race, reparent stranding, and withdrawn parent prompt.

## Results

Implemented wait-race settlement, bounded task/session/parent authority recovery,
stale dependency wake delivery, and withdrawal cleanup for unsent parent prompts.
The stale PostgreSQL authority-sweep regression is present but could not run locally.

Passed:

- `go test -trimpath -race ./internal/task/questions -count=1`
- `go test -trimpath ./internal/task/repository/sqlite -run '^TestAsyncQuestions' -count=1`
- `go test -trimpath ./internal/task/repository/sqlite -run '^TestAsyncQuestionsLifecycleCleanup$' -count=1`
- `go test -trimpath ./internal/orchestrator -run '^TestAsyncQuestions' -count=1`
- `go test -trimpath ./internal/backendapp -run 'TestAsyncQuestion' -count=1`
- `go run ./cmd/sqlguard ./internal`
- `git diff --check`

`TestAsyncQuestionsPostgresAuthoritySweepStalesReparentedRequest` compiled and
was skipped because `KANDEV_TEST_POSTGRES_DSN` is unset. Changed-file lint
passed with `0 issues` after the repository lint lock cleared.
