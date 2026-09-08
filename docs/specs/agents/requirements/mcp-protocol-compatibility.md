---
status: draft
system: agents
created: 2026-08-30
owners:
  - Kandev
---

# MCP Protocol Compatibility Requirements

## Overview

Kandev exposes agent-facing MCP servers for tasks, task titles, configuration,
Office, automations, and external integrations. New MCP clients can use the
2026-07-28 protocol. Existing clients still use the 2025 protocol family and
legacy SSE transports.

The agent system owns this contract because it defines the MCP behavior that
all Kandev tool profiles expose to agents. The integrations system continues to
own external endpoint authentication and tool permissions.

## Terminology

- **Modern protocol:** MCP protocol version `2026-07-28`.
- **Legacy protocol:** An MCP protocol version that predates `2026-07-28`.
- **Automatic negotiation:** A client sends `server/discover`, selects a
  compatible version, and sends requests for that version.
- **Direct modern request:** A valid modern request that does not first send
  `server/discover`.

## Requirements

### REQ-AGENTS-MCP-PROTOCOL-001: Dual-era Kandev MCP compatibility

**Intent:** Agents must use the best MCP protocol that they share with Kandev
without breaking agents that support only the current legacy protocol.

#### Acceptance criteria

- **AC-AGENTS-MCP-PROTOCOL-001.1:** When a client supports and enables
  automatic negotiation, the existing Kandev `/mcp` endpoint shall advertise
  `2026-07-28` and complete requests with that version.
- **AC-AGENTS-MCP-PROTOCOL-001.2:** When a client sends a valid direct modern
  request, the existing Kandev `/mcp` endpoint shall process it without a
  preceding discovery request.
- **AC-AGENTS-MCP-PROTOCOL-001.3:** When a legacy client sends its existing
  initialize and tool requests, Kandev shall keep the legacy protocol behavior
  and tool results.
- **AC-AGENTS-MCP-PROTOCOL-001.4:** When modern and legacy clients use the same
  Kandev endpoint at the same time, each request shall use the protocol era
  selected by that client.
- **AC-AGENTS-MCP-PROTOCOL-001.5:** Task, task-title, configuration, Office,
  automation, and external MCP profiles shall expose the same authorized tools
  and application behavior in both protocol eras.
- **AC-AGENTS-MCP-PROTOCOL-001.6:** Existing Kandev SSE endpoints shall remain
  available for clients that do not support Streamable HTTP.
- **AC-AGENTS-MCP-PROTOCOL-001.7:** When Kandev passes a configured third-party
  MCP server to an agent, the agent and that server shall negotiate their own
  protocol without Kandev changing the server definition.
- **AC-AGENTS-MCP-PROTOCOL-001.8:** When a request declares an unsupported or
  malformed modern protocol version, Kandev shall return a protocol error and
  shall not silently reinterpret that request as legacy MCP.

## Out of scope

- Kandev does not implement the MCP Tasks extension as part of this change.
- Kandev does not change OAuth or MCP authorization flows as part of this
  change.
- Kandev does not proxy third-party MCP traffic.
- Kandev does not require its internal Jira client or mock agent to adopt the
  modern protocol in this change.
