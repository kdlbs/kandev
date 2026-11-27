---
id: "01-undispatched-comment-delivery"
title: "Keep undispatched comment messages queued"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-COMMENTS-002
acceptance_criteria:
  - AC-TASKS-PLAN-COMMENTS-002.9
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
  - ../../specs/tasks/system-design/plan-comments-02.md
---

# Task 01: Keep undispatched comment messages queued

## Summary

Fix issue #4964. When a comment-bearing queued message (plan comments or web
preview feedback) fails after the delivery-attempted marker but before
agentctl I/O, clear the marker and requeue the entry instead of silently
acknowledging it. Log every remaining possibly-delivered acknowledgement.

## In scope

- A `lifecycle.ErrPromptNotDispatched` sentinel, wrapping every error that
  `SessionManager.sendPrompt` returns before `triggerPrompt`.
- Wrapping orchestrator post-boundary, pre-executor failures:
  `admitCeilingDispatch` and `runAfterDispatchAdmission` in
  `validateAndRunDispatchBoundary`, and the model-switch admission branch in
  `attemptModelSwitchForPromptWithAdmission`.
- `messagequeue.Service.ClearDeliveryAttemptForSession`, with memory and SQLite
  (PostgreSQL-compatible) repository implementations that mirror
  `MarkDeliveryAttemptedForSession` validation.
- A not-dispatched branch in `finishQueuedMessageExecution` that clears the
  marker, continues through `handleQueuedMessageExecutionError`, and sets
  `allowFollowupDrain = false`.
- A Warn log for possibly-delivered acknowledgements and an Error log when
  clearing the marker fails.

## Out of scope

- Moving the marker later in the dispatch path.
- Transcript "undelivered" markers or UI changes.
- Managed-input entries. They have their own reconciliation and stay as they
  are.

## Implementation acceptance conditions

1. Given a durable comment queue entry and a lifecycle failure injected after
   `preparePrompt` (for example `persistInitialDeliveryBeforeDispatch` or
   `admitPrompt`), the entry stays in the session queue without
   `delivery_attempted`, has no duplicate transcript, and a later drain
   dispatches it once.
2. Given a `triggerPrompt` failure or `ErrUncertainPromptDelivery` after the
   marker, the entry is acknowledged as before and a Warn log with task,
   session, queue ID, and error is emitted.
3. `ClearDeliveryAttemptForSession` rejects a stale identity, a non-comment
   entry, and an entry no longer reserved, and works on SQLite and, with
   `KANDEV_TEST_POSTGRES_DSN`, PostgreSQL.

## Verification

```bash
cd apps/backend
go test -trimpath ./internal/agent/runtime/lifecycle/ -run 'NotDispatched' -count=1
go test -trimpath ./internal/orchestrator/messagequeue/ -run 'DeliveryAttempt' -count=1
go test -trimpath -race ./internal/orchestrator/ -run 'PlanComment|NotDispatched|DeliveryAttempt' -count=1
cd ../.. && bash scripts/lint-go-changed $(git diff --name-only -- 'apps/backend/*.go')
```

Optional: rerun the messagequeue test with `KANDEV_TEST_POSTGRES_DSN` set.

## Likely files

- `apps/backend/internal/agent/runtime/lifecycle/stream_reconciliation.go` (sentinel next to `ErrUncertainPromptDelivery`), `session.go` (lines ~2044–2083)
- `apps/backend/internal/orchestrator/task_operations.go` (`validateAndRunDispatchBoundary`, `attemptModelSwitchForPromptWithAdmission`)
- `apps/backend/internal/orchestrator/event_handlers_agent.go` (`finishQueuedMessageExecution`, `executeQueuedMessageWithReservation`)
- `apps/backend/internal/orchestrator/messagequeue/service.go`, `repository_sqlite.go`, `repository_memory.go`, and the repository interface
- New tests: `event_handlers_plan_comment_undispatched_test.go`, `messagequeue/clear_delivery_attempt_test.go`, `lifecycle/session_not_dispatched_test.go`. Use new files, because existing test files are near the 800-line limit.

## Risks

- Requeue must keep `client_queue_id`, the request fingerprint, and the
  durable-transcript metadata. Assert this in test 1. If
  `requeueMessageForSession` drops them, restore comment-bearing entries with
  `restoreQueuedMessageForSession` instead.
- Check that `turnStartAlreadyProcessed` stops a second on-turn-start
  evaluation on retry.

## Results

Done.

- `lifecycle.MarkPromptNotDispatched` marks every `sendPrompt` failure before
  `triggerPrompt`, keeping the error text unchanged; the orchestrator marks
  ceiling admission and `afterDispatchAdmission` failures in both paths.
- `ClearDeliveryAttemptForSession` (memory, SQLite, PostgreSQL-compatible SQL)
  and `requeueUndispatchedDelivery` release the receipt through
  `ReleaseQueuedDeliveryForSession`. The ordinary requeue path rejects comment
  receipts because they have no dispatch-claim row, so the design was updated.
- Capacity refusal detection now uses `errors.As`.
- Send Now: `revokeUndispatchedSendNow` clears all comment-bearing sources in
  one `ClearDeliveryAttemptForSession` list call and uses the existing claim
  restore path (`queue_send_now_undispatched_test.go`, red before the fix).

Commands: the three targeted `go test` commands pass; full
`go test -tags fts5 -race ./internal/orchestrator/...` and
`go test -tags fts5 ./internal/agent/runtime/lifecycle/... ./internal/gateway/...`
pass; `bash scripts/lint-go-changed` reports 0 issues. PostgreSQL variants
were skipped (no `KANDEV_TEST_POSTGRES_DSN`).
