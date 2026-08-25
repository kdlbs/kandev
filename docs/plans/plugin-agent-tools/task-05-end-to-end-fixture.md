---
id: "05-end-to-end-fixture"
title: "Managed fixture end-to-end proof"
status: in_progress
wave: 4
depends_on: ["04-runtime-propagation-backend-dispatch"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/agent-tools.md"
adr: "../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md"
---

# Task 05: Managed fixture end-to-end proof

## Acceptance

- The managed fixture declares and implements a tool that returns its arguments
  and immutable Kandev-bound context, plus controlled error/timeout behavior.
- A real subprocess plus MCP JSON-RPC integration test proves discovery,
  invocation, schema rejection before RPC, structured result mapping, and
  cancellation/no-retry behavior.
- Live removal/recovery tests prove one list-changed notification per effective
  change, stale snapshot rejection, fail-closed calls after removal, and clean
  subprocess/server teardown.

## Verification

```bash
cd apps/backend && go test -race -run 'Test.*PluginAgentTool' ./internal/plugins/runtime ./internal/mcp/server ./internal/mcp/handlers ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/plugins/runtime/testdata/fixtureplugin/main.go`
- `apps/backend/internal/plugins/runtime/manager_test.go`
- `apps/backend/internal/mcp/server/plugin_tools_integration_test.go`
- `apps/backend/internal/backendapp/plugin_agent_tools_integration_test.go`

## Dependencies

Task 04.

## Parallelism

Sequential. It exercises the complete implementation and may adjust only test
seams, not redefine public contracts.

## Inputs

- Every spec scenario
- Existing managed runtime fixture, MCP external integration test, initialized
  MCP session, and plugin runtime teardown patterns

## Risks

- Subprocess tests can leak or flake if they use sleeps. Use bounded contexts,
  readiness signals, explicit cleanup, and deterministic invocation counters.
- The integration must prove no retry rather than only asserting one successful
  response.

## Output contract

Report full-path evidence, subprocess artifacts and cleanup, invocation counts,
notification counts, exact commands/results, and task/plan status updates.

## Results

- Extended `cmd/plugin-fixture` with the manifest-declared `test_echo` tool,
  which returns arguments and bound task/surface context over the SDK RPC.
- Built `.build/kandev-plugin-e2e-1.0.0.tar.gz`, sideloaded it into a running
  backend, and verified the installed record is active with `test_echo`.
- Verification: `go test ./cmd/plugin-fixture` and `make e2e-plugin-package`
  passed. The SDK gRPC round-trip test also passes in `pkg/pluginsdk`.
- Full MCP JSON-RPC subprocess integration and lifecycle hot-removal coverage
  remain future work.
