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

## Out of scope

- Deadlines for task, integration, plugin, or agent-launch business operations.
- A new user-interface alert for a stalled MCP request.
- Traffic between an agent and a third-party MCP server.
