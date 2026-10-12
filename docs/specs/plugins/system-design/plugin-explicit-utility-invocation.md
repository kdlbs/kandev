---
status: current
system: plugins
requirements:
  - REQ-PLUGINS-EXPLICIT-UTILITY-001
created: 2026-09-14
owners:
  - kandev
---

# Explicit Plugin Utility Invocation System Design

## Purpose and boundaries

This document defines the replacement for the configuration-driven routing in PR #2870 and its implementation boundary.
The plugin Host boundary owns dispatch. The user settings service owns the default. The plugin owns its saved preference.
The prior [design](plugin-direct-profile-invocation.md) is superseded.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| .1, .2, .3, .8 | Resolution and ownership |
| .4, .5, .6, .7 | Errors and permissions |
| .9 | Plugin configuration lifecycle |
| .10, .11 | SDK and wire compatibility |
| .12 | Execution boundary |

All suffixes refer to REQ-PLUGINS-EXPLICIT-UTILITY-001 and its acceptance criteria.

## SDK and wire compatibility

Use this public Go shape. Names are prescribed for this package.

```go
type UtilityAgentOptions struct {
    ProfileID string
}

InvokeUtilityAgent(ctx context.Context, prompt string, options ...UtilityAgentOptions) (string, error)
```

Zero option values and one value with an empty ProfileID select the default. One non-empty ProfileID selects an override.
Reject more than one options value with InvalidArgument. Do not implement last-value-wins behavior or normalize non-empty IDs to empty.
An existing two-argument call still compiles. Implementations of the Host interface and test doubles must adopt the new signature.
Keep prompt and text response behavior unchanged.

Add the wire RPC `InvokeUtilityAgentWithOptions` with a new `InvokeUtilityAgentWithOptionsRequest` message.
Use `string prompt = 1` and `string profile_id = 2`. Reuse `InvokeUtilityAgentResponse`.
Every call from the revised SDK uses this RPC, including calls without options.
An older host returns Unimplemented; the SDK must not retry with the old RPC.
Adding only a field to the existing request is unsafe: an old host can ignore it and execute its configured profile.
The public SDK still has one method named InvokeUtilityAgent; the separate RPC establishes transport compatibility, not a second product API.

Retain the old prompt-only RPC for existing binaries. On the revised host it delegates to the new implementation without an override.
It uses the platform default even if the plugin has saved agent_profile, utility_agent, or legacy transition metadata.
This preserves wire availability but intentionally changes old configuration-based semantics. Document that change explicitly.
Use the existing proto generation target; never hand-edit generated Go files.

## Resolution and ownership

1. Check agent_invoke before any dependency lookup with side effects.
2. Take ProfileID from the invocation options.
3. If it is empty, call a narrow default-profile source backed by userservice.Service.GetDefaultUtilityAgentProfileID(ctx).
4. Validate the chosen ID with the existing profilebinding resolver through pluginsAgentProfileAdapter.
5. Call pluginsHostUtilityAdapter.ExecuteProfilePrompt with that ID and the unchanged prompt.
6. Return the response text or the classified error.

Do not read plugin configuration, config_schema, utility-agent catalog records, or legacy fallback markers in this flow.
Do not read the default source at all for an explicit override. Read the current default once for each default invocation.
Use the existing settings service identity rules and request context; do not invent a plugin-specific default or a browser-user cache.
A later default change affects the next call. The hostutility manager resolves the selected profile before dispatch and revalidates eligibility.

Keep the narrow interfaces in internal/plugins. Wire the default source, existing profile source, and runner in backendapp/main.go and services.go.
Preserve the mutex-protected late dependency accessor in Service: boot-active plugin hosts must see dependencies wired after startup.
Do not import internal/agent/runtime or the user service implementation into internal/plugins.

## Errors and permissions

PermissionDenied precedes settings and profile access. More than one options value returns InvalidArgument without execution.
An empty default or missing/ineligible selected profile returns FailedPrecondition. Do not substitute any other profile.
An explicit valid override works even when the default is unset or its storage read would fail.
Preserve cancellation, deadlines, storage failures, and runner failures as operational errors through both gRPC adapters.
The default getter must propagate storage failures. It must not convert them into an empty default.
The runner retains the existing final profile eligibility check. No retry changes execution identity.

## Plugin configuration lifecycle

Keep the generic format: agent-profile picker and its Save/Discard flow. The field is ordinary plugin data.
It must work regardless of the property name used by a plugin. A manifest selector is not an execution grant.
A plugin that supports the default makes its selection optional. An absent or empty preference means no override.
For Notes, read agent_profile through Host.GetConfig, validate its type, and pass it as UtilityAgentOptions.ProfileID.
A configuration read or type error must stop the request, not silently use the default.
The existing plugin restart after Save provides refreshed configuration. Reading GetConfig on each invocation is the simplest implementation.
Keep the existing unavailable-selection display and the profile hydration fixes from this PR.
No new host UI controls or copy are required for the monorepo change.

## Persistence and legacy removal

There is no new database, default setting, or plugin configuration key owned by the host invocation API.
Remove the invocation-only UtilityAgent source and backend adapter once no caller needs them.
Remove LegacyUtilityAgentFallback production routing, installation stamping, host capture, and hidden-value preservation introduced for this PR.
Old serialized marker fields must not cause a store reload failure. Ignore them without restoring routing authority.
Do not delete arbitrary user configuration files or migrate stored utility IDs into profile IDs.
Retain generic utility-agent schema rendering for other consumers. Remove only obsolete invocation-specific helpers and tests.
Inspect existing schemas and serialized fixtures before removing fields so the normal store decoder remains compatible.

## Execution boundary

Reuse hostutility.Manager.ExecuteProfilePrompt. It owns the existing inference runtime and its working directory.
Profile settings still control the model and execution configuration. The plugin supplies neither cwd nor a task/session identifier.
General profile execution is a separate future API design. Do not add speculative fields for it here.

## Delivery and diagnostics

Update SDK comments, public authoring examples, manifest reference, and protocol documentation together with implementation.
Explain default, override, invalid override, and old-host rejection. Do not log prompts or complete plugin configuration.
Tests must distinguish selected profiles through runner arguments or test-only execution evidence, not only echoed request values.
The dedicated Notes repository is a separate delivery dependency. Host E2E coverage must use the in-tree fixture and must not depend on Notes installation.

## Related decisions

- [Explicit invocation decision](../../../decisions/2026-09-14-explicit-plugin-utility-selection.md)
- [Host utility execution tier](../../../decisions/0002-host-utility-agentctl-for-sessionless-flows.md)

## Implementation plans

- [Replacement work package](../../../plans/plugin-explicit-utility-invocation/plan.md)
