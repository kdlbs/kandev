---
status: current
system: agents
requirements:
  - REQ-AGENTS-CREATION-CATALOGUE-001
---

# Agent creation catalogue design

## Purpose and boundaries

This design restores local publication through the normal agent creation page.
It uses the established owning app store and existing API/WS normalization. It
introduces no persistence, transport or ordering abstraction. The independent
owner must already be in `settingsAgents.items`; same-target concurrent changes
are outside this contract.

## Requirement mapping

| Criteria of REQ-AGENTS-CREATION-CATALOGUE-001 | Design sections |
| --- | --- |
| .1, .2 | Current-store publication, Projection |
| .3, .4, .5 | Creation callbacks and partial results |
| .6 | Drafts, navigation and mobile |

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
| `app/settings/agents/[agentId]/page.tsx` | `AgentSetupPage` resolves creation from discovery or configured name/id; saved ordinary routes redirect. `useAgentStoreSync` at line 169 publishes both catalogue and options. |
| `app/settings/agents/[agentId]/agent-save-helpers.ts` | `saveNewAgent` line 320 and `saveExistingAgent` line 506 assemble accepted targets and invoke `upsertAgent`; partial reconciliation at lines 329 and 475 uses the same callback. |
| `app/settings/agents/[agentId]/agent-save-contributor.ts` | `useAgentSaveContributor` registers the actual page draft with the shared settings coordinator. |
| `app/actions/agents.ts` | `createAgentAction` line 71 and `createAgentProfileAction` line 118 call `fetchJson` via `agentSettingsRequest`, normalizing real responses. |
| `lib/ws/handlers/agents.ts` | `registerAgentsHandlers` line 247 uses `applyProfileCreatedEvent` line 217 to update the owning store and flattened options. |
| `components/settings/agent-profile-picker.tsx` | `AgentProfilePicker` consumes actual options; a missing selected option becomes an unavailable entry. |

## Current-store publication

Only the creation page's `useAgentStoreSync` needs a production change. Bind
`useAppStoreApi()` to this mounted provider's store. Inside `upsertAgent`, obtain
`storeApi.getState().settingsAgents.items` immediately before deciding whether
to replace the accepted target by ID or append it. Do not read the catalogue at
render, save admission or before the awaited request. Remove the captured
catalogue selector from this hook. Keep target replacement and insertion order
as today; other owners retain the current store's objects and profile membership.

Pass the resulting list to the existing synchronous `syncAgentsToStore` pair
of setters. There is no asynchronous gap inside publication and no new global
atomic-publication promise. Retain `setSettingsAgents` and `setAgentProfiles`
action behavior; do not change slice metadata, versions or WS handling.

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
