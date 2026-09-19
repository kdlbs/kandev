---
id: "02-status-summary-queued-count"
title: "Add queued_prompt_count to the live status summary"
status: done
wave: 2
depends_on: ["01-pending-prompt-count-queries"]
plan: "plan.md"
spec: "../../specs/ui/requirements/sidebar-queued-prompt-count.md"
---

# Task 02: Add `queued_prompt_count` to the Live Status Summary

## Acceptance

- `statussummary.TaskStatusSummary` gains a new field; `Validate`
  rejects negative values; `SemanticJSON`/`semanticPayload` include the field
  so the persisted `task_status_summaries` payload round-trips it; old stored
  rows decode with the field absent (0).

  ```go
  QueuedPromptCount int `json:"queued_prompt_count,omitempty"`
  ```
- The projector (`internal/task/statussummary/projector.go`) subscribes to
  `events.MessageQueueStatusChanged` and refreshes the affected task's count
  through a new `CountQueuedPrompts func(ctx, taskID string) (int, error)` in
  `ProjectorConfig`. A queue event whose recomputed count equals the current
  value does not bump the revision or republish. The count is restored from
  the persisted summary in `restorePersistedState` and included in
  `deriveSummary`.
- The internal `message.queue.status_changed` event payload gains `task_id` at
  both publish sites:
  - `orchestrator.Service.publishQueueStatusEvent` resolves session→task via a
    new `SessionTaskID(ctx, sessionID)` helper (orchestrator repo owns
    sessions); missing/unknown session → log and publish without `task_id`.
  - `handlers.QueueHandlers.publishStatus` resolves via an injected
    `SessionTaskResolver` field (wired to `orchestratorSvc.SessionTaskID` in
    `backendapp/gateway.go`); resolution failure → log, publish without
    `task_id`.
  The WS-facing `message.queue.status_changed` payload therefore also carries
  `task_id` (informational); its session-scoped routing is unchanged.
- The projector is wired in `backendapp/gateway.go` with the count adapter from
  `orchestratorSvc.GetMessageQueue()`.
- Unit tests prove: model round-trip + negative validation; projector updates
  the summary on a queue event, suppresses republish on unchanged count,
  restores the count from persisted state, and ignores events without
  `task_id`; handlers publish `task_id` when resolvable and skip it when not.

## TDD Sequence

1. Model tests (round-trip, validation) + projector tests (event → count
   update, no-change suppression, restore, missing task_id) + publish-site
   tests. Run RED.
2. Implement the field, projector subscription/state/derive/restore, the
   orchestrator `SessionTaskID` helper, the handler resolver, and gateway
   wiring.
3. Run package tests GREEN, then refactor.

## Verification

```bash
cd apps/backend
go test ./internal/task/statussummary/... ./internal/orchestrator/... ./internal/backendapp/...
```

## Files Likely Touched

- `apps/backend/internal/task/statussummary/model.go`
- `apps/backend/internal/task/statussummary/model_test.go`
- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/projector_test.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go` (publishQueueStatusEvent)
- `apps/backend/internal/orchestrator/service.go` (SessionTaskID helper)
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go` (publishStatus + resolver field)
- `apps/backend/internal/orchestrator/handlers/queue_handlers_test.go`
- `apps/backend/internal/backendapp/gateway.go`

## Dependencies

Task 01 supplies `messagequeue.Service.CountPendingByTask`.

## Output Contract

Report RED/GREEN evidence, the new event payload shape, and changed files.
Update this task and `plan.md` status in the same implementation conversation.

## Results

- RED: model/projector/publish-site tests failed against the unimplemented field and payload.
- GREEN: `go test -tags fts5 ./internal/task/statussummary/ ./internal/orchestrator/... ./internal/backendapp/...` —
  round-trip + negative validation, queue-event count updates, no-change suppression, restore, missing-task_id ignore.
- The internal and WS `message.queue.status_changed` payload now carries `task_id` alongside
  `{session_id, entries, count, max}`; session-scoped routing unchanged.
- Follow-up: `TestProjectorQueueEventRetriesAfterRejectedWrite` — competing writer lands a higher revision with
  the stale count; the retried write wins (final stored count 3, revision 8, exactly one publish).
