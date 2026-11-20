---
status: current
system: office
created: 2026-11-02
requirements:
  - REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-001
  - REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-002
  - REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-003
---

# Private automation plugin tools

## Purpose and boundaries

Office owns the saved automation selection and its execution authority.
The plugin system supplies declarations, installation identity, configuration lifecycle, and the existing gRPC invocation path.
The web editor supplies the approval action through its existing Save workflow.

The [decision](../../../decisions/2026-11-02-automation-plugin-tool-selection.md) adds a selection to the fixed automation surface.
It does not use `Context.WithManagedToolPolicy`, which changes the surface to `managed-conversation`.
The existing scheduler, run bindings, task visibility, core tool catalog, and Host capability approval ledger remain unchanged.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-001 | Saved selection, Policy version, Native save boundary |
| REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-002 | Discovery and live updates, Invocation, Recovery and failure |
| REQ-OFFICE-AUTOMATION-PLUGIN-TOOLS-003 | Editor and mobile behavior |

## Saved selection

Add one `plugin_tools` JSON column to `automations`, with `[]` as its default.
Represent each plugin selection as:

```json
{
  "plugin_id": "example-plugin",
  "installation_id": "host-owned-installation-id",
  "policy_version": "host-owned-version",
  "tool_names": ["list_findings", "complete_report"]
}
```

Define the shared selection type in `internal/mcp/plugintools` to avoid an automation/plugin import cycle.
`Automation`, native create/update payloads, store projections, and web types carry the list.
Use pointer or explicit presence handling for updates: omitted preserves, `[]` clears, and `null` is invalid.
Sort and deduplicate plugin entries and tool names.
Reject unknown names and selections outside `TaskModeAutomationRun`.
Changing away from that mode clears the stored selection.

The automation row supplies the automation and workspace binding.
Do not add a grants table or copy authority into task, session, run, or retry metadata.
Old rows migrate to `[]` on both supported SQL dialects.
Malformed stored selections deny plugin access.
Portable export omits the field, installation identities, and policy versions.
Import and non-native creation therefore retain the empty default.

## Policy version

Add `AgentToolPolicyVersion string` to `plugins/store.Record`.
Use a host-generated opaque version value, such as a UUID.
Create it for new installations and migrate missing values during `Registry.Load`.
Persist the migration before tools become eligible.

Every admitted installation replacement gets a new value, including replacement with the same package version.
`Service.UpdateConfig` replaces the value after validation and before vault or configuration writes.
Persist and publish the changed record under the existing lifecycle and dispatch write locks.
If that persistence fails, abort before changing configuration.
If a later operation fails, the new policy version can remain and require review.
Never restore an old value while changed configuration remains effective.
Preserve the value on process restart, backend restart, and status-only disable/enable operations.

All configuration saves invalidate approval, including masked-secret round trips and secret-only updates.
This avoids secret hashing and configuration comparison rules.
The policy version contains no configuration data.
Use the same initialization rule for sideloaded records in `service_sync.go`.

## Native save boundary

Reuse the existing workspace authorization and `automation.create` / `automation.update` native handlers.
Only those handlers can mark a selection edit as native configuration intent.
Use an unexported intent value, never a JSON flag or caller-supplied actor label.
Reject in-session MCP context before marking that intent.
MCP configuration creation and plugin Host schedule commands cannot mark it or renew a selection.
Unrelated non-native updates can preserve the existing field.

The native options action is `automation.plugin_tool_options`, scoped to its supplied workspace through existing authorization.
It returns active, configured plugins with tools that declare `automation`.
Each group includes its display name, installed version, installation ID, policy version, and tool descriptions/annotations.
The editor submits the versions it actually displayed.
The service checks those versions against the installed records before saving.
A stale editor receives a review-required validation error and preserves its draft.
The service must not replace stale values with current values automatically.

Expose plugin support through a narrow interface wired in `internal/backendapp`.
Use the existing `AgentToolCatalogListener` notification path after selection changes commit.
No new approval endpoint or service is needed.

## Discovery and live updates

Add `automation` to the tool-surface constants and manifest/snapshot validation.
An existing plugin declaration does not opt in automatically.

In `handleListPluginTools`, derive the automation surface from `mcpscope.PrincipalFromContext`.
Load its automation through `GetAutomationForDispatch` and check the workspace and hidden task mode.
Intersect the current selection with active/configured declarations and current installation/policy values.
An invalid group contributes no tools and grants no access to another group.
Reject a non-automation caller that requests the automation surface.

`registerTools` continues to register the fixed base catalog.
It also registers the backend-filtered plugin tools for automation sessions.
Do not add core tool checkboxes or change other surfaces.

The existing refresher pushes one global catalog through `SetPluginToolsForAllExecutions`.
That global snapshot is only an invalidation notice for an automation session.
It must never install its candidate tools directly into that session.
Clear the cached automation plugin entries and notify clients through the existing list-change mechanism.
The existing before-list hook then requests the session-filtered catalog from the backend.
If the request fails, retain the core catalog and expose no automation plugin tools.

Keep the filtered backend response distinct from the global push inside the MCP server.
Serialize the automation fetch/apply path to prevent delayed responses from replacing later responses.
Allow changed filtered membership at the same global catalog revision.
A selection change can alter membership without changing any plugin declaration.
This avoids a new per-session revision store or profile policy snapshot.

