---
status: current
system: agents
requirements:
  - REQ-AGENTS-INJECTED-MCP-APPROVAL-001
---

# Injected MCP approval design

## Purpose and boundaries

The process manager owns the automatic permission decision.
The ACP client preserves provider identity fields, and the adapter normalizes provider-specific names.
The MCP server continues to enforce task and session authorization.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| 001.1, 001.3, 001.8 | Identity normalization |
| 001.2, 001.4 | Injection provenance |
| 001.5, 001.6 | Permission decision |
| 001.7 | Lifecycle and observability |

All references belong to `REQ-AGENTS-INJECTED-MCP-APPROVAL-001`.

## Current path and defect

`internal/agent/mcpconfig/passthrough.go:codexServerArgs` adds
`default_tools_approval_mode=approve` for the `kandev` server.
ACP uses a different path:

1. `server/acp/client.go:forwardPermissionRequest` converts the permission frame.
2. `transport/acp/adapter_permissions.go:handlePermissionRequest` forwards it.
3. `server/process/manager.go:handlePermissionRequest` checks only `AutoApprovePermissions`.
4. Without blanket approval, the manager creates a pending request and waits.

The ACP conversion currently drops the programmatic `ToolCall.Name` and its metadata.
The pinned Go SDK already supports both fields. No SDK upgrade is necessary.
The shared adapter's `AutoApprove` field does not implement the permission decision.

## Identity normalization

Extend the internal `types.PermissionRequest` with optional `ToolName` and `ToolMeta` fields.
Copy `ToolCall.Name` and `ToolCall.Meta` in `forwardPermissionRequest`.
These fields remain internal. Do not expose metadata in public permission snapshots or logs.

Use a strict qualified-name parser for `mcp__kandev__<tool>`.
Because this flattened format uses `__` as its boundary, accept only one
unambiguous separator. Reject a separator next to another underscore and
reject another `__` in the remaining name. This keeps names such as
`mcp__kandev__external__execute` and `mcp__kandev___execute` out of the
automatic path, even when the tool identifier itself could contain `__`.
Require the exact server segment and a nonempty tool identifier with letters,
digits, underscores, or hyphens.
Do not match substrings, descriptions, raw arguments, or the `_kandev` suffix alone.

A supplied programmatic name is authoritative, including a nonmatching name.
Never replace a nonmatching name with a matching display title.
For `claude-acp` frames without a name, use a pure compatibility hook in `dialect_claude.go`.
The hook first reads `_meta.claudeCode.toolName` when present.
Malformed or conflicting metadata produces no eligible identity.
Only when both identity fields are absent can the hook use an exact qualified title with kind `other`.
This fallback is specific to Claude's observed MCP presentation, not a generic ACP rule.
Other provider formats retain the normal prompt path until a tested dialect supports them.

The upstream [Claude tool formatter](https://github.com/agentclientprotocol/claude-agent-acp/blob/v0.75.1/src/tools.ts)
uses the tool name as the default MCP title.
Its [permission presentation](https://github.com/agentclientprotocol/claude-agent-acp/blob/v0.75.1/src/permissions/presentation.ts)
also supplies a programmatic name.
Implementation fixtures must cover both shapes because installed versions can differ.

## Injection provenance

Add an internal `InjectedKandevMCP` boolean with `json:"-"` to `config.InstanceConfig`.
Only `Config.NewInstanceConfig` sets it, immediately after local server injection with a positive instance port.
Do not add it to `InstanceOverrides`, public DTOs, persisted profiles, or incoming JSON.
The marker is false for configurations without host injection.

The permission predicate requires this marker and the exact injected server configuration.
Require a URL for the current instance port at `http://localhost:<port>/mcp` or `/sse`.
Require no stdio command and the corresponding supported HTTP/SSE transport.
The current injector replaces user entries named `kandev`, so those entries cannot retain the reserved slot.
Cover a mixed list containing both the injected server and unrelated servers.
The existence of unrelated entries must not grant them approval or disable the injected entry.

## Permission decision

Keep the existing blanket approval branch first.
Before pending-request allocation, evaluate the injected-server predicate.
Select an offered `allow_once`, then an offered `allow_always`.
Return its exact option ID. Never invent a choice or fall back to a reject option.
If no allow choice exists, continue through the existing pending-request path.
The ACP client's existing empty-options cancellation remains intact.

Use a small helper in `server/process/manager_permission_policy.go`.
Do not reuse `autoApprovePermission` for this branch: that helper falls back to the first option, including rejection.
Keep all existing pending-request identity, cancellation, and exactly-once resolution rules unchanged.

## Lifecycle and observability

The policy has no persistence or migration. Instance construction recreates provenance after restart or resume.
The adapter retains normal tool-call and result events on the automatic path.
Automatic approval does not create a pending permission card on desktop or phone.
Log the policy reason, tool identifier, and selected option kind without arguments, headers, or credentials.
No frontend layout or copy changes are required.

## Security and compatibility

This policy trusts the configured agent's protocol identity, as the existing permission flow does.
It does not defend against a malicious agent process that fabricates protocol frames.
Raw MCP arguments cannot establish identity or injection provenance.
Unknown and malformed names fail closed to the normal permission flow.

Server-wide parity also approves destructive Kandev tools at the provider permission layer.
Task reachability, ownership, explicit user-question answers, and workflow completion rules remain separate enforcement points.
The external permission-resolution specification remains authoritative for requests that become pending.
Its unchanged-auto-approval statement describes that feature's scope, not this proposed policy addition.

## Related decisions

- [Injected server approval policy](../../../decisions/2026-09-16-injected-mcp-approval.md)
- [Live permission authority](../../../decisions/2026-08-11-live-agent-permission-authority.md)
