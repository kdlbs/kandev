---
status: draft
system: agents
requirements:
  - REQ-AGENTS-MCP-PROTOCOL-001
created: 2026-08-30
owners:
  - Kandev
---

# MCP Protocol Compatibility System Design

## Purpose and boundaries

This design defines how every Kandev-owned MCP server supports the modern and
legacy MCP protocol eras on one endpoint. It covers the shared server, route
wiring, application authorization, tool profiles, and protocol evidence.

The design does not change third-party MCP servers. Agent adapters still pass
those server definitions directly to clients. The
[external MCP requirement](../../integrations/requirements/external-mcp.md)
owns the public bearer-token boundary. The
[MCP observability design](../../platform/system-design/mcp-session-observability.md)
owns attachment status and evidence retention.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-AGENTS-MCP-PROTOCOL-001` | [Shared server](#shared-server), [Protocol selection](#protocol-selection), [Compatibility boundaries](#compatibility-boundaries), [Failure behavior](#failure-behavior) |

## Shared server

`apps/backend/internal/mcp/server` remains the single implementation for task,
task-title, configuration, external, Office, and automation tool profiles.
The package continues to construct Streamable HTTP and legacy SSE handlers.

The backend upgrades `github.com/mark3labs/mcp-go` from `v0.43.2` to the v1
line that supports MCP `2026-07-28`. The first planned target is
`v1.0.0-beta.1`. Implementation must re-check for a newer compatible v1
release before it changes `go.mod`.

The shared server uses the SDK's dual-era request handling. Kandev does not
fork protocol parsing or maintain a local modern-protocol shim.

## Routes and profiles

Agentctl keeps these routes:

- `/mcp` for negotiated Streamable HTTP;
- `/sse` and `/message` for legacy SSE.

The external backend keeps `/mcp`, `/mcp/sse`, and `/mcp/message`. No route or
configuration key identifies a protocol era.

Agent injection continues to prefer the Streamable HTTP `/mcp` URL and retains
SSE as a compatibility transport. Passthrough adapters keep their current
client-specific configuration format.

All tool profiles use the same protocol handler. Protocol selection must not
change profile filtering, dynamic plugin tools, annotations, result data, or
Kandev task and session identity.

## Protocol selection

The SDK selects the protocol era for each request.

1. A modern automatic client sends `server/discover` to `/mcp`.
2. Kandev advertises `2026-07-28` and supported legacy versions.
3. The client selects a version.
4. Modern calls include the required protocol metadata and HTTP headers.
5. Kandev validates and handles each call as a stateless modern request.

A client can skip discovery and send a valid direct modern request. Kandev
handles it with the same modern path. A legacy client keeps the existing
`initialize` sequence and session behavior.

The TypeScript MCP SDK v2 does not enable automatic negotiation by default.
Client compatibility tests must enable its automatic mode when they prove the
modern path. Default legacy mode remains a valid compatibility test.

## Application and security boundaries

Protocol negotiation changes the wire contract only. Tool handlers keep their
existing Kandev authorization and application context.

Agentctl binds task and session identity from the server instance. A client
cannot select another Kandev task or session through MCP metadata. The external
backend resolves its personal access token on every HTTP request. This is
required because modern requests are stateless and can arrive on separate HTTP
connections.

The modern tool-list cache metadata stays conservative for dynamic tool
profiles. Kandev advertises no shared cache and no positive cache lifetime.
This prevents one external caller or attachment from reusing another caller's
dynamic tool list.

## Compatibility boundaries

Kandev keeps these clients on their existing explicit behavior:

- `apps/backend/internal/jira/mcp_client.go` continues to request
  `2025-06-18` until a separate change adopts automatic negotiation.
- `apps/backend/cmd/mock-agent` continues to use legacy SSE.
- Configured third-party MCP servers continue to flow from Kandev to the agent
  without a Kandev protocol proxy.

The compatibility test matrix covers:

| Client | Kandev server | Expected result |
| --- | --- | --- |
| Modern automatic client | Dual-era `/mcp` | Select `2026-07-28` and use tools. |
| Modern direct client | Dual-era `/mcp` | Use tools without discovery. |
| Legacy client | Dual-era `/mcp` | Complete initialize and use tools. |
| Legacy SSE client | Legacy SSE routes | Use tools without behavior changes. |
| Modern automatic client | Legacy-only fixture | Select a shared legacy version or use the client's documented fallback. |

Tests also cover concurrent modern and legacy calls, external bearer-token
authorization, every Kandev tool profile, and a dynamic plugin-tool list.

## Attachment observability

Legacy sessions retain their opaque MCP connection ID, initialize evidence,
tool-list evidence, tool-call evidence, and close evidence.

Modern MCP is stateless. A successful discovery or other validated modern
request records protocol-accepted evidence against the current attachment
attempt. A successful `tools/list` still makes the Kandev server Active and
stores the safe catalog. Modern requests do not create a fabricated connection
ID. The end of one HTTP request does not record connection-closed evidence.

The event model adds a general `protocol_accepted` evidence kind. Existing
`initialize_observed` history remains readable and continues to represent
legacy initialization.

## Failure behavior

Kandev returns the SDK's protocol error for missing, malformed, or unsupported
modern metadata. The server does not downgrade the same invalid request to the
legacy handler. A client can make another request through its own documented
negotiation or fallback path.

Failure in one protocol era does not disable the other era. The endpoint does
not use a runtime feature flag because the SDK selects the era per request and
the legacy behavior remains available.

## Persistence and migration

The protocol change needs no database migration. The optional
`protocol_accepted` evidence kind extends the existing bounded attachment
history. Readers must continue to accept stored `initialize_observed` events.

## Related decisions

- [Use one dual-era MCP endpoint](../../../decisions/2026-08-30-dual-era-mcp-protocol.md)
- [Keep MCP attachment evidence session owned](../../../decisions/2026-07-30-session-owned-mcp-observability.md)
- [Compose MCP tool profiles from typed context](../../../decisions/2026-08-08-mcp-tool-profiles.md)
