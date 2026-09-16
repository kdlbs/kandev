---
status: superseded
system: plugins
requirements:
  - REQ-PLUGINS-DIRECT-PROFILE-INVOCATION-001
created: 2026-09-14
owners:
  - kandev
---
# Plugin Direct Agent Profile Invocation System Design

> Superseded by [explicit invocation design](plugin-explicit-utility-invocation.md).
> Retained as historical context for PR #2870.

## Purpose and boundaries

The plugin system owns the `Host.InvokeUtilityAgent` contract and the
plugin-configuration state that selects its execution identity. This design
adds a direct agent-profile selection path beside the legacy utility-agent
selection, without changing the public RPC, SDK, or capability gate. The
agents system continues to own profile eligibility and the utility execution
resolution described by
[Profile-backed Utility Agents](../../agents/system-design/utility-agent-profiles.md).
The UI system's shared settings-save coordinator and hydration contracts
remain independent dependencies, as does
[ADR 0048](../../../decisions/0048-plugin-host-utility-agent-invoke.md) for
the invocation precedence rule.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLUGINS-DIRECT-PROFILE-INVOCATION-001` | [Configuration selection](#configuration-selection), [Host execution routing](#host-execution-routing), [Eligibility and error translation](#eligibility-and-error-translation), [Legacy compatibility](#legacy-compatibility) |

## Configuration selection

`apps/web/lib/plugins/config-schema.ts` adds the `agent-profile` renderer
format beside the existing `utility-agent` format. Settings > Plugins renders
the field through the shared utility-agent profile picker component with
`agentRegistration` data, which exposes each platform profile's name, label,
workspace scope, CLI passthrough, and `inference_capable` flag.

Eligibility filtering is `utility-execution` scoped: a profile is selectable
only when it is enabled, non-deleted, global (not workspace- or
Office-scoped), not CLI passthrough, and its agent declares
`inference_capable === true`. The picker does not fall back to matching
platform profiles against utility or custom-prompt catalog rows: rows from
`/api/v1/utility/agents` are never shown in the `agent-profile` field.
Unknown or ineligible saved IDs render through the picker's unavailable
option so the stored value stays visible-and-replaceable.

File-field hydration (`plugin-config-form.tsx`) merges its backend profile
response with the current store snapshot instead of replacing the record, so
plugin config hydration and independent profile-list updates do not clobber
each other. Store-side profile-list hydration in
`use-settings-data.ts`/`hydrator.ts` compares an incoming profile snapshot
against what is already loaded and does not overwrite a higher store
revision; a WebSocket profile update triggers a retry when the list render
would otherwise go stale.

## Host execution routing

`internal/plugins/host_utility.go` resolves the invocation identity at the
start of each call, before any execution:

1. The `agent_invoke` capability gate runs first; a plugin without it is
   denied before configuration or profile lookup.
2. If the manifest's `config_schema` declares a field with
   `format: agent-profile` and the stored value is non-empty, that value is
   the direct profile ID. The host validates the declared format first, then
   resolves the profile through the agent registry: missing or ineligible
   profiles return typed `FailedPrecondition` before any runner starts, so
   configuration failure and operational failure remain distinct classes.
3. With a resolved eligible profile, the host calls
   `ExecuteProfilePrompt(profileID, prompt)` directly on the host-utility
   tier. No utility or custom-prompt definition is looked up or executed in
   this path.
4. Legacy `utility_agent` routing is consulted only when no direct profile is
   selected, the active manifest declares the `utility-agent` format, and the
   stored legacy value passes the existing trusted-resolution checks. Its
   behavior is otherwise unchanged.

`internal/backendapp/services.go` adapts the host-utility manager's typed
revalidation errors to gRPC codes: not-found/missing selection and
ineligibility surface as `FailedPrecondition`; store failures other than
not-found are returned unchanged so they remain server-side errors rather
than misleading the plugin's configuration messaging.

## Eligibility and error translation

The Notes webhook and other plugin callers translate `FailedPrecondition`
into an actionable "select an agent profile" setup message (HTTP 412) and
leave other codes (for example `Internal`) as execution failures (HTTP 502).
Service wiring in `services.go` builds the agent-profile provider from the
same registration store used by the settings API, so the picker and the
execution path judge eligibility with one source of truth. Renaming or
reflagging a profile revalidates at the next invocation; an in-flight call
keeps the configuration resolved when it started.

## Legacy compatibility

Manifests that keep only `format: utility-agent` continue to render the
utility-agent picker and persist utility prompt IDs; their stored values
resolve and execute exactly as before this change. A plugin updating its
manifest from `utility-agent` to `agent-profile` keeps its legacy value
until the operator saves a direct selection, and
`docs/decisions/0048-plugin-host-utility-agent-invoke.md` documents the
verified-transition rule that bounds the legacy fallback to trusted
pre-transition configurations. No config migration runs; both selector
formats coexist in stored plugin configuration.
