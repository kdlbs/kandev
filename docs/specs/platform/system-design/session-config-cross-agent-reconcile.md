---
status: current
system: platform
created: 2026-11-02
requirements:
  - REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001
---

# Session config reconciliation system design

## Purpose and boundaries

Platform owns reconciliation between saved session configuration and the running
provider's advertised controls. The [requirement](../requirements/session-config-cross-agent-reconcile.md)
owns the observable behavior. Agent identity and sessionless option discovery
remain with Agents. This design does not change provider selection or startup policy.

The selector can use a usable catalog before startup settles. Replay and
persistence need stronger evidence for model-dependent options. The
[Cursor fix plan](../../../plans/cursor-model-option-reconciliation/plan.md)
tracks that work.

## Requirement mapping

All criterion suffixes below refer to
`AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001`.

| Requirement | Criteria | Design sections |
| --- | --- | --- |
| `REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001` | .1, .2, .3, .6, .8, .9 | Backend replay and persistence |
| `REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001` | .4, .7 | Selector availability |
| `REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001` | .5 | Hydration and live updates |

## Components and responsibilities

- `lifecycle/session.go` applies profile and runtime layers at startup and resume.
- `lifecycle/manager_interaction.go` supplies the catalog-aware runtime filter.
- The ACP v1 adapter supplies model and catalog evidence from provider responses.
  The agentctl client carries the evidence to the startup and event paths.
- `orchestrator/event_handlers_streaming.go` persists accepted model snapshots and
  publishes `session.models_updated`.
- `backendapp/helpers.go` and `backendapp/boot_state.go` project replay and boot state.
- `lib/state/slices/session-runtime/model-hydration.ts` reconstructs boot and
  reconciliation state. `session-model-hydration.ts` reconstructs client-fetch state.
- `lib/ws/handlers/session-models.ts` applies live model updates and preserves
  startup, source, and pending-selection rules.
- `components/task/model-selector.tsx` decides whether the advertised controls
  can render. The shared selector serves desktop and phone composers.

## Data and contracts

The session retains `runtime_config`, `runtime_config_overrides`, profile
configuration, and the `acp_model_state` snapshot. Runtime option values remain
a flat map. The model snapshot contains provider models, config options, and
the optional `config_options_settled` marker.

The store uses `configOptionsSettled`. Missing persisted settlement becomes
`false` through both hydration helpers. This conversion remains unchanged.
The marker records completed startup configuration, not permission to display
each already advertised control.

Catalog completeness is internal to agentctl and the backend. It has a separate
meaning from `config_options_settled`, which controls startup publication.
The existing selector event carries its current fields. The session metadata
continues to use the same keys and value shapes.

## Backend replay and persistence

### Provider catalog evidence

