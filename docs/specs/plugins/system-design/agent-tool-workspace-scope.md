---
status: current
system: plugins
created: 2026-11-06
requirements:
  - REQ-PLUGINS-AGENT-TOOL-WORKSPACE-001
---
# Workspace-scoped plugin tools

## Purpose and boundaries

The plugin system owns the `workspace` invocation scope, its catalog
eligibility, and plugin dispatch. This extends
[plugin tools across MCP contexts](agent-tool-contexts.md), which owns the
`task` and `instance` scopes and the trusted external transport marker.
External endpoint authentication remains owned by
[integrations](../../integrations/requirements/external-mcp.md). Workspace
permission decisions remain owned by the task service
(`internal/task/service/service_access.go`).

Source request: [discussion #5102](https://github.com/kdlbs/kandev/discussions/5102).

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.1, .2 | Declarations |
| AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.3 | External catalog |
| AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.4 to .7 | External invocation |
| AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.8 | Task-backed surfaces |
| AC-PLUGINS-AGENT-TOOL-WORKSPACE-001.9 | External invocation; dispatch gate |

## Declarations

Extend `invocation_scope` in `internal/plugins/manifest` to
`task | instance | workspace` and mirror the constant in
`internal/mcp/plugintools`. Omission still normalizes to `task`.

Validation rules for a `workspace` declaration:

- Allowed surfaces are `external`, `kanban-task`, and `office-task`.
  `automation`, `coordinator`, and `managed-conversation` are rejected.
- With `external`, `annotations.read_only_hint` must be `true`, matching the
  instance-only rule.
- The input schema must have top-level `type: object`. A top-level
  `properties.kandev_workspace_id` is rejected so the host-owned argument can
  never collide with a plugin input.

`external` stops being an instance-only surface: it accepts `instance` or
`workspace` reads. `coordinator` stays instance-only. Runtime descriptors
(`plugintools.Definition.InvocationScope`) carry the normalized value. Add
`Definition.IsWorkspaceRead()` beside `IsInstanceRead()`; the MCP server's
external registration filter accepts either.

A plugin that declares `workspace` sets `min_kandev_version` to `0.100.0`,
the first host release that ships this scope, so an older host rejects the
manifest instead of silently narrowing it.

## External catalog

`handleListExternalPluginTools` (`internal/mcp/handlers/plugin_tools.go`)
returns the external catalog. For each workspace definition it returns a copy
whose `InputSchema` adds `kandev_workspace_id` (`type: string`, `minLength: 1`,
a fixed description naming `list_workspaces_kandev`) to `properties` and to
`required`. The plugin's own schema is unchanged in the plugin catalog and is
still the schema used to validate plugin arguments. The schema change happens
in the backend catalog response, so `server.go` registers the definition
unchanged and the fanout/`tools/list_changed` path needs no change.

Clients discover IDs through the existing `list_workspaces_kandev` tool, which
the external surface already exposes and which already filters by caller.

## External invocation

`handleInvokeExternalPluginTool` keeps its existing guards: trusted external
transport marker, no execution context, no task principal, and empty
payload-level `task_id`, `session_id`, and `workspace_id`.

Then, for a workspace tool:

1. Read the descriptor's scope from the live catalog.
2. Remove `kandev_workspace_id` from a copy of `arguments`. Reject when it is
   absent, not a string, or blank.
3. Load the workspace with `taskSvc.GetWorkspace`. This lookup is mandatory
   because `AuthorizeWorkspaceAccess` skips the lookup when authentication is
   disabled.
4. Call `taskSvc.AuthorizeWorkspaceAccess(ctx, id)` (scope
   `ScopeWorkspaceRead`). The context carries the `authn.Identity` set by the
   global auth middleware and enforced by `externalMCPAuthMiddleware`; the
   mcp-go streamable HTTP and SSE handlers derive the tool context from the
   request context. Disabled authentication yields the synthetic identity and
   any existing workspace passes.
5. Call `pluginSvc.InvokeAgentTool` with the stripped arguments and
   `AgentToolInvocationContext{Surface: external, WorkspaceID: id}`.

Steps 3 and 4 return one shared forbidden error ("workspace is not available")
for not-found and denied, so the error does not reveal whether a workspace
exists. Nothing is cached; every call repeats steps 3 and 4, so revoked tokens
and memberships take effect on the next call. Only the workspace ID crosses the
plugin boundary: the existing `AgentToolContext.workspace_id` proto field carries
it. No protobuf or SDK type change is needed.

For an instance tool the arguments pass through unchanged and the context keeps
an empty workspace.

## Dispatch gate

`validateInstanceSurfaceDeclaration` in `internal/plugins/agent_tools.go`
re-checks eligibility under the dispatch lease. For the `external` surface:

- `instance` requires an empty `WorkspaceID`.
- `workspace` requires a non-empty `WorkspaceID` and a true read-only hint.
- `task` is rejected.

The plugin service trusts the in-process handler for the authorization
decision. The gate makes a scope and context mismatch fail closed if a future
caller skips the handler checks.

## Task-backed surfaces

On `kanban-task` and `office-task`, a workspace tool dispatches exactly like a
task tool: the handler resolves task, session, and workspace from the trusted
execution, and the schema is not modified. A retained pre-workspace agentctl
rejects the unknown scope string and discards the entire plugin catalog, not
just the tool. `ProjectLegacySafe` therefore rewrites `workspace` to `task` for
the task surfaces that survive its surface projection. Dispatch on those
surfaces is identical, and the plugin service still re-checks the real manifest
declaration, so the projection cannot widen eligibility. A workspace tool
declared only for `external` is already dropped by the same projection.

## Failure and observability

All failures return before the plugin RPC with the existing MCP error shape and
the existing `plugin tool request denied` log. Add reasons
`workspace_argument_invalid` and `workspace_unavailable`. Log the plugin ID,
local name, and reason. Never log arguments, results, tokens, or the caller's
credentials.

## Related decisions

This is a bounded extension of the explicit-context model in
[agent-tool-contexts](agent-tool-contexts.md) and
[plugin MCP routing](../../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md).
Rejected alternatives: forwarding the caller's PAT to the plugin (credential
exposure), letting plugin schemas declare `workspace_id` (plugin input would
become authority), and filtering the catalog per workspace (the catalog is
instance-wide and authorization is per call). No additional ADR is needed.
