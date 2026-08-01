---
id: message-queue-merge-02
title: Backend service and WS handler
status: completed
wave: 2
depends_on: ["message-queue-merge-01"]
plan: plan.md
spec: ../../specs/ui/message-queue-merge.md
---

# Task 02: Backend service and WS handler

## Acceptance

1. `ActionMessageQueueMerge = "message.queue.merge"` exists in
   `pkg/websocket/actions.go` and `Service.MergeIntoAbove` delegates to the
   repository (with the same logger pattern as `RemoveEntry`).
2. `QueueHandlers` registers `message.queue.merge`, validates `session_id` /
   `entry_id`, rejects reserved `user_id`, defaults `user_id` to `QueuedByUser`,
   and maps `ErrEntryNotFound` → `entry_not_found` / `ErrNoMergeTarget` →
   validation, publishing `message.queue.status_changed` on success.
3. Handler tests cover the happy path, missing fields, head/mismatch → validation,
   reserved `user_id` rejection, and drained source → `entry_not_found`.

## Verification

- `cd apps/backend && go test -race ./internal/orchestrator/handlers/... ./internal/orchestrator/messagequeue/...`

## Files

- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_merge_test.go`
- `apps/backend/internal/orchestrator/messagequeue/service_test.go`

## Inputs

- Spec sections: `API surface`, `Failure modes`, `Permissions`.
- Plan sections: `Backend → Contract`, `Backend → WS handler`, `Tests` (service
  + handler rows).
- Patterns to mirror: `wsUpdateMessage` validation/error mapping
  (`queue_handlers.go:243`), `wsRemoveEntry` shape (`queue_handlers.go:332`),
  `publishStatus`.

## Risks

- Any other `QueueService` implementor in the codebase must gain the new
  `MergeIntoAbove` method — grep for `QueueService` implementations (e.g. mocks
  in tests) and update them.
