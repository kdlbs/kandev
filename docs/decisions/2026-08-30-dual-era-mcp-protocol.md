# ADR-2026-08-30-dual-era-mcp-protocol: Use One Dual-Era MCP Endpoint

**Status:** accepted
**Date:** 2026-08-30
**Area:** backend, agentctl, protocol, security

## Context

Kandev currently uses `github.com/mark3labs/mcp-go v0.43.2`. It supports MCP
through `2025-06-18`. MCP `2026-07-28` adds an optional discovery exchange and
new request metadata. Modern requests are stateless, while Kandev must still
serve clients that use legacy initialize sessions or SSE.

All Kandev MCP tool profiles share one server package. They include task,
task-title, configuration, Office, automation, and external profiles. A second
endpoint or a hand-written protocol fork would duplicate this shared security
and tool-selection boundary.

The mark3labs v1 line supports modern and legacy MCP on one handler. The
official `github.com/modelcontextprotocol/go-sdk` also supports both eras, but
moving to it would require a larger rewrite of handlers, hooks, transports, and
tests. The mark3labs v1 beta has known gaps in the optional MCP Tasks extension
and authorization hardening. Neither gap is required for Kandev's current tool
server change.

## Decision

Kandev will serve modern and legacy MCP from the existing `/mcp` routes.

- Upgrade `github.com/mark3labs/mcp-go` to a v1 release that supports MCP
  `2026-07-28`. Start with `v1.0.0-beta.1`, unless a newer compatible v1
  release exists when implementation starts.
- Let the SDK select the protocol era for each request. Do not add `/mcp/v2`
  or a protocol feature flag.
- Support automatic discovery and valid direct modern requests.
- Preserve the legacy initialize path and legacy SSE routes.
- Keep all Kandev tool, profile, identity, and authorization behavior
  independent from the selected era.
- Keep configured third-party MCP servers as direct agent-to-server
  connections. Kandev does not negotiate on their behalf.
- Treat modern observations as attachment-attempt evidence. Do not fabricate a
  connection ID or a disconnect for each stateless HTTP request.
- Do not add the MCP Tasks extension or change OAuth and authorization flows in
  this work.

## Consequences

Modern clients can select `2026-07-28` without a new Kandev route. Legacy
clients keep working on the same server and through SSE. Kandev maintains one
tool and authorization implementation.

The implementation takes a prerelease dependency until mark3labs publishes a
stable v1. The focused compatibility suite must therefore protect discovery,
direct modern calls, legacy initialize calls, SSE, authorization, hooks, and
dynamic tools. If no acceptable mark3labs v1 release exists at implementation
time, work stops at dependency selection instead of adding a local protocol
fork.

Modern attachment evidence is less connection-specific than legacy evidence.
The status can still prove protocol acceptance, tool loading, and tool use for
the current attempt.

This decision amends the connection-only wording in
[ADR-2026-07-30-session-owned-mcp-observability](2026-07-30-session-owned-mcp-observability.md).

## Alternatives Considered

- **Add `/mcp/v2`.** Rejected because the client and server can negotiate on
  the existing endpoint. Another route would duplicate configuration and
  increase migration work.
- **Replace mark3labs with the official Go SDK now.** Rejected because it
  causes a broad server rewrite without a compatibility benefit for this
  change. Kandev can reconsider it when another requirement needs the official
  SDK's distinct features.
- **Wait and keep legacy-only support.** Rejected because a compatible upgrade
  path exists and modern clients need a Kandev server that can negotiate the
  new protocol.
- **Implement the modern wire contract inside Kandev.** Rejected because
  protocol parsing, validation, and negotiation belong in a maintained MCP
  SDK.
- **Gate modern MCP with a runtime feature flag.** Rejected because protocol
  selection is per request and does not replace the legacy path. The
  compatibility suite is the safer release control.
