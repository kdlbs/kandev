---
status: current
system: agents
requirements:
  - REQ-AGENTS-CREATION-CATALOGUE-001
  - REQ-AGENTS-PROFILE-DELETION-CATALOGUE-001
  - REQ-AGENTS-PROFILE-SAVE-CATALOGUE-001
---

# Agent profile mutation catalogue design

## Purpose and boundaries

This design restores local publication through the normal agent creation page.
It uses the established owning app store and existing API/WS normalization. It
introduces no persistence, transport or ordering abstraction. For creation, the
independent owner must already be in `settingsAgents.items`; same-target
concurrent changes are outside that creation contract. The separate list-row
deletion contract preserves independently held options without rebuilding them.
The concrete editor save sections below own only accepted target publication.

## Requirement mapping

| Criteria of REQ-AGENTS-CREATION-CATALOGUE-001 | Design sections |
| --- | --- |
| .1, .2 | Current-store publication, Projection |
| .3, .4, .5 | Creation callbacks and partial results |
| .6 | Drafts, navigation and mobile |

`REQ-AGENTS-PROFILE-DELETION-CATALOGUE-001` maps to the accepted-deletion
sections below: .1-.3 to publication, .4 to failure behavior, .5 to consumers,
and .6 to permissions and responsive controls.

`REQ-AGENTS-PROFILE-SAVE-CATALOGUE-001` maps .1-.2 to concrete save publication,
.3-.4 to compatibility and .5 to the rendered regression boundary below.

## Current source and accepted evidence

At checkout/main `8ca57f611c90ee696043b340883ac8b02aab0ad2`, the accepted proof's
three source blobs still match: `page.tsx` =
`5234fb64bfe9521b906950885d9640c025ae7b39`, `agent-save-helpers.ts` =
`52f6080cebea2e66d629d20b3cbece80a4ef0f69`, `agent-save-contributor.ts` =
`08c7c1c657a7b63c7d11f5787d48cf7aa9aaf176` (all under
`apps/web/app/settings/agents/[agentId]/`).

ROOT's qualified receipt accepts corrected native20995/terminal3ff8ce, joined
exit1: one causal failure and two passing controls, reaching both current-store
and actual picker observations. The held real creation POST allows the real
`agent.profile.created` handler to publish another existing owner's profile;
older creation then removes it from both catalogue and picker. Earlier
native92011/146341 was an external fixture matcher error and is noncausal.
This package accepts the receipt without executing or accessing its protected
test. No server deletion or ordinary-edit behavior is inferred.

## Components and responsibilities

Paths in this table are relative to `apps/web/`.

| Component | Responsibility and current source anchor |
| --- | --- |
| `app/settings/agents/[agentId]/page.tsx` | `AgentSetupPage` resolves creation from discovery or configured name/id; saved ordinary routes redirect. `useAgentCreationStoreSync` publishes both catalogue and options. |
| `app/settings/agents/[agentId]/agent-save-helpers.ts` | `saveNewAgent` and `saveExistingAgent` assemble accepted targets and invoke `upsertAgent`; partial reconciliation uses the same callback. |
| `app/settings/agents/[agentId]/agent-save-contributor.ts` | `useAgentSaveContributor` registers the actual page draft with the shared settings coordinator. |
| `app/actions/agents.ts` | `createAgentAction` line 71 and `createAgentProfileAction` line 118 call `fetchJson` via `agentSettingsRequest`, normalizing real responses. |
| `lib/ws/handlers/agents.ts` | `registerAgentsHandlers` line 247 uses `applyProfileCreatedEvent` line 217 to update the owning store and flattened options. |
| `components/settings/agent-profile-picker.tsx` | `AgentProfilePicker` consumes actual options; a missing selected option becomes an unavailable entry. |

## Current-store publication

Creation publication uses `useAgentCreationStoreSync`, bound through `useAppStoreApi()`
to this provider's store. Inside `upsertAgent`, obtain
`storeApi.getState().settingsAgents.items` immediately before deciding whether
to replace the accepted target by ID or append it. Do not read the catalogue at
render, save admission or before the awaited request. Remove the captured
catalogue selector from this hook. Keep target replacement and insertion order
as today; other owners retain the current store's objects and profile membership.

