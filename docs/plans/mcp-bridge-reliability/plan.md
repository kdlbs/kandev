---
created: 2026-09-04
status: done
requirements:
  - REQ-AGENTS-MCP-BRIDGE-RELIABILITY-001
system_design:
  - ../../specs/agents/system-design/mcp-bridge-reliability.md
legacy_specs: []
---

# Implementation Plan: MCP Bridge Reliability

## Overview

Make backend-backed MCP requests use the current dispatcher and explicit
transport ownership. Add regression tests before the implementation changes.

## Scope

### In scope

- Remove the recovered-stream dispatcher capture race.
- Return correlated errors for missing handlers and empty request responses.
- Make the existing send timeout detect an absent stream consumer.
- Release pending requests after write errors and owning-stream disconnects.
- Add default-level diagnostic events without tool arguments.

### Out of scope

- Business-operation deadlines for tools and agent launch.
- User-interface stall indicators.
- Third-party MCP traffic.
- Changes to MCP tool schemas or results.

## Technical approach

### Lifecycle dispatcher binding

Update `StreamManager` to return a stable execution-bound handler proxy. The
proxy reads the current dispatcher and scope functions for each request under
a read-write mutex.

Update manager setters to use the synchronized `StreamManager` boundary. Keep
task, session, execution, user, and automation scope derived from the current
server-owned execution.

### Backend request handling

Update `readUpdatesStream` to write an error when a direct caller supplies no
handler. Update `dispatchMCPRequest` to reject an empty response for a request.

Record request receipt at `Info`. Record terminal bridge errors at `Warn` or
`Error` without payload values.

### Agentctl request ownership

Make `ChannelBackendClient.requestCh` unbuffered. Replace bare response
channels with pending result records that can carry a response or transport
error.

Assign one identifier to each backend stream. Bind a request after its writer
sends it. Complete the request after a write error or the owning stream closes.
Do not cancel requests owned by an overlapping replacement stream.

## Tests

- `AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.1` maps to lifecycle handler-binding
  tests in `internal/agent/runtime/lifecycle`.
- `AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.2` maps to channel-send tests in
  `internal/mcp/server`.
- `AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.3` and `.4` map to stream-dispatch
  tests in `internal/agent/runtime/agentctl`.
- `AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.5` maps to stream ownership tests in
  `internal/agentctl/server/api` and channel-client tests.
- `AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.6` and `.7` map to existing behavior
  tests plus new log assertions.

## Work orders

- [x] [Task 01: Stabilize the internal MCP bridge](task-01-stabilize-internal-mcp-bridge.md)

## Verification results

- `cd apps/backend && go test ./internal/mcp/server ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle -count=1`
  passed 3,560 tests in four packages.
- `cd apps/backend && go test -race ./internal/mcp/server ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle -run 'MCP|ChannelBackendClient|StreamUpdates' -count=1`
  passed 109 tests in four packages.

## Risks

- Stream replacement can overlap with old-stream cleanup. Request ownership
  must use the stream identifier and must not use a global reset.
- Dispatcher and scope setters can run after recovery starts. All related
  reads and writes must use the same synchronization boundary.
- An unbuffered request channel changes scheduling. Tests must cover close,
  reset, context cancellation, and concurrent requests.
