---
id: "02-settle-disabled-state"
title: "Settle retained state on a disabled boot"
status: done
wave: 2
depends_on:
  - "01-gate-async-entry-points"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-011
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-011.4
  - AC-TASKS-ASYNC-QUESTIONS-011.5
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 02: Settle retained state on a disabled boot

## Summary

Boot with async questions off against the same data an enabled boot wrote.
Make async waits and unsent notifications inert while preserving the request
and outcome history.

## In scope

- Use the startup preparation boundary before recovery, prompt consumers, or
  ready reads. Keep normal schema setup in both states. Run a narrow,
  repeatable disabled-state settlement without constructing the async service.
- Cancel `requested`, `parked`, and `release_requested` waits with reason
  `feature_disabled`. There is no generated turn, wake or workflow advance.
  Rebuild affected compact pending-action and task status summaries from the
  existing blocking and permission sources. Ensure a retry after interrupted
  cleanup also repairs the stored projection. Keep regular waiting sessions
  promptable under the existing rules.
- Abandon pending prompt outbox intents, including those in retry backoff.
  Check the server actor and durable admission identity before retiring a
  correlated queue entry. Preserve ordinary FIFO. Record retirement on the
  existing outbox record before removing a matching unsent row. Retain the
  original admission timestamp and receipt. Recover a crash between queue
  admission and outbox acknowledgement without issuing a second prompt.
  An accepted submission follows normal settlement. Keep an uncertain
  submission's claim and use the existing recovery decision. If recovery
  confirms the prompt was not sent, the retirement record prevents it from
  being restored, even after re-enable. Keep cleanup retryable if a step fails.
- Keep question, forward, outcome, transcript, and delivery history. Re-enable
  with the same data and prove that a retained transcript intent settles
  once, an eligible pending request can still be answered, and a replaced
  author or changed direct parent cannot reclaim authority. Work started
  before disable keeps its own recovery contract.
- Preserve deletion independently of the optional service. Reuse task and
  workspace storage cleanup for question, wait, dependency, forward and
  delivery rows. The question tables do not have task foreign-key cascades.
  Keep archive and normal authority checks when the feature is enabled.

## Out of scope

- Restarting or interrupting an agent as part of disabling the feature.
- Rewriting messages that reached an agent or changing the queue's general
  recovery contract.
- Altering blocking clarification holds, permissions, or session state.

## Acceptance

- The first service read after an off boot shows no async wait or false
  clarification indicator. A real blocking clarification still holds the
  session, and an unrelated queued prompt can drain.
- The off restart cannot dispatch an old async prompt, even after partial
  cleanup, queue recovery, or re-enable. Accepted or uncertain submissions
  keep their existing recovery semantics. The queue retains ordinary work.
- Retained questions are available when enabled again under current
  authorization. Storage deletion still works with the service absent.

## Verification