Pass the resulting list to the existing synchronous `syncAgentsToStore` pair
of setters. There is no asynchronous gap inside publication and no new global
atomic-publication promise. Retain `setSettingsAgents` and `setAgentProfiles`
action behavior; do not change slice metadata, versions or WS handling.

The app-store factory observes Settings owner removals once for that store's
lifetime. Pending new-owner callbacks consult this observation even after their
editor unmounts. Store instances have independent observations; a never-observed
absent identity remains eligible for accepted new-owner publication. This adds
no persisted field or server catalogue contract.

The narrow precedent is `ProfileRow.handleDelete` in
`components/settings/agents/agent-profiles-section.tsx:374` and standalone
dynamic save in `components/settings/dynamic-agent-profile-editor-state.ts:187`:
both read their owning store after the await. Their operations and their
same-owner revision checks are not part of this repair.

## Projection

Keep the current `nextAgents.flatMap(...toAgentProfileOption)` projection.
`lib/state/slices/settings/types.ts:290` owns labels, identity, enabled state,
capability and model mapping. The same current list feeds the catalogue and
flattened choices. Preserve accepted target metadata and existing profile order.

An event for an absent owner currently creates an option but no agent row in
`handleProfileCreated`; a full flatten rebuild can drop that orphan option.
This design neither hides that limitation nor extends its promise to absent
owners. Existing `reconcileAgentProfileOptions` and `applyProfileDuplicated`
preserve orphan options with revision merging in other flows. Importing their
broader merge policy is unnecessary for this qualified existing-owner defect.

## Creation callbacks and partial results

| Reachable callback | Target semantics to preserve |
| --- | --- |
| Configured owner, `?mode=create`, successful `saveExistingAgent` | `saveExistingProfiles` starts with saved target profiles, appends accepted profiles, skips deletion, then publishes `nextAgent`. Preserve this intentional assembly. |
| Configured owner, accepted POST then failed MCP write | `PartialProfileSaveError` carries accepted identities, pending MCP drafts and submitted-ID correlation. `reconcilePartialProfileSave` publishes those accepted profiles then rethrows the original error; no successful route replacement. |
| Discovered owner, successful `saveNewAgent` | Create owner plus profiles; save MCP; optionally PATCH the MCP path; publish accepted target and replace the route with its encoded agent name. |
| Discovered owner, accepted create then failed MCP write | `preservePendingMcpDrafts` publishes accepted identities with pending MCP data, remaps the draft, replaces the route as currently implemented, then rejects. Do not invent all-or-nothing rollback or suppress this navigation. |
| Rejected creation POST | No accepted target callback publication. Existing error propagation and page draft remain. |

`useAgentSaveHandlers` is the sole production caller of these two save helpers.
Another creation API caller is `components/agent/cli-profile-editor.tsx:596`:
it chooses an existing owner or creates an owner and invokes `onSaved(profile)`.
It does not invoke this page's catalogue upsert and is excluded from modification
and claims. This is a creation-caller inventory, not a broad writer audit.

## Drafts, navigation and mobile

Leave `mergeSavedAgentDraft`, ID correlation, current draft functional setters,
validation, admin checks, contributors and real routing intact. Tests observe
the actual coordinator result and route rather than substituting either.
Do not require a new-agent partial error to keep the creation page mounted:
its existing route replacement may redirect to the Agents index.

This is pure state/data publication inside unchanged composition and copy.
The mobile-parity narrow exception applies: one shared publication path serves
phone and desktop. Targeted real component tests prove the data outcome; no
new viewport, touch, scroll or navigation design, ASCII layout or mobile
Playwright scenario is required. Expand coverage if implementation changes
composition or interactions.

## Verification boundary

Independent permanent tests must be authored after ROOT's explicit release in
`app/settings/agents/[agentId]/agent-create-catalogue.test.tsx`. Mount actual
`AgentSetupPage`, `StateProvider`/`createAppStore`, `ToastProvider`,
`SettingsSaveProvider`, API actions, WS handlers and `AgentProfilePicker`.
Only external `fetchJson` transport and the external editor capability may be
stubbed. Use actual browser history and shared `saveAll`; select a real option
before releasing the deferred POST, then assert both store and picker after it
settles. Clean up deferred requests, providers and navigation blockers.

