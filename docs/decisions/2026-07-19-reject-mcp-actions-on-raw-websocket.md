# ADR-2026-07-19-reject-mcp-actions-on-raw-websocket: Reject MCP Actions on the Raw WebSocket

**Status:** accepted
**Date:** 2026-07-19
**Area:** backend, protocol

## Context

Kandev's agent stream, external MCP endpoint, and browser WebSocket gateway share
one backend action dispatcher. Task-mode MCP adapters inject task and session
identity into internal `mcp.*` action payloads, but a raw `/ws` client can
currently invoke those same actions and forge the injected fields. A
direct-parent-only stop or interrupt therefore cannot treat payload identity as
trusted unless the transport boundary is enforced.

## Decision

The browser/raw WebSocket client path in
`apps/backend/internal/gateway/websocket/client.go` rejects every action whose
name starts with `mcp.` before it reaches the shared dispatcher. Internal MCP
actions are reachable only through an MCP adapter:

- mode-scoped MCP requests arriving through an authenticated agentctl agent
  stream;
- the external MCP server's mode-specific registered tool set; or
- trusted in-process backend callers.

Handlers may treat identity fields injected by the task MCP server as
transport-trusted only under this boundary. New `mcp.*` actions must remain
unavailable through raw `/ws`, and gateway regression tests must prove that a
forged payload cannot reach its handler. The public WebSocket catalog may list
these action names as internal transport shims, but must state that raw clients
cannot dispatch them.

## Consequences

Task-scoped authorization can use server-injected identity without accepting a
browser-forged task ID. Frontend code must continue using public non-MCP
WebSocket actions; it cannot reuse internal MCP action names. Agent-stream and
external MCP behavior stays unchanged because those adapters do not traverse
the raw gateway client dispatch path.

This is a transport-origin boundary, not user authentication. Kandev's external
MCP endpoint retains its separately documented open-network policy and remains
limited by the tools registered for its mode.

## Alternatives Considered

An exported context provenance marker carrying session/execution identity was
rejected for this change because it would require threading a new security
principal through the agent stream, dispatcher, and every identity-sensitive
handler. A per-action gateway denylist was rejected because each new MCP
mutation could be accidentally omitted. Trusting `sender_task_id` after only
database relationship checks was rejected because the raw caller controls that
field. A separate dispatcher for every MCP mode was rejected as a larger
routing refactor than the required prefix boundary.
