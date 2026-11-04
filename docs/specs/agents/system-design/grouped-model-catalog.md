---
status: draft
system: agents
created: 2026-11-03
requirements:
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004
---

# Grouped and composed model catalog system design

## Purpose and boundaries

Preserve provider model structure from ACP through the shared selector for [issue #5026](https://github.com/kdlbs/kandev/issues/5026).
Agents owns catalog meaning, exact model identity, and provider interpretation.
The UI consumes normalized data and retains its existing configuration-summary contract.
Requirements 002 through 004 extend the settings resolver in requirement 001 to structured catalogs and live presentation.
The [first-draft correction](profile-capability-discovery.md#identity-before-the-first-profile-save) remains a prerequisite for new-profile acceptance.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001 | Projection and cache ownership; Resolution and application |
| REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002 | Catalog shape; ACP preservation; Restoration |
| REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003 | Composition qualification; Exact combination resolution |
| REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004 | Shared selector; Resolution and application; Mobile and summaries |

## Evidence and current loss

At commit `8cd31a0896abc50e6d9683369cee92252c5d2e44`, live `convertACPConfigOptions` copies each group's choices into one flat array.
Utility `probeConfigOptions` uses `selectOptionsUngrouped`, which also discards the groups.
The stream, probe, host utility, and settings types cannot represent the lost structure.
Frontend `configOptionToModelOptions` and selector types consume flat choices.

Two temporary no-inference tests passed and reproduced the loss in both converters.
Each input contained two native groups and two choices with repeated display labels.
Both exact values and choice descriptions survived. Both group IDs and labels disappeared.
The tests were removed after diagnosis.

The issue reports grouped Devin options after advertising `_meta["cognition.ai/groupedSessionConfigOptions"]=true`.
The installed SDK represents native groups through `group`, `name`, and a flat choice list.
ACP does not define arbitrary nesting or composed-model bindings in that type.
The issue's example pairing and catalog counts are observations, not production constants or a decoder grammar.

## Catalog shape

Add an optional typed `catalog` field to the existing normalized select option.
Retain `options` as a derived flat compatibility projection for existing consumers and older records.
There is one catalog and one owning configuration snapshot, not another selection store.

Use shared Go types under `internal/common/modelcatalog` and TypeScript types under `@kandev/types/model-catalog`.
Backend and independently shipped agentctl consume the neutral Go package.
The catalog contains:

- `version: 1`.
- Ordered `nodes`, a bounded union of group and choice nodes.
- Group nodes: stable scoped ID, label, optional description, and children.
- Choice nodes: exact provider value, label, and optional description.
- Optional `compositions`, scoped to a group ID.
- A composition: ordered dimensions and explicit bindings from complete dimension values to exact advertised choice values.
- A dimension: stable ID, label, and ordered value choices.

Nodes contain presentation data, without expressions, commands, executable callbacks, or renderer instructions.
Native ACP groups become one group level. Additional nesting requires supplied or qualified relationships.
Repeated labels remain distinguishable through group paths.
Repeated provider values can retain separate presentation occurrences without changing executable identity.

Validate depth, node count, strings, dimensions, compositions, and bindings before publication.
Initial limits are four total catalog node levels (the root list plus three nested group levels), 4096 nodes, 64 compositions, eight dimensions per composition, 256 choices per dimension, and 4096 bindings.
Malformed or oversized richer metadata retains the original validated flat choices.
Do not truncate a composition into an apparently complete but inaccurate mapping.
Forward only reviewed presentation metadata, not arbitrary vendor `_meta`, filesystem/runtime details, inferred pricing, or recommendations.
Existing qualified metadata for other providers keeps its current behavior.

## ACP preservation

Extend `acpcompat.ClientCapabilityMeta` for Devin's qualified grouped-options capability.
Live, host-probe, and one-shot handshakes already share that helper.
Preserve Cursor and other existing metadata without mutating caller maps.
Do not advertise Devin's extension to unrelated providers.

Put shared typed ACP group conversion in `internal/agentctl/acpcompat`.
Both `adapter/transport/acp` and `server/utility` consume the same normalized meaning.
Preserve groups in initial, restore, update, and post-selection snapshots.
Untyped `_meta.configOptions` compatibility follows the same validation and preservation rules.
The flat index supports legacy lookup but cannot replace the new catalog.
Support providers that return config options without legacy `models`.

## Composition qualification

Prefer explicit provider relationship metadata or a documented model catalogue.
Use backend compatibility normalization for any necessary Devin interpretation.
The renderer must not parse model IDs, names, `fusion`, `lead`, or `sidekick`.

Task 04 is a mandatory qualification gate before implementing Devin composition bindings.
Record the actual source, mapping rules, ambiguity handling, and supported observed shape.
An illustrative ID from the issue is not a verified decoder contract.
A narrow decoder needs sanitized real fixtures and exact current-catalog validation.
Unknown, changed, or ambiguous entries remain ordinary selectable leaves.

The [Fusion documentation](https://docs.devin.ai/cli/fusion) explains the controls but does not publish a machine-readable relationship schema.
If qualification cannot establish the bindings, report the missing evidence and retain the gate.
Do not mark #5026 complete because grouping or synthetic compositions work.
Its Fusion criterion requires verified bindings.

## Projection and cache ownership

Carry the optional catalog through stream and probe config options, host utility conversions/results/caches, and settings DTOs.
Preserve it through agentctl events, lifecycle projection, orchestrator publication, and existing session selector metadata.
Extend task-detail boot state, WebSocket payloads, frontend runtime slices, and profile/workflow option adapters.
`agentctl/sessionmodel.ConfigOption` stays a narrow operation-selection view. It does not need the full catalog.

Live data belongs to the current session snapshot.
Settings data belongs to its existing authorized probe context and transient cache.
Preserve profile scope, launch context, selected model/mode/options, runtime generation, and opaque context revisions.
Resolve dimensions to an exact advertised model before requesting dependent options.
Do not key caches by display labels or make a settings result authoritative for a live session.

## Exact combination resolution

The shared pure resolver accepts a composition, draft dimension values, and the current catalog.
It filters valid bindings and reports zero, one, or ambiguous matches.
Only one complete exact match enables Apply.
A changed dimension retains valid choices and clears invalid choices visibly, without selecting another pairing.

Thinking, Speed, and future settings remain real provider config options.
Apply the exact model to the isolated settings probe and consume its complete updated option snapshot.
Effort-like model suffixes never determine the effective Thinking value.

Keep the temporary navigation/composition draft with the existing selector owner and snapshot generation.
Fence responses by surface, profile/session, launch context, model, and request generation.
Viewport changes retain the unfinished draft. Cancellation discards it without a live mutation.

## Resolution and application

Ordinary leaves retain their existing immediate selection behavior.
Composed groups open a focused view with explicit Use configuration.
The completed choice contains an exact model value and real option values.
Mode remains in the existing session-permission UI.

Profile and workflow consumers update their drafts after complete Apply.
Existing Save persists exact model/options. Group paths and dimension values remain derived presentation state.

Live consumers reuse current mutation handlers and busy-turn rules.
Picker navigation resolves candidate options without mutating the live session.
On Apply, request the exact model, observe confirmation, then request valid options.
Fence each response/update to the same session, execution, and configuration generation.

Multiple ACP calls are not an atomic transaction.
If the model succeeds and an option fails, display confirmed state and a retryable partial-application error.
Retain the conversation. Do not report full success or claim an unconfirmed rollback.
Retry starts from confirmed state and cannot replace a newer user mutation.

## Restoration

Extend the existing selector snapshot rather than add another record or table.
Older flat snapshots remain valid.
Retain separate ownership of the comparison baseline, mutable provider state, and explicit user overrides.
Startup default events cannot replace settled restore intent.
Restore exact model/mode/options before a later prompt under the current restoration policy.
Historical turns retain their captured values and labels, without borrowing from the current catalog.
Settings probe catalogs remain transient, without a database capability cache.

## Shared selector

Extend `ModelConfigSelector` and its content with optional normalized catalog data.
Share pure tree navigation, search, combination filtering, and draft state.
Do not add a Devin component or another provider selection store.
Desktop uses the existing popover with Back navigation and an in-place composition view.
Singleton ordinary groups can collapse while retaining visible context.
Search returns exact matching leaves with paths and includes dimension labels/values.
Keep the selected/current entry discoverable outside the open group.

## Mobile and summaries

Phone uses one inset bottom sheet with a fixed header, one vertical scroller, and safe-area-aware Apply.
Dimension choices replace its body and return through Back, without stacked overlays.
Use `MobilePickerSheet` as an interaction precedent without importing task-owned behavior into settings.
Use shared primitives with 28px fine-pointer controls and at least 44px touch targets.
Dismissal returns focus to the owning trigger.

The compact task trigger identifies the verified pairing, then applies the existing baseline-aware option summary.
Settings triggers retain their all-values behavior.
The [summary and attribution requirement](../../ui/requirements/acp-model-configuration-summary.md) remains authoritative.
Preserve full accessible details, provider descriptions, and historical labels.

## Delivery

- [Combined implementation plan](../../../plans/first-draft-profile-discovery/plan.md)
- [Architecture decision](../../../decisions/2026-11-03-provider-neutral-model-catalog.md)
- [Requirements](../requirements/dynamic-provider-options.md)
- [Devin permission controls](devin-profile-permissions.md), a separate mode/sandbox concern.
