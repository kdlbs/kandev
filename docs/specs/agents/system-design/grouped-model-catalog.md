---
status: draft
system: agents
created: 2026-11-03
updated: 2026-11-04
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

## Implementation baseline

The original delivery preserved native ACP groups through `acpcompat.CatalogFromSelectOptions` and `CatalogFromRawOptions`.
Both the live converter and utility probe use that shared boundary.
The optional composition shape and generic editor also exist.

[Issue #5053](https://github.com/kdlbs/kandev/issues/5053) identifies two remaining gaps.
`CatalogCommandGroups` renders ordinary singleton headings, and no Devin source populates compositions.
The [repair plan](../../../plans/model-catalog-selector-repair/plan.md) records the current reproduction and remaining delivery work.

ACP's native `group`, `name`, and choice list do not establish composition relationships.
The [qualification record](../../../plans/first-draft-profile-discovery/fusion-qualification.md) separates reported native evidence from captured fixtures.

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

Devin's observed `models list --format json` command supplies `families[]` with `family_uid`, `family_label`, and `variants[]`.
Each variant supplies an exact `model_uid` and `label`.
The [Fusion documentation](https://docs.devin.ai/cli/fusion) explains the controls but does not define the pairing wire contract.

Qualify the native schema and Fusion family identity with a sanitized capture before enabling its adapter.
Use actual non-Fusion variant records as candidate leads and sidekicks.
Accept a candidate only when both relationships match exactly:

```text
fusion.model_uid == "fusion-" + lead.model_uid + "-sidekick-" + sidekick.model_uid
fusion.label == "Fusion (" + lead.label + " + " + sidekick.label + ")"
```

The native Fusion record must also match an existing ACP leaf's exact value and label.
The native catalog supplies relationships, never additional model availability.
Do not split a name or identifier into guessed dimension values.
Use bounded indexed matching, retaining every candidate until ambiguity checks finish.

Use the lead's native family identity and family label for the Lead dimension.
Use the sidekick's exact variant identity and label for Sidekick.
Preserve first occurrence order from the ACP group when deriving dimension choices and bindings.
Do not remove effort suffixes from labels with string heuristics.

Accept each advertised value only if exactly one native record and one candidate pair explain it.
Then require each `(lead family, sidekick variant)` tuple to identify exactly one advertised value.
If a tuple collides, leave every colliding member unbound.
Never choose the first match or silently add an effort dimension.

Scope each composition to its existing ACP group ID.
Keep unbound choices beside its configuration entry, in source order.
A mixed group can therefore offer verified composition and ordinary unknown choices together.
If no bindings survive, retain the ordinary group.
Validate the enriched copy with `modelcatalog.Validate` before publication.
If validation fails, preserve the original catalog and flat projection.

Thinking and Speed remain separate ACP options even when a variant identifier contains an effort-like suffix.
Unsupported fast variants and changed relationships remain ordinary leaves.
A newer CLI version qualifies only through the supported shape and exact checks, never its version number alone.
Do not claim universal decoding or mark #5026 complete from ordinary fallback or synthetic fixtures.

## Native catalog acquisition

Acquire evidence inside the owning agentctl, using the resolved launch snapshot for that ACP process or probe.
The snapshot includes the executable, authorized wrapper or prefix, working directory, environment, authentication, proxy configuration, and executor.
Preserve the existing credential and configuration environment.
Do not create an empty configuration home, copy credentials, or retry under a global account.

Construct the native command from the admitted Devin command before wrapper expansion.
Replace only the qualified ACP subcommand with `models list --format json`.
Retain configuration flags only when the captured CLI contract supports them for that subcommand.
An unknown command shape or flag translation retains ordinary selection.
Use `prepareSessionArgs` for the live wrapper and `prepareACPLaunch` for the probe wrapper.
Never remove an authorized wrapper to make discovery succeed.

Keep acquisition separate from pure compatibility normalization in `acpcompat`.
A small `internal/agentctl/devincatalog` source holds one immutable evidence snapshot and accepts an owner-provided command runner.
The source exposes a preparation operation and a pure catalog enrichment operation.
The optional internal adapter configuration carries this source through `adapter.Config.ToSharedConfig`.
It is not a browser DTO, provider registry, or second selection store.

For a live generation, `process.Manager` supplies the final admitted command context and owns the listing process.
`Adapter.Initialize` prepares the source once after successful initialization and authentication, outside configuration locks.
Every later model catalog uses pure enrichment before adapter caching and publication.
This includes typed/untyped initial state, new/load/resume/reset, notifications, model confirmations, and option confirmations.
Recheck the current catalog on every projection, so removed models cannot survive through old bindings.

For settings, `ACPInferenceExecutor.Probe` acquires at most one native snapshot after a successful ACP probe.
The listing shares the probe's deadline, sanitized environment, command preparation, and process-group cleanup.
The existing hostutility caches and flights retain authorization scope, context revisions, expiry, and refresh authority.
Do not add a host-global native cache or pass a settings result into live state.
Each uncached probe can perform one listing. Cache hits, browser renders, and ACP notifications perform none.

Give a listing a 10-second maximum within its parent's remaining deadline.
Retain at most 2 MiB of stdout and 64 KiB of stderr, with an explicit overflow result.
Parse stdout separately from stderr and reject truncated JSON, trailing data, malformed required fields, and excessive catalog structures.
Cancel and reap the whole owned process tree on timeout, overflow, parent cancellation, and shutdown.
Use the existing managed process and ACP cleanup paths instead of a bare `exec.Command` without ownership.
The host CLI runner's 64 KiB combined-output buffer is unsuitable for this source.

Source state belongs to one live launch generation or one admitted probe operation.
Stop, replacement, reconfiguration, or a new initialization discards the old source.
No old-generation result can update a replacement adapter.
The first initialization owns live preparation. Settings cancellation retains the existing hostutility flight policy for other waiters.
There is no independently detached source job or periodic poller.

Listing failure is optional enrichment failure, not ACP discovery failure.
Return the ordinary catalog without changing the selected model, profile, modes, or provider options.
An explicit settings refresh or a new live initialization can retry.
Log only a bounded outcome such as unavailable, unsupported context, timeout, overflow, or invalid data.
Raw native output, pricing, credentials, account identifiers, and filesystem paths do not enter generic logs or browser metadata.

This is distinct from [host CLI model suggestions](host-cli-model-discovery.md).
That contract can merge suggestions from a host account. Fusion enrichment only describes exact choices from the current ACP account.
The [existing catalog decision](../../../decisions/2026-11-03-provider-neutral-model-catalog.md) remains authoritative.

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
Project ordinary singleton groups into one model row at render time.
Keep the canonical nodes and `buildCatalogTree` lossless.
Determine singleton eligibility from the full catalog before search filtering.
Collapse a chain only when its entire subtree contains one ordinary choice and no composition entry.
Run composition detection first, including a composition with only one binding.
Retain multi-choice groups when search happens to leave one visible result.

Render one ordered traversal of the source entries. Do not hoist loose leaves or merge repeated labels or values.
Carry the leaf occurrence key separately from its executable model value into the command item identity.
Selection callbacks still receive the exact provider value.
Show useful removed path context as secondary row detail and accessible description, alongside any provider description.
Avoid repeating path labels that only duplicate the model label.
Paths remain searchable even when a provider description exists.
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
- [Selector repair package](../../../plans/model-catalog-selector-repair/plan.md), including source qualification and desktop/phone acceptance.
- [Architecture decision](../../../decisions/2026-11-03-provider-neutral-model-catalog.md)
- [Requirements](../requirements/dynamic-provider-options.md)
- [Devin permission controls](devin-profile-permissions.md), a separate mode/sandbox concern.
