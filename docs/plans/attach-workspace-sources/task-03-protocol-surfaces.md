---
id: "03-protocol-surfaces"
title: "Protocol surfaces"
status: done
wave: 3
depends_on: ["02-attachment-service"]
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 03: Protocol Surfaces

## Acceptance

- HTTP accepts the documented mixed batch, maps typed failures to the documented status codes, and
  returns the durable source/session projection.
- `add_workspace_sources_kandev` defaults to the current task and reaches the same service;
  `add_branch_to_task_kandev` remains wire-compatible.
- Task and session source-update events are routed through the gateway only after successful
  materialization, with handler/MCP/event tests covering payloads and failure truthfulness.

## Verification

```bash
cd apps/backend
rtk go test ./internal/task/handlers/... ./internal/mcp/... ./internal/gateway/websocket/... ./pkg/websocket/...
```

## Files likely touched

- `apps/backend/internal/task/handlers/task_handlers.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/*workspace_sources*_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`
- `apps/backend/internal/events/types.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go`

## Dependencies

Task 02.

## Inputs

- Spec: API surface and error codes.
- Existing create-task repository union and add-branch MCP forwarding tests.

## Output contract

Summary, files changed, tests run, blockers, risks, divergence, and task/plan status updates.
