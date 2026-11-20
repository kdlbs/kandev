# ADR-2026-11-16-instance-scoped-builtin-mcp-routes: Scope native MCP routes by helper identity

**Status:** accepted
**Date:** 2026-11-16
**Area:** protocol

## Context

Each agentctl listener binds built-in MCP calls to one task session. Bare
routes identify only a port. Port reuse and hard-coded URLs caused wrong-task
clarifications on 2026-11-02 and 2026-11-06.

[PR #5294](https://github.com/kdlbs/kandev/pull/5294), investigated by João Manso
(`JnManso`), proposes a per-instance bearer credential. URL path scoping covers
the same accidental cross-task threat without provider-specific header support.
Implementation and release validation remain pending.

## Decision

An opted-in instance serves built-in MCP only under `/i/<instance-id>`.
The identifier is the helper's identity, including for package routes and SSE
message advertisements. Wrong-instance and bare paths return 404 before dispatch.
One shared builder supplies every native endpoint and approval comparison.

The backend requests `scoped_mcp_routes: true` on new managed instances.
The helper echoes its effective mode in the create response and reports it in
`InstanceInfo` for adoption. Omission means legacy routing. This handles both
directions of version skew without a runtime flag or a new capability probe. A scoped 404 never
authorizes fallback to a bare path.

The prefix uses the accepted helper ID, which is unique per execution.
A replacement for the same execution can reuse that ID because it still serves
the same task session. Staged delivery uses the staged client's ID and mode.
Existing identity allocation, metadata, ownership, and durable-delivery contracts
remain unchanged.

## Consequences

Stale URLs and bare-port guesses fail before they can change another task.
Every MCP client already accepts an endpoint URL, so no provider header-support
matrix or experimental toggle is required. Installed-provider smoke checks still
verify the existing delivery mechanisms.

Old helpers and instances created by old backends retain bare routing and lack
this protection. Retained legacy agents migrate only on normal replacement.
Instance IDs are not secrets. A process that copies another full URL can still
reach that helper, subject to existing task and operation authorization.

The implementation must update all URL consumers together, including exact-URL
approval, native configuration parsers, helper-local snapshots, and recovery.
SSE must advertise a usable scoped message URL. The external backend MCP API
keeps its existing routes and authentication.

## Alternatives considered

- **Per-instance bearer credential:** Depends on each provider honoring MCP
  headers. The credential appears in arguments or workspace files and covers
  the same accidental-cross-target threat. The extra distribution and rollout
  machinery does not justify that dependency.
- **ACP session matching:** MCP configuration arrives before the ACP session
  identity exists. MCP requests do not carry that identity. TUI agents have none.
- **Peer process lineage:** Requires platform-specific TCP-to-process inspection
  and fails across container and remote network boundaries.
- **Capability-only activation:** Protects a new backend with an old helper.
  An old backend still emits bare URLs for a new helper.
- **Always reject bare paths in new helpers:** Breaks old-backend passthrough
  and project-file delivery. Per-instance negotiation provides the same default
  protection for current pairs while preserving those deployments.
- **Dual bare and scoped aliases:** Leaves stale bare-port calls effective on
  protected instances and defeats the required isolation.

## Related records

- [Requirements](../specs/agents/requirements/builtin-mcp-instance-scoped-routes.md)
- [System design](../specs/agents/system-design/builtin-mcp-instance-scoped-routes.md)
- [Per-CLI injection](0014-passthrough-mcp-injection-strategies.md)
- [Package authority](2026-10-22-builtin-mcp-tool-policy.md)

Contributor investigation credit remains due to João Manso (`JnManso`). This
record does not authorize updates to the contributor's fork or decide which PR
will carry the implementation.
