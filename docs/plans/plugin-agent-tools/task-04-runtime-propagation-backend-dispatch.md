---
id: "04-runtime-propagation-backend-dispatch"
title: "Runtime propagation and backend dispatch"
status: done
wave: 3
depends_on: ["02-plugin-catalog-invocation", "03-agentctl-dynamic-registry"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/agent-tools.md"
adr: "../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md"
---

# Task 04: Runtime propagation and backend dispatch

## Acceptance

- Launch and resume profiles include the current catalog, while Configuration
  and External surfaces expose no plugin tools.
- A scoped backend MCP action verifies the bound task/session pair and current
  plugin declaration before one plugin RPC, and maps valid text/structured/error
  results back to agentctl.
- A serialized backendapp refresher pushes each effective snapshot to every
  active execution, continues after individual failures, and leaves later
  launch/resume or refresh able to repair stale runtime state.

## Verification

```bash
cd apps/backend && go test -race ./internal/mcp/handlers ./internal/mcp/scope ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/backendapp
```

## Files likely touched

- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/plugin_tools.go`
- `apps/backend/internal/mcp/handlers/plugin_tools_test.go`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_mcp_mode_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_mcp_plugin_tools_test.go`
- `apps/backend/internal/backendapp/plugin_tool_refresher.go`
- `apps/backend/internal/backendapp/plugin_tool_refresher_test.go`
- `apps/backend/internal/backendapp/main.go`

## Dependencies

Tasks 02 and 03.

## Parallelism

Sequential. This task joins shared plugin, MCP, orchestrator, lifecycle, and
backendapp contracts.

## Inputs

- Spec `Permissions`, `Failure Modes`, and hot-refresh scenarios
- Root/backend auth scoping guidance for in-session MCP
- Existing provider live refresher, `resolveTaskSessionMCPProfile`, lifecycle
  `SetMcpProvidersForSession`, and MCP dispatcher patterns

## Risks

- A handler that trusts agent-supplied task/session IDs creates a cross-user
  authorization defect. Use only server-bound identity and authorize first.
- Out-of-order or blocking catalog propagation can restore stale tools or stall
  plugin supervision.

## Output contract

Report the exact identity/authorization chain, refresh serialization and failure
behavior, launch/resume wiring, commands and results, and task/plan status
updates.

## Results

- Added `mcp.list_plugin_tools` and `mcp.invoke_plugin_tool` actions with task /
  session binding checks, surface filtering, and service dispatch.
- Wired the plugin service into backend MCP handler registration and preserved
  existing identity scoping.
- Added the agentctl plugin-tool control endpoint, lifecycle fan-out, and a
  coalescing backendapp refresher wired to plugin lifecycle notifications.
- Agentctl retains the complete revisioned catalog and applies surface filters
  while rebuilding the active MCP profile, so live surface changes do not lose
  declarations.
- Verification: `go test -race ./internal/mcp/handlers ./internal/mcp/scope
  ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle
  ./internal/backendapp` passed.
