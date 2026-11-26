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
persistence need evidence that the options belong to the active model. The
[model change and resume fix](../../../plans/cursor-model-option-reconciliation/plan.md)
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
- Transport adapters supply model and catalog evidence from provider reports.
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

The adapter confirms that a report belongs to an active model, native session,
and settings generation. It marks the catalog complete when the protocol
defines the list as complete and the projection preserves every option ID.
The projection requires unique, nonempty IDs. The model comes from the report
or the same provider response's model state. A requested model ID alone gives
no catalog evidence.

ACP v1 and v2 can supply this evidence in typed setup, `set_config_option`
replies, and `config_option_update` notifications. A model-shaped row must
agree with the reported model. A projection with unsupported variants can
verify the entries it preserves. Deletion requires the complete set of IDs.
[ACP v1 session options](https://agentclientprotocol.com/protocol/v1/session-config-options)
[ACP v2 session options](https://agentclientprotocol.com/protocol/v2/session-config-options)

Legacy transports can verify individual options from provider evidence. Grok
can verify a reasoning value accepted by the model setter or supplied by the
target model's metadata. A fallback used for display supplies no value
evidence. Mark only verified entries when the list also contains cached options.

Carry verified entries, completeness, and display-only provenance through the
agentctl setup model state and `session_models` event. Carry correlated setter
evidence in the configuration result. Missing evidence from older helpers is
unknown. The replay and persistence consumers use this contract for every agent.

### Startup replay

`InitializeAndPromptWithLayers` resolves the effective model through the start
policy. Use verified options from the correlated model setter or a newer
provider report for that model and native session. Read the agentctl client
before the ordered lifecycle dispatcher. Use the existing bounded wait when
saved settings need evidence from a delayed report. Reject evidence superseded
by a report of another active model or session. When startup keeps its model,
use the current setup report or wait within the same budget for a report that
supplies the options. A synchronous snapshot of copied display options can
supply only independently verified entries.

Filter both saved option layers by the verified IDs. Defer unverified options
when evidence is missing. Keep the order and values of supported writes, the
dedicated model and mode setters, and the provider-restored recovery policy.
Record errors from attempted runtime setters in the startup failure list.
After startup, a late catalog can reconcile saved options for future use.

### Persistence

Apply catalog reconciliation after the existing attempt, source, and settings
generation checks under the session settings lock. Carry evidence through the
settled startup publication only when it belongs to the published snapshot.
Keep the provider-default baseline separate from that snapshot.

For a complete active-model snapshot, write the intersection of the saved map
and the advertised IDs, then add provider-reported current values to that map.
An empty or missing current value leaves the previous value for that advertised
ID. The value `"false"` is a reported value. Use the same path when the saved
model already equals the reported model, including legacy rows cleaned during
ordinary resume.

A partial report can update an explicitly observed value for its model, even
when the same event contains copied display options. Only verified entries
from that report update the saved map. Omissions preserve saved values. Use
current metadata for concurrent model-only writes so they preserve the
reconciled map. Record the generation of a sequenced event after the runtime
write succeeds. Redelivery can retry a failed write at the same generation.

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
