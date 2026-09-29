---
id: "02-mcp-startup-order"
title: "MCP startup ordering"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-BRIDGE-RELIABILITY-001
acceptance_criteria:
  - AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.1
  - AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.3
  - AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.8
system_design:
  - ../../specs/agents/system-design/mcp-bridge-reliability.md
---

# Task 02: MCP startup ordering

## Summary

Complete dispatcher and scope wiring before the first recovered or newly
launched agent can request MCP tools. Keep bootstrap health available and
readiness unsuccessful until recovery and final startup checks complete.

## In scope

- Separate backend dependency construction from launch-capable activation.
- Install the complete real dispatcher and scope callbacks before Manager.Start,
  orchestrator startup reconciliation, automation, run scheduling, and plugin
  task-launch callbacks become active.
- Keep watcher-before-recovery, required-store admission, cancellation, restore
  cleanup, schema-version recording, and readiness handoff ordering intact.
- Add a production-wired startup test whose first recovery/launch callback issues
  `mcp.list_plugin_tools` through the execution-bound handler proxy.

## Out of scope

- No changes to Office CEO permission behavior, listener binding, authentication,
  MCP deadlines, provider manifests, or external plugin implementations.
- No duplicate early registry, polling sleep, or request wait that blocks boot.

## Acceptance

- First discovery during recovery and first launch succeeds with the intended
  scope. On current code this regression reaches the unconfigured dispatcher.
- Handler construction failure/cancellation prevents dependent consumers and
  readiness; partial resources are cleaned up exactly once.
- Retained terminal events still reach their watcher, missing-handler defensive
  errors remain correlated, and bootstrap health/readiness semantics remain.

## Verification

Run the regression first and record its expected failure, then implement and run:

```bash
(cd apps/backend && go test ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl -count=1)
(cd apps/backend && go test -race ./internal/backendapp ./internal/agent/runtime/lifecycle -run 'StartupMCP|StartOrchestrator|Bootstrap|MCPHandler|Recovery' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/backendapp/startup_mcp_order_test.go (new)`
- `apps/backend/internal/backendapp/startup_order_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/mcp_identity.go`
- `apps/backend/AGENTS.md (startup convention if changed)`

## Dependencies

None. Execute in manifest order in the primary session.

## Risks

Constructors can start background work. Audit launch-capable side effects and cleanup registration, including dynamic plugin task-write dependencies; a helper-only ordering test is insufficient.

## Parallelism

`sequential`

## Inputs

- [Plan and incident evidence](plan.md).
- [Requirement REQ-AGENTS-MCP-BRIDGE-RELIABILITY-001](../../specs/agents/requirements/mcp-bridge-reliability.md).
- [System design](../../specs/agents/system-design/mcp-bridge-reliability.md).

## Results

- Backend construction and route/dispatcher wiring are separated from activation; HTTP server construction and `registerMCPAndDebugRoutes` run before `lifecycleMgr.Start`.
- `MCPHandlerFor` is exposed on lifecycle Manager, and recovery callbacks have full access to the real dispatcher and execution-bound scopes.
- Orchestrator event watcher is subscribed before recovery and downstream consumers are activated only after recovery completes.
- Tests in `startup_mcp_order_test.go` and `startup_order_test.go` pass with race detector enabled.
