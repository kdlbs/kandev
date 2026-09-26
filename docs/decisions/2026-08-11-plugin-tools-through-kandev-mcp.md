# ADR-2026-08-11-plugin-tools-through-kandev-mcp: Route Plugin Agent Tools Through Kandev MCP

**Status:** accepted
**Date:** 2026-08-11
**Area:** backend, agentctl, protocol, plugins, security

## Context

Kandev plugins can receive events, handle webhooks, use capability-gated Host
APIs, and invoke a selected utility agent, but task agents cannot invoke
plugin-owned operations. The first plugin buildout briefly had a parallel
`tools[]` plus `InvokeTool` path, but it never reached agent tool discovery and
was removed because MCP is already Kandev's agent-tool protocol. Requiring each
plugin to expose a separate MCP server would duplicate installation, transport,
credentials, task context, and lifecycle policy.

## Decision

1. Plugin-contributed agent tools are declared in the installed plugin manifest
   and invoked through the existing supervised plugin gRPC process.
2. Agentctl exposes those declarations as proxy tools on Kandev's existing MCP
   server. It never connects agents directly to a plugin-owned MCP endpoint.
3. The backend owns the complete active plugin-tool catalog and the applicability
   decision. Agentctl receives immutable runtime descriptors and filters them by
   the backend-owned MCP surface; an agent cannot request or register tools.
4. Plugin tool names use the readable provider-safe
   `kandev_<slugged-plugin-id>_<local-name>` namespace. Slugs replace
   punctuation with underscores; names exceeding the provider limit truncate
   the slug and append a short stable hash suffix. Local names contain only
   lowercase alphanumerics and underscores; install and upgrade validation
   reject invalid or colliding final names.
5. The existing atomic `SetTools` registry replacement is the live-update
   mechanism. Catalog changes send `notifications/tools/list_changed` without
   restarting the agent, agentctl, or task session.
6. Catalog snapshots carry a backend-process generation and monotonic revision.
   Agentctl ignores an older revision from the same generation so delayed
   refreshes cannot restore stale tools.
7. Discovery is never authorization. The backend revalidates current plugin
   state, declaration, surface, bound task/session identity, and input schema on
   every invocation before dispatching the plugin RPC.
8. Plugin tool invocation adds no Host capability. Existing manifest capability
   gates remain the authority for data, state, secrets, authentication, and
   utility-agent access.
9. Tool calls are cancelable, bounded, single-attempt operations. Kandev does
   not retry a failed call because it cannot know whether a plugin performed a
   side effect before returning or disconnecting.

## Consequences

- Plugin authors get one installable package and one lifecycle for UI, events,
  Host APIs, and agent tools.
- Task agents can gain and lose plugin tools during a running MCP connection on
  clients that honor `tools/list_changed`; other clients converge on reconnect.
- Backend and agentctl gain a versioned dynamic descriptor contract and a new
  backend dispatch action, while plugin execution continues over the existing
  go-plugin/gRPC channel.
- A stale client may display a removed tool briefly, but backend revalidation
  makes stale discovery fail closed.
- Tool definitions add agent context and installed plugin code remains trusted,
  privileged code. Conservative limits and annotations reduce, but do not
  eliminate, that operational risk.
- The initial version is instance-wide and task-surface-scoped. Finer workspace
  or profile enablement requires a later product contract rather than ad hoc
  filtering in agentctl.

## Alternatives Considered

### Require every plugin to run its own MCP server

Rejected because it duplicates runtime supervision and credentials, depends on
executor network reachability, requires manual profile configuration, and
cannot reliably follow plugin enable/disable or Kandev task identity.

### Restore the original parallel plugin tool HTTP/RPC API

Rejected because agents would need a second discovery and invocation protocol,
and Kandev would have to reproduce MCP schemas, notifications, results, and
client compatibility behavior.

### Let plugins register arbitrary tools directly with agentctl

Rejected because plugin processes do not own task/session authorization or MCP
surface policy. Direct registration also makes stale registrations and name
collisions harder to revoke safely.

### Restart agents whenever the catalog changes

Rejected because mcp-go already supports atomic tool replacement and MCP defines
`tools/list_changed`. Restarting destroys conversational continuity and turns a
plugin lifecycle event into an unrelated agent lifecycle failure.

### Expose one generic `invoke_plugin` MCP tool

Rejected because agents would lose per-operation descriptions, schemas,
annotations, and selective discovery. It would also move validation into prompt
conventions rather than the MCP contract.
