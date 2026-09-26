---
status: active
system: agents
created: 2026-09-04
owners:
  - Kandev
---

# MCP Bridge Reliability Requirements

## Overview

Kandev agents use a local MCP server in agentctl. Backend-backed tools cross an
internal WebSocket bridge before the backend handles them.

The agent system owns this contract because the bridge is part of every
Kandev-managed agent runtime. Task and integration systems own the behavior of
their tools after the bridge delivers a request.

## Terminology

- **MCP bridge:** The request and response path between the agentctl MCP server
  and the Kandev backend.
- **Backend stream:** The WebSocket connection that carries MCP requests from
  agentctl to the backend and returns correlated responses.

## Requirements

### REQ-AGENTS-MCP-BRIDGE-RELIABILITY-001: Reliable backend-backed MCP delivery

**Intent:** An agent must receive a result or a descriptive bridge error when
it calls a Kandev tool. A lost internal request must not consume the outer MCP
client timeout without diagnostic evidence.

#### Acceptance criteria

- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.1:** When a new or recovered agent
  stream receives an MCP request, Kandev shall use the current configured
  dispatcher. Startup order shall not leave the stream bound to an earlier
  empty dispatcher.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.2:** When no backend stream consumes
  an MCP request, agentctl shall return a descriptive error within five
  seconds and shall record the action at the default log level.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.3:** When the backend stream has no
  MCP dispatcher, Kandev shall return a correlated error response and shall
  record the request identity at the default log level.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.4:** When a dispatcher returns no
  response for a request, Kandev shall return a correlated error response. It
  shall not silently discard the request.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.5:** When the backend stream closes
  before a response arrives, agentctl shall fail each request owned by that
  stream with a descriptive disconnect error.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.6:** Successful tool results and
  intentional user or parent question waits shall keep their current behavior.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-001.7:** Default logs shall identify the
  action, request, and session for accepted requests and terminal bridge
  errors. These logs shall not contain tool arguments.

### REQ-AGENTS-MCP-BRIDGE-RELIABILITY-002: Empty response payload reporting

**Intent:** A backend client must report an empty response payload as a bridge
error when a caller expects a result. It must not turn an empty payload into a
successful result with an unset value.

#### Acceptance criteria

- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.1:** When a successful response has
  zero payload bytes and the caller supplied a result sink, the backend client
  shall return an error.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.2:** The error shall identify the
  empty-payload condition and include the action name. It shall not include the
  request payload or any tool argument.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.3:** When the caller supplied no
  result sink, a zero-byte payload shall keep the existing successful behavior.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.4:** An error response shall keep the
  existing backend-error behavior, even when its payload has zero bytes.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.5:** A valid empty object shall keep
  decoding successfully. The task-plan tool shall keep its current no-plan
  text for that object.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.6:** A valid `null` payload shall keep
  its current decoding behavior and shall not become an empty-payload error.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.7:** A non-empty invalid or
  unassignable payload shall keep returning its decoding error.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.8:** The channel and dispatcher
  backend clients shall use the same response classification rules.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.9:** Each empty-payload error shall
  create one warning with the request ID and action. The channel client shall
  also include the session ID and duration. Warnings shall not include tool
  arguments.
- **AC-AGENTS-MCP-BRIDGE-RELIABILITY-002.10:** The error shall reach the caller
  on its first occurrence. This behavior shall add no retry or backoff.

## Out of scope

- Deadlines for task, integration, plugin, or agent-launch business operations.
- A new user-interface alert for a stalled MCP request.
- Traffic between an agent and a third-party MCP server.
- Treating valid `null` payloads as transport errors.
- Reproducing or assigning a root cause to a historical empty-result incident.
