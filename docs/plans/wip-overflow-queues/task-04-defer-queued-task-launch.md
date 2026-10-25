---
id: "04-defer-queued-task-launch"
title: "Defer queued task launch"
status: completed
wave: 4
depends_on:
  - "03-reconcile-queued-promotion"
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 04: Defer Queued Task Launch

## Acceptance

- A queued create-and-start request stores all resolved launch inputs atomically
  with the task.
- Queued work creates no session, workspace, checkout, executor/container, or
  agent process before promotion.
- Promotion launches admitted work exactly once through the existing session
  launch pipeline, including after backend restart.
- Launch failure leaves a visible, retryable intent without demoting the task.
- Manual relocation, archive, and deletion cancel queued launch intent.
- HTTP, WebSocket, MCP, and ordinary UI creation return queued success with the
  actual visible step and do not invoke immediate launch.

## TDD sequence

1. Add failing persistence/rollback tests for the deferred launch record.
2. Add failing adapter tests proving queued success performs no runtime work.
3. Add failing promotion and restart idempotency tests.
4. Implement the launch-intent repository and promotion consumer.
5. Add cancellation and launch-failure retry tests.

## Verification

```bash
cd apps/backend
go test -tags fts5 ./internal/task/handlers ./internal/mcp/handlers ./internal/orchestrator -run 'Test.*(DeferredTaskLaunch|QueuedCreate|QueuedStart|PromotionLaunch)' -count=1
```

## Files likely touched

- task launch-intent migration/model/repository files
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- task WebSocket create handlers
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- focused handler, MCP, launch, and recovery tests

## Dependencies

- Task 03 supplies committed promotion events and cancellation paths.

## Parallelism

`sequential`

## Output contract

Record the durable launch schema, atomic boundary, idempotency key, cancellation
rules, failure retry behavior, resource-side-effect evidence, and exact tests.