For ordinary global pushes, remove automation-only definitions and automation surface tags.
For ordinary list responses, project declarations onto the requested supported task surface.
This keeps older agentctl validators from rejecting the entire catalog after a plugin adds an automation declaration.
Automation discovery itself requires the updated agentctl.

## Invocation

Permit the plugin list/invoke actions through `automationSurfaceActions` only to reach the dedicated authorization path.
Do not permit generic plugin actions or other configuration actions.
Bind task, session, workspace, and surface from the trusted execution and principal.
Reject conflicting request identities and forged surfaces.
Plugin-owned resource IDs remain subject to the plugin's workspace checks.

Before every call, load the current automation selection.
Attach the matching typed selection to `AgentToolInvocationContext` as backend-owned data.
For surface `automation`, `Service.InvokeAgentTool` requires that data.
Under its existing dispatch read lease, check installation ID, policy version, declaration, configuration readiness, and exact local name.
Then use the existing argument validation, bounded gRPC call, result validation, and invocation logging.
The dispatcher must not hold an automation configuration lock across plugin code.
Existing Host callbacks retain their own capability checks.

Admission occurs at the current selection and plugin-policy check.
An already admitted call can finish while a selection removal commits.
Calls admitted after removal fail.
Plugin lifecycle/configuration write locks wait for the existing dispatch read lease.
Caller cancellation ends only that invocation through the current context path.
No automatic tool retries or shared waiters are introduced.

Core administrative tools remain absent and unavailable through raw MCP actions.
A tool declaration or read-only annotation adds no Host API capability.
Installed plugin code remains trusted under the existing plugin model.
This feature does not sandbox arbitrary plugin behavior.

## Editor and mobile behavior

Add a small `AutomationPluginTools` section to the existing private automation configuration form.
Use a domain hook and API helper for options, rather than component-level requests.
Group labelled checkboxes by plugin and show readable tool descriptions and read/write indications.
Keep the section expanded and reuse `useSettingsSaveContributor` for Save, Discard, and navigation handling.
Do not add a second approval dialog or page.

Valid saved selections hydrate checked rows.
For stale selections, show previous tool names and Review required.
Show current options unchecked until the person explicitly selects them.
Allow a saved group to be cleared even when its plugin is unavailable.
Track selection edits separately from other form fields.
Omit `plugin_tools` on unrelated saves, including saves while options are loading or unavailable.
Preserve in-flight edits when an earlier save response arrives.

The phone entry point remains Settings > workspace > Automations > editor.
Use the current editor's single vertical scroll owner and inline grouped rows.
The nearest shipped precedent is `ConfigSection` and the phone automation editor in `settings/mobile-automations-settings.spec.ts`.
This is a short configuration list, so an additional picker drawer adds no value.
Make the complete checkbox label row touchable with a minimum 44-pixel target on phones and coarse pointers.
Keep ordinary desktop actions at the existing 28-pixel size.
Wrap descriptions and long plugin names without document horizontal overflow.
Preserve keyboard selection, accessible names, and the existing safe-area behavior of the editor.
All copy uses `t()` and all supported catalogs, including pseudo.

The plan contains the labelled [UI previews](../../../plans/automation-plugin-tools/plan.md#ascii-ui-preview).

## Recovery and failure

- Every tools/list and invocation reads current saved authority. Recovery needs no extra policy metadata or scheduler integration.
- A removed automation, foreign workspace, corrupt selection, missing policy version, or failed authority read denies plugin access.
- Disabled or unconfigured plugins expose no tools. Re-enabling unchanged code/configuration can restore still-valid selections.
- Plugin changes leave the old selection visible for review. Reverting configuration does not revive the old policy version.
- Existing runs keep their normal history and completion behavior. A stale direct call returns a review-required tool error.
- Notify catalog listeners after invalidation, including invalidation followed by a failed configuration write.
- Add automation/workspace identity and a stable denial reason to existing structured invocation logs. Do not log arguments, configuration, or secrets.

## Compatibility

| Consumer | Behavior and evidence |
| --- | --- |
| Updated Kandev MCP clients over existing transports | Filtered discovery and typed invocation. MCP and fixture integration tests cover the shared server path. |
| Clients without live list-change support | Converge on the next list request or reconnect. Backend checks still reject stale calls. |
| Older retained agentctl | Retains core automation behavior. New automation plugin support requires an execution upgrade/restart. Ordinary catalogs remain compatible. |
| Existing plugins | No automatic opt-in. Existing task and managed-conversation permissions remain unchanged. |
| Kandev Security | Its separate repository must add `automation` to intended tool declarations and the handler's accepted surfaces. |

Kandev Security currently rejects other surfaces in [its handler](https://github.com/meanderix/kandev-security/blob/36816abe43af93ba28de5e48ab8278ee1a750db7/server/agent_tools.go#L43).
Its hidden-run task/session/workspace context is already sufficient for report ownership.
Document the small author update, but prove core behavior with `cmd/plugin-fixture`.
No production plugin is added to this monorepo.

## Related contracts

- [Fixed automation authority](../../../decisions/2026-08-22-user-configured-automation-continuity.md).
- [Plugin tools through Kandev MCP](../../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md).
- [Plugin agent tools](../../plugins/requirements/agent-tools.md).
- [Configuration readiness](../../plugins/system-design/agent-tool-config-readiness.md).
- [Implementation plan](../../../plans/automation-plugin-tools/plan.md).
