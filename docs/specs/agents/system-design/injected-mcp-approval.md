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
| 001.1, 001.3, 001.8, 001.9, 001.10 | Identity normalization |
| 001.2, 001.4 | Injection provenance |
| 001.5, 001.6 | Permission decision |
| 001.7 | Lifecycle and observability |

All references belong to `REQ-AGENTS-INJECTED-MCP-APPROVAL-001`.

## ACP request path

`server/acp/client.go:forwardPermissionRequest` preserves `ToolCall.Name` and
`ToolCall.Meta` in the internal request. The ACP adapter normalizes the
identity and forwards it to the process manager. The manager applies the
session policy before it creates a pending request.

Codex supplies `default_tools_approval_mode=approve` for the injected server
through its own MCP launch configuration.

## Identity normalization

Keep `ToolName` and `ToolMeta` internal to the permission path.
Public permission snapshots contain display and decision data.

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
Each other provider format requires wire evidence before it can use the injected-server policy.

The upstream [Claude tool formatter](https://github.com/agentclientprotocol/claude-agent-acp/blob/v0.75.1/src/tools.ts)
uses the tool name as the default MCP title.
Its [permission presentation](https://github.com/agentclientprotocol/claude-agent-acp/blob/v0.75.1/src/permissions/presentation.ts)
also supplies a programmatic name.
Implementation fixtures must cover both shapes because installed versions can differ.

### Antigravity CLI

Antigravity CLI 1.5.15 reports `mcp_kandev_<tool>` in typed tool notifications.
Use the selected `AuthPolicy.RuntimeVariant` and registered `antigravity-acp`
identity to scope interpretation of that format to the CLI. Keep the shared
`ParseQualifiedMCPToolName` grammar for double-underscore names. The CLI's
flattened name and matching notification identify the tool call, but they do
not identify which server supplied it.

When `session/request_permission` omits a typed name, correlate a typed tool
notification by both native session ID and tool-call ID. Store notification
identity and the active-call entry while holding the same adapter lock. Keep
conflicting notification names as an ambiguous state; a typed request name
does not override that conflict. The wait for identity uses the same
session-and-call key, so an active call with a reused ID in another session
cannot satisfy it. If a notification arrives without a name, continue waiting
for a named or conflicting update until the correlation window expires.
Free-form titles and tool arguments cannot establish a server identity.

Before resolving `mcp_<server>_<tool>`, the process manager builds a
conservative inventory from Antigravity's documented CLI sources. Read the
global `~/.gemini/config/mcp_config.json` and installed plugin MCP configs
under `~/.gemini/config/plugins`. For workspaces, inspect `mcp_config.json`
and plugin MCP configs under `.agents` and recognized legacy customization
directory names from the work directory through the nearest repository root.
Include each plugin's actual machine name from `plugin.json`
(`.antigravity-plugin/plugin.json` takes precedence; a missing name uses the
plugin folder name), then join it to each server key with `_`, matching the
provider's plugin server namespace rule. This includes disabled plugin
candidates as a safe superset.

Antigravity's `plugins.json` discovery manifests can register plugins from
paths outside these default directories. If a global or effective workspace
manifest is present, keep the inventory incomplete and leave requests in the
ordinary permission flow. Also fail closed on malformed, unreadable, oversized,
symlinked, or special-file sources. Read only regular JSON files up to 1 MiB.
See Google's [MCP configuration](https://antigravity.google/docs/mcp#global-and-workspace-server-configs),
[plugin naming](https://antigravity.google/docs/build-plugins), and
[customization discovery](https://antigravity.google/docs/customization-discovery/)
documentation.

An alias is eligible only when no configured Kandev or native server name can
produce its `mcp_<server>_` prefix, the exact tool exists in the live Core
registry, and `resolveInjectedKandevPermission` verifies the current injected
endpoint. The single underscore can also appear in server names, so a
competing global, workspace, or plugin registration leaves the request in the
ordinary permission flow. A missing Core catalog also fails closed for
Antigravity legacy profiles without a package manifest. Package profiles use
the manifest, live package registry, and exact package endpoint.

Classify native human questions before this lookup. The coordinator and
managed-tool policy checks remain authoritative for their sessions.

## Injection provenance

The process manager requires the internal `InjectedKandevMCP` marker. Instance
construction sets the marker after host injection. The manager checks the
injected HTTP or SSE address for the current helper, including the instance
identity when scoped routes apply. Injection owns the reserved server name.

In package mode, `builtinPermissionAllowed` also checks the manifest, live tool
catalog, and exact endpoint for the selected package. Mixed server lists can
contain unrelated registrations. Only a proven built-in registration is eligible.

## Permission decision

Classify native questions first. Apply the actor's session policy before pending
request allocation. In an ordinary session, blanket approval takes precedence.
For an injected-server approval, prefer an offered `allow_once` option. If none
exists, use an offered `allow_always` option. Return the exact option ID.
If no allow choice exists, follow the
[permission control integrity policy](../requirements/permission-control-integrity.md).

The manager owns option selection in `server/process/manager_permission_policy.go`.
Requests that reach the pending path use the existing identity and resolution contract.

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
