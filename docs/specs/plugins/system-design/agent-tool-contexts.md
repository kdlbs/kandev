---
status: current
system: plugins
created: 2026-11-03
requirements:
  - REQ-PLUGINS-AGENT-TOOLS-001
  - REQ-PLUGINS-AGENT-TOOLS-002
---
# Plugin tools across MCP contexts

## Purpose and boundaries

The plugin system owns declarations, catalog eligibility, and plugin dispatch.
This extends [agent tools](../requirements/agent-tools.md).
Automation approvals remain owned by [Office](../../office/system-design/automation-plugin-tools.md).
Coordinator identity and core permissions remain owned by
[coordinators](../../coordinator/system-design/copilot-tools.md).
External transport authentication remains owned by
[integrations](../../integrations/requirements/external-mcp.md).

This extension ships in host 0.99.0. Phase-1 coordinators retain their fixed
core catalog.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLUGINS-AGENT-TOOLS-001 | Declarations; lifecycle |
| REQ-PLUGINS-AGENT-TOOLS-002 | Declarations; authorization; provider usage; verification |

## Declarations

Add optional `invocation_scope: task | instance` to each agent-tool declaration.
Omission means `task`, preserving existing declarations.
Copy the normalized field through manifest validation and runtime descriptors.
Accept `coordinator` and `external` as explicit surfaces.
These two surfaces require `invocation_scope: instance`.
Require a true read-only annotation for these two surfaces as an eligibility
check; annotations alone never authorize discovery or dispatch.

An instance declaration says the operation does not require task identity.
It does not grant Host capabilities or a workspace identity.
Keep configuration tools excluded. Existing task, managed, and automation
declarations keep their respective authorization rules.

These contexts first ship in host release 0.99.0. A plugin that declares
`coordinator` or `external` sets `min_kandev_version: "0.99.0"` or later so an
older host cannot install a declaration it would reject or silently narrow.

## Authorization

Automation uses the existing saved exact-tool selection, installation, policy
version, workspace, and genuine execution checks.
Do not auto-grant the usage tool to existing automations.

Extend the phase-2 coordinator binding with exact plugin ID/local-name references
and installation/policy-version values, separate from core `ToolNames`.
Create a version-2 binding at coordinator conversation creation or explicit reopen.
Its plugin list is the eligible active, configured, instance-read catalog whose
declarations explicitly include `coordinator`.
Keep version-1 bindings readable with an empty plugin list.
Never treat an absent binding as a wildcard plugin grant.
Revalidate the live coordinator, current conversation task, workspace, binding,
and current installation/policy version for every call. Carry the exact pinned
reference into the plugin dispatch boundary and compare it with the record under
the same dispatch lease that covers the lookup and RPC, so a config save or
replacement in the handler-to-dispatch gap still fails closed.
New or changed plugins require a new binding; removed tools fail immediately.

Allow only plugin list/invoke actions through the coordinator action guard.
Authorize each descriptor or invocation against the exact plugin binding before
generic dispatch. Do not add arbitrary plugin names to the core tool universe
or let generic plugin actions bypass the coordinator policy.
Phase-1 and malformed bindings expose no plugin tools.

External listing and invocation require the in-process trusted external
transport marker established by `NewExternalDispatcherBackendClient`.
Preserve the endpoint's PAT requirement when authentication is enabled and its
existing behavior when authentication is disabled.
Reject caller-supplied task, session, workspace, execution, or surface identity.
Derive surface `external` from trusted transport context.
Leave task, session, execution, and workspace empty for these instance calls.
Do not create synthetic tasks or sessions.
[Workspace-scoped plugin tools](agent-tool-workspace-scope.md) adds the
`workspace` scope, whose external calls carry a host-verified workspace.

Task-backed calls keep host-resolved task/session/workspace identity, including
automation and coordinator calls to instance tools.
Derive each execution's surface from trusted execution metadata or principal;
a payload surface cannot upgrade a task call to an external call.

## Dispatch and lifecycle

Keep active/configured checks, schema validation, dispatch leases, cancellation,
30-second deadline, 1 MiB result limit, no retries, and output validation.
Check scope and profile authorization again at dispatch, after stale discovery.
A plugin declaration is installed privileged code, not a sandbox guarantee.

Reuse catalog generations, revision ordering, atomic replacement, and
`tools/list_changed`. External clients pull an eligible catalog before listing
and the backend external server participates in the lifecycle catalog fanout, so
an initialized client is refreshed without another unsolicited list. Coordinator
clients intersect snapshots with their bound references.
Old agentctl builds receive legacy-safe snapshots and cannot invoke new contexts.
No protobuf field is necessary: existing context strings can be empty.

## Provider usage

The provider plugin adds instance scope and the automation, coordinator, and
external surfaces alongside its existing task surfaces.
Its local name, canonical MCP name, description, empty arguments, and output
schema remain unchanged.
The SDK import path need not change for a manifest-only extension.
Record the first supporting host release in compatibility documentation; do not
claim support from a new plugin package on an older host.

`get_provider_usage` returns the poller's in-memory, instance-wide report.
It never refreshes telemetry, launches CodexBar, or reads credentials on demand.
Consumers inspect `partial`, `stale`, `availability_state`, and individual
window `utilization_percentage` / `remaining_percentage` before choosing a
provider. A cached percentage cannot guarantee admission to a future request.
Keep scoped windows distinct from the provider-wide quota.
For external instance calls such as `get_provider_usage`,
`scope.invocation_workspace_id` is the empty string. External workspace reads
carry the host-verified workspace ID described by
[workspace-scoped plugin tools](agent-tool-workspace-scope.md).

## Verification

The [implementation package](../../../plans/provider-usage-contexts/plan.md)
maps criteria to focused host, plugin, and transport tests.
Exercise real MCP initialize/list/call paths against a disposable host with
synthetic telemetry. This checks routing without claiming real provider access.

## Failure and observability

A missing grant, invalid binding, forged identity, stale approval, disabled
plugin, or unsupported context fails before the plugin RPC.
Return the existing MCP error shape and record invocation metadata and outcome.
Never log tool arguments, results, credentials, or plugin configuration.

## Related decisions

This extends the task-only boundary in
[plugin MCP routing](../../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md)
while preserving [automation approval](../../../decisions/2026-11-02-automation-plugin-tool-selection.md).
The explicit context declaration and separate coordinator binding preserve the
rationale here; no additional ADR is needed for this bounded extension.
