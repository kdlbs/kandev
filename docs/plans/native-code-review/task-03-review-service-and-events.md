---
id: "03-review-service-and-events"
title: "ReviewService, validation, and bus/WS events"
status: done
wave: 2
depends_on: ["01-schema-and-repository"]
plan: "plan.md"
spec: "../../specs/agents/requirements/native-code-review.md"
---

# Task 03: ReviewService, validation, and bus/WS events

The single write path for runs and findings, plus the event fan-out the UI subscribes to.

## Inputs

- Spec **State machine**, **Failure modes** (malformed-entry rules), **API surface** → WebSocket events.
- Pattern: `internal/task/service/walkthrough_service.go` + `walkthrough_service_test.go` (minimal local repo interface, `publishEvent`, normalize-then-validate).
- `internal/events/types.go`, `internal/gateway/websocket/task_notifications.go`, `pkg/websocket/actions.go` for the walkthrough event wiring to copy.

## Work

1. `internal/events/types.go` — `TaskReviewRunUpdated`, `TaskReviewFindingsPublished`, `TaskReviewFindingUpdated`, `TaskReviewCleared`.
2. `pkg/websocket/actions.go` — matching client action names `task.review.run_updated`, `task.review.findings_published`, `task.review.finding_updated`, `task.review.cleared`.
3. `internal/gateway/websocket/task_notifications.go` — forward each bus event to its client action, scoped to the task's subscribers exactly as walkthrough events are.
4. `internal/task/service/review_service.go`:
   - local `reviewRepo` interface covering only the methods from task 01.
   - `CreateRun`, `MarkRunRunning`, `CompleteRun(runID, summary, counts, tokens, durationMs)`, `FailRun(runID, code, message)`, `CancelRun` — each persists then publishes `TaskReviewRunUpdated`.
   - `PublishFindings(ctx, PublishFindingsRequest{TaskID, RunID, Trigger, Summary, Findings []FindingInput})`. When `RunID` is empty it creates a `completed` run with the given trigger (the MCP path). Normalizes and validates per the spec; any invalid entry rejects the whole batch with a wrapped `ErrInvalidReviewFinding` naming the offending index and field. Deletes superseded rows, inserts, publishes `TaskReviewFindingsPublished`.
   - `UpdateFindingStatus(findingID, status)` — sets `resolved_at` when moving to `resolved`, clears it when returning to `open`; publishes `TaskReviewFindingUpdated`.
   - `ClearTaskReview(taskID)` — deletes findings and runs, publishes `TaskReviewCleared`.
   - `GetTaskReview(taskID)` — runs (newest first, capped at 20) plus all findings.
5. Wire the service in `internal/backendapp/services.go` and hand it to the MCP handlers and orchestrator in later tasks.

## Acceptance

- A batch with one malformed entry stores nothing and returns an error naming the index and field.
- Publishing a finding that duplicates an earlier run's open finding leaves exactly one row.
- Each mutating method publishes exactly one event of the expected type with the expected payload keys.

## Verification

```
cd apps/backend && go test ./internal/task/service/... -run 'Review'
cd apps/backend && go test ./internal/gateway/websocket/...
```

## Files likely touched

`internal/events/types.go`, `pkg/websocket/actions.go`, `internal/gateway/websocket/task_notifications.go`, `internal/task/service/{review_service.go,review_service_test.go}`, `internal/backendapp/services.go`.

## Output contract

Summary, files changed, tests run with results, blockers, risks, `status: done`, plan checkbox.
