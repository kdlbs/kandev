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

The existing implementation filters backend replay and supports settled catalogs.
The proposed selector correction also handles usable catalogs without settlement.
Implementation status belongs to the [delivery plan](../../../plans/model-selector-stale-config-availability/plan.md).

## Requirement mapping

All criterion suffixes below refer to
`AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001`.

| Requirement | Criteria | Design sections |
| --- | --- | --- |
| `REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001` | .1, .2, .3, .6, .8 | Backend replay and persistence |
| `REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001` | .4, .7 | Selector availability |
| `REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001` | .5 | Hydration and live updates |

## Components and responsibilities

- `lifecycle/session.go` applies profile and runtime layers at startup and resume.
- `lifecycle/manager_interaction.go` supplies the catalog-aware runtime filter.
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

No new field, endpoint, event, database migration, or cleanup write is required.
The repair does not infer provider confirmation from a populated model list.

## Backend replay and persistence

`applyRuntimeSessionLayers` captures the running catalog after model selection.
It sends only supported restorable runtime options through `SetConfigOption`.
An unknown catalog causes runtime replay to send no unverified options.
An advertised option that fails to apply still enters the existing warning path.

`persistSessionModelsSnapshot` preserves settlement for the same settings owner.
Attempt and source changes retain the existing ownership checks.
Live events can omit settlement. Backend startup publication and configuration
confirmation continue to use their existing settlement barriers.

The selector correction does not change these backend rules. Saved unsupported
keys remain on disk and never become synthetic controls or provider requests.

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

A missing catalog remains distinct from an absent settlement marker.
The repair never sends an unsupported option to force readiness.
It does not change permissions, recovery admission, or provider-restored policy.

Existing `model-selector:gate` diagnostics expose the required keys, option IDs,
and availability result. No new log stream or metric is required.
The incident's original missing-marker transition remains unproven. This design
does not identify a provider or transport event as that transition's cause.

## Related decisions

- [Explicit recovery settings](../../../decisions/2026-09-29-explicit-resume-settings.md)
  retains provider-restored authority and saved selections.
- [Original reconciliation plan](../../../plans/session-config-cross-agent-reconcile/plan.md)
  records the prior backend filter and settled-catalog repair.
- [Resume model-label plan](../../../plans/resumed-session-model-labels/plan.md)
  records the existing startup hydration barrier.

No new ADR is required. This correction implements the existing availability
criteria without changing provider authority or a public contract.