Seed the actual auth state with an administrator identity, discovery for the
creation route, loaded available-agent metadata with a valid default model and
permissions, loaded secrets and both catalogue/projection slices. Do not mock
`useIsAdmin`, discovery hooks or `useAvailableAgents`. Keep dynamic capability
status settled so this static fixture does not start an unrelated poller.

Existing helper tests stub actions and `upsertAgent`, so they do not prove the
page's publication or real options. They remain useful for partial-result and
draft-remapping compatibility. The plan maps exact tests and checks.

## Persistence, permissions and observability

No backend, schema, auth or persistence changes. Existing administrator
configuration permission and validation apply. No extra logs or metrics are
needed; the real store, picker, coordinator and route provide regression evidence.
No ADR is required for this local conformance repair of existing store ownership.

## Implementation plan

- [Creation catalogue preservation](../../../plans/agent-creation-catalogue-preservation/plan.md)
- [List-row deletion catalogue preservation](../../../plans/agent-profile-delete-inventory/plan.md)
- [Concrete editor save catalogue preservation](../../../plans/agent-profile-save-catalogue/plan.md)

## Accepted list-row deletion: evidence and inventory

The active `AgentProfilesSubList` renders `ProfileRow` from
`components/settings/agents/agent-profiles-section.tsx` on the Agents index.
`ProfileRow.handleDelete` calls `deleteAgentProfileAction(profile.id)`. Its
successful branch reads current `settingsAgents.items` after the await, removes
the target, then rebuilds all `agentProfiles.items` with `nextAgents.flatMap`.
That rebuild is the defect: a settings catalogue is not a complete picker
inventory at every instant.

`registerAgentsHandlers` in `lib/ws/handlers/agents.ts` registers the real
`agent.profile.created` producer. It normalizes an accepted global profile,
builds an option with `profileEventAgent`/`toAgentProfileOption`, and publishes
that option even if its owner is absent from `settingsAgents.items`. It only
adds a nested profile when an owner row already exists. An absent owner does
not invalidate the option; the event's inference capability and profile data
remain available to consumers. Office-scoped events are rejected here.

`applyProfileDuplicated` in `hooks/domains/settings/use-profile-duplicate.ts`
already preserves options not represented by its rebuild using
`mergeOptionsByNewest`. This is context for the independent inventory, not a
deletion algorithm to import. Deletion owns only the removed ID and has no
reason to recreate or arbitrate unrelated option values.

ROOT's qualified receipt on main `c176df170bf2ceee5ab4e3f55dc3b296ab187b55`
reports native46666, joined `cb7205`, exit 1: one causal rendered-row failure
loses selectable `root-new-profile` after delete ACK, and two passing controls
prove ordinary deletion and failed deletion. The receipt is
`/tmp/kandev-root-profile-delete-options-discovery-20261010/qualified-proof.json`.
Its protected candidate test is not an implementation input and must never be
read, copied, modified or removed. This design accepts the receipt and confirms
the active source path; no new execution was performed at the design checkpoint.

## Accepted list-row deletion: publication

Keep the existing successful `ProfileRow.handleDelete` branch and owning
`useAppStoreApi()` store. After `status === "ok"`, read current state at write
time. Derive the next settings catalogue by filtering only `profile.id` from
its nested profiles. Independently derive the next options by filtering only
that ID from current `agentProfiles.items`. Publish through the existing
synchronous `setSettingsAgents` and `setAgentProfiles` actions, without an
await between reading and publication. Do not capture either inventory before
the request and do not flatten settings rows into picker options.

All remaining options retain their values and relative order, including options
whose owners are absent, and options newer than the settings representation of
the same profile. Preserve current agent/profile metadata in the catalogue.
Preserve slice metadata other than the profile snapshot epoch. The setters
replace only items; accepted deletion advances `agentProfiles.version` so an
in-flight list read cannot reintroduce the deleted profile. This epoch fence is
owned by [Settings profile ordering](profile-list-ordering.md). No global atomic-publication or revision
contract is added. Reading current state for each accepted response preserves
the existing overlapping-delete behavior in either completion order.

Remove the now-unused `toAgentProfileOption` import and replace the inaccurate
flattened-inventory comment with the deletion invariant. Do not add a helper,
hook, store action, owner stub or merge policy for this local correction.

