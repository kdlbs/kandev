---
id: "01-stabilize-internal-mcp-bridge"
title: "Stabilize the internal MCP bridge"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-BRIDGE-RELIABILITY-001
acceptance_criteria:
  - AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.1
  - AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.2
  - AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.3
  - AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.4
  - AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.5
  - AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.6
  - AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.7
system_design:
  - ../../specs/agents/system-design/mcp-bridge-reliability.md
---

# Task 01: Stabilize the Internal MCP Bridge

## Summary

Add failing regression tests for the startup race and lost-response paths.
Then make every bridge request receive a result or a transport error.

## In scope

- Add synchronized, request-time dispatcher resolution to `StreamManager`.
- Preserve server-derived execution and principal scope.
- Return correlated errors for an unavailable handler and an empty response.
- Make the request channel unbuffered.
- Bind pending requests to the backend stream that sends them.
- Release pending requests after write errors and stream disconnects.
- Add safe default-level diagnostic events.

## Out of scope

- Add a common deadline to all backend handlers.
- Change user-question or parent-question wait behavior.
- Add user-interface behavior.
- Change public MCP protocol shapes.

## Acceptance

- A recovered stream created before dispatcher setup uses the dispatcher after
  setup without a reconnect.
- Each missing-delivery path returns one correlated error and records a safe
  diagnostic event.
- Existing successful, cancellation, reset, close, and long-wait paths pass
  without behavior changes.

## Verification

```bash
go test ./internal/mcp/server ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle -count=1
go test -race ./internal/mcp/server ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle -run 'MCP|ChannelBackendClient|StreamUpdates' -count=1
```

## Files likely touched

- `apps/backend/internal/mcp/server/backend_client.go`
- `apps/backend/internal/mcp/server/backend_client_test.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/agent_mcp_stream_test.go`
- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/agent/runtime/agentctl/agent_mcp_stream_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager.go`
- `apps/backend/internal/agent/runtime/lifecycle/mcp_identity.go`
- `apps/backend/internal/agent/runtime/lifecycle/mcp_handler_binding_test.go`

## Dependencies

None.

## Risks

- Old and replacement streams can overlap during reconnect.
- A completion path can race with response delivery, reset, context
  cancellation, or client close.

## Parallelism

`sequential`

## Inputs

- [MCP bridge reliability requirements](../../specs/agents/requirements/mcp-bridge-reliability.md)
- [MCP bridge reliability system design](../../specs/agents/system-design/mcp-bridge-reliability.md)
- [GitHub issue 3364](https://github.com/kdlbs/kandev/issues/3364)

## Results

Implemented synchronized request-time dispatcher resolution, correlated error
responses, unbuffered request publication, and per-stream pending-request
ownership. Added regression tests for startup wiring, absent consumers,
unavailable or empty dispatch, write failure, disconnect, and replacement
stream isolation.

The targeted suite passed 3,559 tests in four packages. The race-focused suite
passed 108 tests in the same packages.
