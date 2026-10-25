---
id: "02-backend-service-handler-reorder"
title: "Backend service and WS handler reorder"
status: done
wave: 2
depends_on: ["01-backend-repository-reorder"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-reorder.md"
---

# Task 02: Backend service and WS handler reorder

## Acceptance

1. `messagequeue.Service.ReorderEntries` validates input (non-empty, no duplicate ids) and delegates to the repository; drift errors surface as `ErrQueueChanged`.
2. New WS action `message.queue.reorder` is registered and handled: `{session_id, ordered_ids}` parsed, validation errors for missing session / empty / duplicate ids, session access authorized, `ErrQueueChanged` mapped to the `queue_changed` error code, success responds `{session_id, reordered: N}` and publishes `message.queue.status_changed`.
3. Handler tests cover happy path (publish called), validation, access denied, error mapping, and a real `NewServiceMemory` round-trip; the action is added to the registration test table.

## Verification

```bash
cd apps/backend && go test ./internal/orchestrator/handlers/...
cd apps/backend && go test ./internal/orchestrator/messagequeue/...
cd apps/backend && go vet ./internal/orchestrator/handlers/...
```

## Files likely touched

- `apps/backend/pkg/websocket/actions.go` — `ActionMessageQueueReorder = "message.queue.reorder"`.
- `apps/backend/internal/orchestrator/messagequeue/service.go` — `ReorderEntries`.
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go` — `ReorderEntries` on the `QueueService` interface, `queueErrorCodeQueueChanged = "queue_changed"` constant, `wsReorder` + registration.
- `apps/backend/internal/orchestrator/handlers/queue_handlers_reorder_test.go` — new.
- `apps/backend/internal/orchestrator/handlers/queue_handlers_test.go` — add `reorder` row to the table (mirror the `merge` row; body needs a second entry id).

## Dependencies

Task 01 (repository `ReorderEntries` + `ErrQueueChanged`).

## Parallelism

Sequential.

## Inputs

- Spec: `## API Surface`, `## Permissions`, `## Failure Modes`.
- Plan: `### Service`, `### WebSocket action`, `### Handler`, `### Tests`.
- Existing patterns: `wsMergeIntoAbove` + `publishStatus` in `queue_handlers.go`, send-now error mapping, `queue_handlers_send_now_test.go`.

## Output contract

Summary, files changed, exact test commands and outcomes, blockers, risks; update task + plan statuses in the same conversation.

## Results

- `cd apps/backend && go test -count=1 ./internal/orchestrator/handlers/...` → `ok`.
- `cd apps/backend && go build ./...` → exit 0 (whole backend compiles with the new `QueueService.ReorderEntries`).
- Files: `pkg/websocket/actions.go` (`ActionMessageQueueReorder`), `messagequeue/service.go` (`ReorderEntries`), `handlers/queue_handlers.go` (interface method, `queueErrorCodeQueueChanged`, `wsReorder` + registration, `hasDuplicateIDs`), `handlers/queue_handlers_reorder_test.go` (new), `handlers/queue_handlers_test.go` (auth-table row).