## Accepted list-row deletion: consumers and controls

Task creation reads `agentProfiles.items` in `task-create-dialog-state.ts`;
`filterCompatibleAgentProfiles` in `task-create-dialog-computed.ts` applies
enablement, dynamic routing and executor eligibility before
`useAgentProfileOptions` in `task-create-dialog-options.tsx` renders labels.
`CreateEditSelectors` in `task-create-dialog-form-body.tsx` uses the actual
`AgentSelector`/`Combobox` consumer. `NewSubtaskDialog` likewise reads
`agentProfiles.items`, derives `useAgentProfileOptions(..., "task_create")`,
and renders `SelectorsRow` in `task/new-subtask-form-parts.tsx`. Neither requires
a settings owner row to display an otherwise eligible option.

Preserving an option does not change its eligibility. Keep disabled/dynamic
gates, executor compatibility, global/Office boundaries and selected-label
behavior. Do not change a selection, choose a fallback or fetch a new catalogue.
Keep the non-success branches intact: conflict toast and `router.push(href)`,
handled-error return, ordinary error toast, and confirmation close/focus return.
Server reference checks and guided profile-page resolution are unchanged.

`useIsAdmin` continues to gate row actions. Keep full-desktop inline icons,
compact overflow actions, fine-pointer anchored confirmation and coarse-pointer
inline confirmation. Phones retain the real `AgentProfileDeleteConfirmation`
and `MobileActionConfirmation`, with menu dismissal before the confirmation,
the same cancellation/focus owner and existing touch geometry. This state-only
correction meets the narrow mobile-parity exception; it changes no layout,
navigation, scroll owner, overlay, copy or interaction. Existing mobile
`mobile-agent-profile-delete.spec.ts` is read-only compatibility context for
row menu/cancel/focus controls. Real rendered row/task/subtask selector proof
and affected row/admin/confirmation controls cover this state/data-only change;
no untouched mobile E2E replay or rebuild is required. Any actual viewport,
layout or control change requires scope reassessment.

## Accepted list-row deletion: regression boundary

Author a new independent `agent-profiles-section-delete-inventory.test.tsx`
alongside the row only after explicit implementation release. Use actual
`ProfileRow`, `createAppStore` with subscribed `useStore`/`StateProvider`, and
`registerAgentsHandlers`. Hold the deletion response at the external transport
boundary, deliver a real created event for an absent owner, prove its actual
choice is selectable before ACK, then inspect both slices and real task/subtask
option labels after ACK. Use actual derivation and `AgentSelector`/`Combobox`
through `CreateEditSelectors` and `SelectorsRow`; do not replace them with a
test-only list or prove only stored IDs.

Seed settled available-agent metadata, administrator auth, recent-use readiness
and the other loaded slices needed by real hooks. Clean up deferred requests,
mounted subscriptions and UI. Require a causal assertion failure before the
fix, not a setup/import/cleanup failure. Add ordinary success, rejected deletion,
overlapping completions, newer option metadata and mixed eligible/disabled
controls; pass an Office-scoped event through the unchanged handler to confirm
its boundary. Preserve existing row and admin tests and use their real controls
for conflict/handled error and mobile compatibility where coverage is missing.
Do not change `useProfileEnabledToggle`, which has no active caller in this path.

No new ADR, public docs, backend change, API change or global-handler repair is
needed. This correction restores an existing local ownership boundary; its
reason fits the owning pair and focused work order. The creation sections and
their completed delivery package retain their separate scope and guarantees.

## Concrete editor save: source and evidence

At main `ac25d0c22e8e7db8c57cddd5098ba6946dd7d4af`, the active
`components/settings/agent-profile-page.tsx::ProfileEditor` calls
`useProfileEditorState`, `useProfileSave` and `useSyncAgentsToStore` from
`agent-profile-page-state.ts`. After a real PATCH resolves, `useProfileSave`
calls `acceptProfileSaveResponse`, then builds `nextAgents` from its captured
`settingsAgents`. `useSyncAgentsToStore` publishes that list and reconciles
options by newest revision. The options merge retains independently received
choices, but cannot distinguish a deleted sibling newly rebuilt from the stale
nested list. `registerAgentsHandlers` already removes live deletions from both
slices; rewriting that handler does not repair the save writer.