From the repo root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/questions ./internal/orchestrator/messagequeue -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp ./internal/task/repository/sqlite ./internal/task/service ./internal/orchestrator -run 'AsyncQuestion|PendingAction|PendingQueueDispatch' -count=1)
(
  : "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
  cd apps/backend &&
    go test -trimpath -p=1 ./internal/task/repository/sqlite ./internal/orchestrator/messagequeue ./internal/orchestrator -run 'Test(AsyncQuestionsPostgres|Postgres.*(AsyncQuestion|PendingAction))' -count=1 -v
)
git diff --check
```

Use the [scoped Go lint command](plan.md#verification-results). Record names
and results from the real PostgreSQL run, including the planned
`TestPostgresAsyncQuestionsDisabledRestartClearsPendingAction` and
`TestPostgresAsyncQuestionsDisabledRetiresOnlyUnsentQueueEntries`. A skipped
case supplies no evidence.

## Lifecycle evidence

| Scenario | Invariant to prove | Test evidence |
| --- | --- | --- |
| Off restart during requested, parked, or releasing wait | Clear only async hold and projection. No turn, wake, or workflow completion. | `TestAsyncQuestionsDisabledBootClearsWaitProjection` and the PostgreSQL counterpart |
| Before admission, admission before acknowledgement, or pending queue dispatch | Retire unsent generated prompts once, keep receipts and unrelated queue order. Accepted or uncertain work remains under recovery. | `TestAsyncQuestionsDisabledRetiresOnlyUnsentQueueEntries`, `TestAsyncQuestionsDisabledRecoveryDoesNotReplay`, `TestSQLiteRetiredAsyncQuestionReservationSkipsFuturePrompts` |
| Re-enable after interrupted cleanup, invalidated authority, or deletion | No resurrection or redirect. Preserve allowed question history and answer paths. | `TestAsyncQuestionsDisabledRestartPreservesRecords`, `TestAsyncQuestionsReenableKeepsValidAuthority`, `TestAsyncQuestionsDeleteTaskCleansRowsWithoutService` |

Update Results with the exact commands and outcome for each row. Task 03
exercises the resulting restart and first-render behavior through Playwright.

## Files likely touched

- `apps/backend/internal/backendapp/{main,startup,async_questions}.go` startup
  composition and pending-action tests.
- `apps/backend/internal/orchestrator/{startup_preparation,queue_dispatch_recovery}.go`
  and `internal/orchestrator/messagequeue/` queue ownership and recovery code.
- `apps/backend/internal/task/repository/sqlite/{async_question_waits,async_question_delivery,task,workspace}.go`
  and the associated tests.
- `apps/backend/internal/task/service/service_status_summary_rebuild.go` and
  existing pending-action projection helpers if the startup adapter needs them.

## Dependencies

[Task 01](task-01-gate-async-entry-points.md).

## Risks

- A wait row can disappear from SQL while an older persisted task summary
  still advertises it. Verify the boot and shared task-list reads.
- Wiping a queue or treating an uncertain send as known unsent risks losing
  ordinary work or resending a delivered prompt. Fence by exact admission,
  keep receipts, and preserve the existing recovery decision.

## Parallelism

`sequential`

## Inputs

- [Installation release gate](../../specs/tasks/system-design/async-agent-questions.md#installation-release-gate).
- `PrepareStartup`, `GetPendingActionsBySessionIDs`, and
  `reconcilePendingQueueDispatchesOnStartup`.
- [Lifecycle evidence checklist](../../../.agents/skills/tdd/references/lifecycle-evidence.md).

## Results

The race run passed for `internal/task/questions` (10.404s) and
`internal/orchestrator/messagequeue` (6.307s). The focused non-race run
passed in `backendapp` (0.272s), `task/repository/sqlite` (6.077s),
`task/service` (11.816s), and `orchestrator` (12.637s). Both commands are
listed in [Verification](#verification). The worker startup regression
reproduced the default-off panic before the fix. It now passes with
`TestAsyncQuestionsDisabledStartupSkipsTypedNilWorker`.

The PostgreSQL command above ran against a disposable PostgreSQL 16.16
database. The task repository package passed in 13.075s. Its named cases
covered resolution concurrency, request replay, concurrent create and wait,
reparenting, forwarded visibility, and pending-action precedence:

- `TestAsyncQuestionsPostgresResolutionRace`
- `TestAsyncQuestionsPostgresCreateReplay`
- `TestAsyncQuestionsPostgresConcurrentCreateAndWait`
- `TestAsyncQuestionsPostgresAuthoritySweepStalesReparentedRequest`
- `TestAsyncQuestionsPostgresForwardedConversationVisibility`
- `TestPostgresAsyncQuestionsParkedGateProjectsPendingAction`
- `TestPostgresPendingActionsExcludeTerminalSessionsAndPreferClarification`
- `TestPostgresNextPendingActionProjectionEpochPreservesMonotonicCorruptValue`

`TestPostgresAsyncQuestionsDisabledRetiresOnlyUnsentQueueEntries` passed in
the messagequeue package (0.208s), and
`TestPostgresAsyncQuestionsDisabledRestartClearsPendingAction` passed in the
orchestrator package (1.914s). The messagequeue race run also passed the
SQLite controls `TestSQLiteRetiredAsyncQuestionReservationSkipsFuturePrompts`
and `TestAsyncQuestionRetirementIgnoresOtherScopesWithSameClientID`.

`(cd apps/backend && go run ./cmd/sqlguard ./internal)` passed.
`(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)`
passed in 44.717s. The [scoped lint command](plan.md#verification-results)
reported `0 issues` in five sequential runs covering MCP and clarification,
orchestrator, executor and messagequeue, backendapp, task packages, and
runtime flags, config and profiles. `git diff --check` passed.
