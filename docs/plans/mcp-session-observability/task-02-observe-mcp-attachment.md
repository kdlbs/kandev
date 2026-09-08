---
id: "02-observe-mcp-attachment"
title: "Observe per-attempt MCP attachment"
status: in_progress
wave: 2
depends_on: ["01-attachment-evidence-contract"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 02: Observe per-attempt MCP attachment

## Acceptance

- ACP new/load/reset creates an attempt before delivery and emits configured,
  filtered, delivered, accepted, and explicit error evidence through the
  existing agent update stream.
- Capability filtering returns stable per-server decisions and preserves the
  current HTTP-first/SSE-fallback behavior.
- Kandev's in-session MCP endpoint emits register, initialize, `tools/list`,
  tool-call, error, and unregister evidence with opaque connection IDs.
- Hook events use instance-owned task/session/execution/attempt identity and
  contain no tool arguments/results or agent-supplied ownership IDs.
- Passthrough starts emit honest materialization evidence. A missing strategy
  is Unavailable instead of a silent success.
- Status publication is non-blocking and does not create another agent stream.

## Verification

- `cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/mcp/server ./internal/agent/runtime/lifecycle ./cmd/agentctl`

Start with RED adapter tests for filter decisions and attempt rotation, MCP
server hook tests with two connections, and passthrough tests for nil strategy
and materialization failure.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/mcp_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/manager_test.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/agent_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/cmd/agentctl/main.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_test.go`

## Dependencies

- Task 01 defines the evidence event and reason-code contract.

## Parallelism

Parallel-safe with Task 06 only after Task 01. This task exclusively owns the
live agentctl and passthrough observation paths.

## Inputs

- Current ACP capability filter and new/load/reset handlers
- `mcp-go` session and request hooks
- `process.Manager.updatesCh`
- Existing passthrough strategy/materialization paths

## Output contract

Report the RED assertions, attempt-creation boundary, emitted hook evidence,
passthrough behavior, payload redaction audit, exact test results, files
changed, blockers, and risks. Mark this task `done` and update its plan
checkbox.