ROOT reports qualified native75344, actual join `d7edb6`, exit 1: one causal
post-ACK failure plus two passing controls (ordinary save and rejected PATCH).
The selected deleted sibling changes to Unavailable during the held save, then
becomes selectable again after another profile's accepted name/revision save.
This is ROOT's receipt, not independently executed evidence for this package.
Its protected source is never read or copied. No service deletion reversal or
other-owner scenario is claimed as proved.

## Concrete editor save: publication

Keep `useProfileSave` in its existing file. Bind the existing `useAppStoreApi`
to the mounted provider. Only after the awaited PATCH and existing
`acceptProfileSaveResponse` acceptance, read the current store. Resolve the
current owner by the admitted agent ID and its current target by response ID.
If either is absent, skip catalogue/options publication; never append an owner
or profile. Keep local save-response acceptance and status semantics intact.

For a present target, replace only that target in the current owner's current
profiles, preserving other owners, profile order and current owner metadata.
Publish through the existing `setSettingsAgents` action. Reconcile current
options with only the accepted target projected through its current owner:
`reconcileAgentProfileOptions(currentOptions, [{ ...currentOwner,
profiles: [updated] }])`. This preserves the existing newest-option rule for
the target and leaves every unrelated option's values and order intact. Use
the existing `setAgentProfiles` action and preserve its slice metadata/version.
No await occurs between reading and these synchronous setters; this adds no
global atomicity guarantee.

Remove only the save hook's captured `settingsAgents` and `syncAgentsToStore`
arguments and their concrete save call-site entries. `ProfileEditor` still
needs those values for `useProfileDelete`; retain that existing hook and
`useSyncAgentsToStore` behavior. No new hook is needed. If a correctness-driven
extraction becomes necessary, new hooks belong in `hooks/domains/settings/`.

## Concrete editor save: compatibility

Leave `shouldSyncProfileSaveResponse`, `useProfileEditorState`,
`reconcileAgentProfileSnapshot` and `sameEditableProfile` unchanged. Retain
normalization in `updateAgentProfileAction`, the PATCH builder's permissions,
enabled omission, Cursor MCP preferences and provider fields, submitted-snapshot
correlation, dirty-draft handling, dependency/utility conflict and force behavior.
The separate MCP document contributor remains separate. The
[Platform editor design](../../platform/system-design/agent-settings-parity.md#editor-reconciliation)
owns these baseline and revision rules; this repair adds no server ordering.

No backend, auth, selection, tombstone, persistence or observability change is
needed. This restores local writer ownership and needs no new ADR or public
workflow documentation. The existing page/picker composition serves phone and
desktop; targeted rendered state/data checks satisfy the mobile-parity narrow
exception. If implementation changes layout, navigation or touch behavior,
reassess scope before proceeding.

## Concrete editor save: rendered regression boundary

After explicit implementation release, independently author
`components/settings/agent-profile-save-catalogue.test.tsx`. Prefer the actual
`AgentProfilePage`/`ProfileEditor`, `StateProvider`, `SettingsSaveProvider`, real
API PATCH action/normalizer, registered WS handler and real
`AgentProfilePicker`/`Combobox`. Exercise actual task/subtask selector derivation
and controls for preserved eligible/ineligible options. Hold only external
transport and seed settled capability/auth/settings prerequisites. If a
noncausal external capability or toast effect needs isolation, document exactly
what it replaces; never stub save acceptance, reconciliation, store actions,
API adapter, WS handler or actual choice consumer.

Select the same-owner sibling before saving another profile, deliver the real
deletion while PATCH is held, prove unavailable before ACK, and assert both
nested membership and actual absence of a selectable choice after ACK. Require
one causal RED and ordinary success/rejected PATCH controls passing before the
fix, then GREEN. Add bounded current target/owner absence, same-owner latest
metadata/new membership, independent options, eligibility and late revision/
new-draft controls. These are planned regressions, not already proved cases.
Seed real loaded resources and clean up requests, subscriptions, coordinator
and navigation blockers. Adapt existing direct save-hook mocks only to supply
the actual store dependency; retain their payload assertions and run all
changed suites. The work order owns exact commands and final evidence.
