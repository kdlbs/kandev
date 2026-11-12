# ADR-2026-11-11-instance-scoped-builtin-mcp-credential: Instance-scoped built-in MCP credential

**Status:** accepted
**Date:** 2026-11-11
**Area:** backend, protocol

## Context

Each agentctl instance serves Kandev's built-in MCP server for one task session.
Tool calls take their session identity from the server instance, and
[ADR-2026-07-19](2026-07-19-reject-mcp-actions-on-raw-websocket.md) lets
handlers trust that identity. The built-in MCP routes are exempt from instance
authentication, though. Any local process that reaches the port acts as that
session.

On 2026-11-02 and 2026-11-06, an agent in another workspace called a hard-coded
instance port. Kandev created its clarifications in unrelated sessions, cancelled
those sessions' turns, and delivered the answers to the wrong agents. Port
reuse made a stale port point at a different session without any error.

## Decision

Each agentctl instance issues its own random MCP caller credential when it is
created. The built-in MCP routes require it as `Authorization: Bearer` when
enforcement is enabled for that instance. The credential:

- is separate from the control credential and never authorizes another route;
- is returned to the backend only through a control-authenticated instance
  read, and lives in agentctl memory for the instance lifetime;
- is delivered to agents as a header on the built-in server entries, using each
  provider's existing header carrier.

Built-in entry construction stays centralized. Permission and managed-tool
policies recognize the built-in entry only with its exact expected header.
Enforcement is a per-instance value derived from a runtime release toggle. Its
default stays off until provider compatibility evidence exists.

This is protection against accidental or blind cross-session access. It is not
isolation between processes of the same operating-system user.

## Consequences

- A blind call, a port scan, or a stale port gets 401 instead of acting on an
  unrelated session.
- Every built-in delivery path must carry the header. Policy code can no longer
  identify the built-in entry by an empty header set.
- Some paths expose the credential in argv (Codex passthrough, Claude ACP
  launches) or in workspace files (`.cursor/mcp.json`, `.pi/mcp.json`). The
  credential expires with the instance.
- A provider that ignores configured headers loses Kandev tools under
  enforcement. The rollout toggle is the recovery path.
- Mixed backend and agentctl versions degrade to today's unauthenticated
  behavior rather than failing.

## Alternatives Considered

- **Reuse the control credential.** Rejected because it authorizes git, shell,
  file, and process routes, so agents would gain control-plane authority.
- **Credential in the URL path or query.** Rejected because URLs are logged and
  compared exactly in policy code. `builtinPassthroughEndpoint` also rejects
  query strings, and the URL would leak the same way as a header.
- **Peer-process verification over loopback.** This would map the TCP peer to a
  process and check it against the agent's process group. Rejected because it
  is Linux-specific and fails across Docker, SSH, Kubernetes, and Sprite
  boundaries.
- **Unix domain sockets or stdio bridge.** Rejected because providers accept
  HTTP or SSE built-in servers, and Kandev has no stdio bridge.
- **Validate only the claimed session in the tool payload.** Rejected because the
  server already binds identity. The defect is reachability by the wrong caller,
  and a payload claim is caller-controlled.
- **Reject new questions on sessions without an active turn.** This is useful as
  defense in depth. It was not selected as the boundary because it leaves every
  other tool reachable and does not cover a session that has a turn running.