Apply this repair to the built-in Cursor ACP v1 binding. A complete catalog
comes from a typed ACP setup response, a `set_config_option` reply, or a
`config_option_update` for the same native session. The catalog identifies the
active model. Its projection accounts for every option ID and has unique,
nonempty IDs. [ACP session config options](https://agentclientprotocol.com/protocol/v1/session-config-options)

Mark completeness at the adapter when the provider payload supplies this
evidence. A partial projection, such as one that loses an option variant,
cannot establish absence. Synthetic convergence reports carry display state.
The complete catalog and its model travel together in the optional internal
agentctl result and on qualified `session_models` reports. Scope the evidence
to the native session and settings generation that produced it.

### Startup replay

`InitializeAndPromptWithLayers` resolves the model through the existing start
policy. Use the complete catalog returned by the correlated model setter, if
available. Require the catalog model to match the confirmed effective model.
When startup uses the provider's current model without a switch, use the
matching catalog from the current setup. If a model setter relies on
notifications, wait within the existing catalog wait budget for a complete
report for the selected model from the same native session, newer than the
pre-switch report. Read that evidence from the agentctl client before the
ordered lifecycle dispatcher. Leave uncertainty about the model to the
existing selection policy and defer options that lack current evidence.

Use the verified list for both saved Cursor option layers. Preserve the order
and values of supported writes and the dedicated model and mode setters.
Populate the startup failure list with errors from attempted runtime setters.
After startup, a late catalog can reconcile persisted options for future use.

### Persistence

Apply catalog reconciliation after the existing attempt, source, and settings
generation checks under the session settings lock. Carry evidence through the
settled startup publication only when it belongs to the published snapshot.
Keep the provider-default baseline separate from that snapshot.

For a qualified Cursor snapshot, write the intersection of the saved map and
the advertised IDs, then add any provider-reported current values to that map.
An empty or missing current value leaves the previous value for that advertised
ID. The value `"false"` is a reported value. Use the same path when the saved
model already equals the reported model, including legacy rows cleaned during
ordinary resume.

A partial report can update an explicitly reported value for its model. Its
omissions preserve saved values. A synthetic copy of another catalog cannot
supply observed option values. Use current metadata for concurrent model-only
writes so they cannot restore keys removed by reconciliation. Record the
catalog generation after the runtime write succeeds. Duplicate events then
converge on the same option map.

The provider-derived map belongs to `runtime_config`. Profiles,
`runtime_config_overrides`, the original effective settings, and the baseline
retain their existing owners. Provider-restored recovery continues to store
observations in `acp_model_state` under its attempt policy.

## Selector availability

The gate distinguishes profile-required keys from keys recorded only in runtime
metadata. `requiredConfigKeys` can retain their union for diagnostics.

For the additional availability rule, a usable advertised catalog has both:

1. At least one raw config option in the session entry.
2. At least one flat model choice, or one usable model-shaped select option.

The second condition is existential. Other unusable or unsupported entries do
not invalidate a usable model choice. Reuse `usableConfigOptions` and
`isModelConfigOption` for model-shaped select options, including category aliases.
Static agent discovery alone does not satisfy this rule.

For each required key, the gate keeps the existing checks first:

- An advertised key is satisfied.
- A flat model list satisfies the existing `model` exception.
- Settled catalogs retain the existing non-agent-key behavior.
- The existing legacy `agent` exception retains its own settlement condition.

The additional rule satisfies an unadvertised key only when all conditions hold:

1. The key comes from `runtime_config` or `runtime_config_overrides`.
2. Neither the profile snapshot nor the matching current profile requires it.
3. The key is not `agent`.
4. The session entry contains a usable advertised catalog as defined above.

This rule applies with missing, false, or true settlement. It does not mutate
the marker, saved values, baseline, confirmed config, or session state.
The universal check still evaluates every required key. One ignored historical
key cannot release a different unresolved profile key.

Empty catalogs and flat-list-only snapshots retain existing readiness behavior.
The normal current-model and control-availability checks still decide whether
there is any selector content to render. The repair adds no loading surface.

## Hydration and live updates

Boot, client fetch, and WebSocket updates feed the same selector gate.
Both hydration helpers preserve the absent marker as false. The live handler
retains its existing omitted-marker, explicit-false, startup, and ownership rules.
The selector displays the controls in those accepted entries without changing
which values the store treats as provider-confirmed.

Model changes continue through `setSessionConfigOption` or `setSessionModel`.
Only explicit user choices call these setters. Stale keys do not appear in the
picker and do not trigger automatic writes. RPC failures retain the existing
toast and optimistic rollback behavior.

Tests cover hydrated snapshots and live catalog changes, including a model that
removes an option used by the preceding model. Existing startup tests continue
to protect saved model labels from intermediate provider defaults.

## Desktop and phone behavior

Both viewports recover the same model and option controls. Desktop retains the
existing model picker. Phone retains the touch picker in the composer toolbar.
The nearest shipped exemplar is `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts`.

The repair changes availability inside existing composition. It does not change
navigation, focus policy, scroll ownership, safe areas, or control sizing.
Phone checks use touch interaction and retain the existing 44px trigger target.
The [plan previews](../../../plans/model-selector-stale-config-availability/plan.md#ascii-ui-preview)
define the affected rows and the rendered checks.

## Failure, security, and observability

An unknown model catalog leaves unverified writes deferred. The existing
startup warning reports rejected settings after a verified write attempt.

Existing `model-selector:gate` diagnostics expose the required keys, option IDs,
and availability result. No new log stream or metric is required.
The missing-marker transition tracked by the earlier selector plan remains
unproven. The Cursor replay failure has a separate, reproducible ordering.

## Related decisions

- [Explicit recovery settings](../../../decisions/2026-09-29-explicit-resume-settings.md)
  retains provider-restored authority and saved selections.
- [Session setting semantics](../../agents/system-design/session-setting-semantics.md)
  defines the distinct confirmation rules for Pi's thinking settings.
- [Original reconciliation plan](../../../plans/session-config-cross-agent-reconcile/plan.md)
  records the prior backend filter and settled-catalog repair.
- [Resume model-label plan](../../../plans/resumed-session-model-labels/plan.md)
  records the existing startup hydration barrier.
