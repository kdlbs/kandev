---
id: "02-agent-delivery-and-queue"
title: "Agent attachment delivery"
status: completed
wave: 2
depends_on: ["01-attachment-storage-api"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/prompt-attachments.md"
---

# Task 02: Agent attachment delivery

## Acceptance

1. Task creation, direct/new-session prompts, queued messages, queue edits,
   queue drain, and idempotent replay carry claimed attachment descriptors and
   never persist or send staged file bytes through the application WebSocket.
2. The lifecycle manager streams each claimed file to the authorized active
   agentctl session, materializes it beneath the session attachment directory,
   and dispatches the prompt only after every attachment succeeds.
3. Traversal, owner/task/session mismatches, oversize inline compatibility
   payloads, and failed/partial materialization fail safely; the shared
   WebSocket read limit remains 32 MiB.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/orchestrator/messagequeue ./internal/orchestrator/handlers ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/adapter/transport/shared ./internal/gateway/websocket
```

## Files likely touched

- `apps/backend/pkg/api/v1/task.go`
- `apps/backend/internal/orchestrator/message_meta.go`
- `apps/backend/internal/orchestrator/messagequeue/types.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/agentctl/client_attachments.go` (new)
- `apps/backend/internal/agentctl/server/api/attachment.go` (new)
- `apps/backend/internal/agentctl/server/adapter/transport/shared/attachments.go`
- `apps/backend/internal/gateway/websocket/client.go`
- Focused `*_test.go` files beside the changed packages

## Dependencies

- Task 01 supplies the registry, authorized resolver, storage reader, and claim
  transaction boundary.

## Parallelism

Sequential. It changes shared message/queue contracts and the backend-agentctl
delivery seam used by the frontend task.

## Inputs

- Spec: What, API surface, State machine, Failure modes, Scenarios
- Plan: Backend / Agentctl materialization and prompt delivery
- Task 01 results and final attachment service interfaces

## Output contract

Report descriptor flow, queue/idempotency behavior, materialized paths, files
changed, exact tests and outcomes, trust/authorization boundaries, blockers,
risks, and synchronized task/plan status.

## Results

Implemented descriptor propagation through task/message/queue contracts, idempotent queue/message claims, bounded inline compatibility validation, streaming backend-to-agentctl materialization, session-scoped attachment storage, and path reuse in the agent adapter. Full backend verification passed with `go test -tags fts5 ./...` (9,495 tests in 189 packages).
