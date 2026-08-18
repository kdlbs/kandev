# ADR-2026-08-16-session-mcp-tool-catalog: Keep MCP Tool Catalogs Session Owned

**Status:** superseded by 2026-08-18-session-mcp-tool-definition-details
**Date:** 2026-08-16
**Area:** backend, agentctl, frontend, protocol, security

## Context

The chat toolbar shows session-owned MCP attachment status. It does not show
which tools the active agent received. Kandev observes its own `tools/list`
response, but third-party MCP servers connect directly to the agent.

Collecting every third-party catalog requires Kandev to connect to, or proxy,
those servers. That change expands credential, transport, and
failure ownership beyond the existing observability boundary.

The follow-up decision
[ADR-2026-08-18-session-mcp-tool-definition-details](2026-08-18-session-mcp-tool-definition-details.md)
supersedes this decision. It keeps session ownership but adds bounded input
schemas and deterministic token estimates.

## Decision

Kandev stores a bounded tool catalog only for its built-in session MCP server.
The catalog comes from the actual `tools/list` response that Kandev serves to
the agent.

Each stored entry contains only the tool name and description. The current
attempt stores at most 128 entries. Each description contains at most 1,024
UTF-8 bytes. The report keeps the total tool count and a truncation marker.

The catalog is sorted by tool name before publication. A superseded attempt
keeps its total count, but Kandev removes its catalog entries. Kandev renders
descriptions as plain text.

The report does not store schemas, annotations, arguments, results, prompts,
credentials, or endpoint configuration. Kandev does not connect to or proxy a
third-party server for catalog discovery.

The catalog uses the existing session attachment event, persistence, boot
hydration, and WebSocket path. Kandev does not add a separate catalog endpoint.

## Consequences

Users can inspect the exact Kandev tools that the current attachment attempt
loaded. The catalog also includes enabled plugin tools after Kandev serves an
updated `tools/list` response.

Third-party rows remain asymmetric. They show Kandev-owned attachment metadata,
but they explain that their tool catalogs are unavailable.

The bounds limit session metadata growth. A large catalog can be incomplete in
the UI, but the UI shows the stored count and the full count.

## Alternatives Considered

### Connect to every configured server from the backend

Rejected. This path duplicates agent connections and requires Kandev to
own third-party credentials, process cleanup, and network failure behavior.

### Proxy all MCP traffic through Kandev

Rejected. This path changes transport ownership, latency, credentials, and
failure domains for every agent integration.

### Add a live agentctl catalog endpoint

Rejected. The MCP server already emits the exact served catalog through its
session-owned evidence path. A second request path creates different live
and reload behavior.

### Persist complete tool schemas

Rejected. Names and descriptions meet the explorer need. Schemas increase the
metadata size and can expose more provider-controlled content.
