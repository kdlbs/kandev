---
status: current
system: agents
requirements:
  - REQ-AGENTS-TARGET-CREATION-CATALOGUE-001
---

# Existing-owner profile creation catalogue design

## Boundary and evidence

Extend only accepted additional-profile publication on the configured-owner
`AgentSetupPage` creation route. The [existing creation design](creation-catalogue.md)
already reads the current outer catalogue, but deliberately replaces the target
with the helper's assembled target snapshot. Its different-owner and new-agent
behavior remains. This extension does not reopen its deletion design.

ROOT's qualified receipt at
`/tmp/kandev-root-creation-target-discovery-20261010/qualified-proof.json` records
main `94b762a6704d9da7f09fbc822228a5f11cbdf6bc`, native89683, actual terminal
join `92e311`, exit 1: one causal case fails after ACK in both nested catalogue
and actual picker; ordinary accepted creation and rejected POST controls pass.
Before ACK, the same-owner sibling is actually selectable and selected. Only
external `fetchJson` transport is isolated; page, saveAll, adapters, normalizer,
store, registered WS handler, navigation and picker are real. This is supplied
evidence, not child execution. The protected candidate is never an input to read,
copy or delete; implementation authors independent tests after release.

## Requirement mapping

| Criteria of REQ-AGENTS-TARGET-CREATION-CATALOGUE-001 | Design sections |
| --- | --- |
| .1, .2 | Accepted-creation publication |
| .3 | Creation result boundary; Drafts and compatibility |
| .4, .5 | Partial and rejected results |
| .6 | Drafts and compatibility; Verification |

## Current components

Paths are relative to `apps/web/`.

| Component | Current responsibility |
| --- | --- |
| `app/settings/agents/[agentId]/page.tsx` | `AgentSetupPage`, `useAgentSaveHandlers`; actual creation form, callback wiring and navigation |
| `hooks/domains/settings/use-agent-creation-store-sync.ts` | `useAgentCreationStoreSync`; callback-time current target read, submitted-result merge and option projection |
| `app/settings/agents/[agentId]/agent-save-helpers.ts` | `saveExistingAgentPatch`, `saveExistingProfiles`, `saveExistingAgent`, `reconcilePartialProfileSave`; accepted responses, MCP, draft correlation |
| `app/settings/agents/[agentId]/agent-save-contributor.ts` | Actual contribution to `SettingsSaveProvider.saveAll`, dirty/valid/admin admission |
| `app/actions/agents.ts` | Real PATCH/POST request adapter and `normalizeAgentProfile` response conversion |
| `lib/ws/handlers/agents.ts` | `registerAgentsHandlers` and `applyProfileCreatedEvent` publish the same-owner sibling and choice |
| `components/settings/agent-profile-picker.tsx` | Actual option/selected-label consumer through `Combobox` |

Before this correction, `saveExistingProfiles` seeds create-mode `nextProfiles` with captured
`savedAgent.profiles`, then appends POST responses. `saveExistingAgent` publishes
that array through `upsertAgent`; partial reconciliation similarly seeds a map
from the captured profiles. The page's original store-sync hook reads current owners but replaces
the target wholesale. Its flatten rebuild consequently removes a live target
sibling. Reading only the outer list later is insufficient.

## Creation result boundary

`SaveAgentCallbacks.upsertAgent` accepts the optional, narrowly typed
`AgentCreationPublication` context. It identifies only
the normalized profiles actually accepted as new by this submission, plus the
agent fields owned by a successful submitted patch when applicable. This context
is supplied only for `saveExistingAgent` with `isCreateMode`; new-agent and ordinary
save callers retain their existing behavior.

Derive publication entries from actual persisted results for submitted creation
draft rows and their submitted-ID correlation, never the baseline profiles seeded
into `nextProfiles`. Initial creation publishes new POST identities; a partial
retry can also publish the previously accepted, remapped submitted row after its
pending save completes. The full assembled `nextAgent.profiles` is still needed
for existing draft reconciliation; it is not the list of profiles publication owns.
Never append a captured sibling merely because it is absent from the current
catalogue. Carry the actual accepted subset from `PartialProfileSaveError` into
partial publication.

Create-mode retry replaces the submitted persisted profile's baseline entry in
`nextProfiles` instead of appending it a second time. This keeps the draft's accepted
identity unique after partial MCP recovery; ordinary-save assembly is unchanged.

## Accepted-creation publication

At the creation callback, read the mounted provider's current store immediately
before publication. For the existing target, base the result on its current
agent object and current profile membership. Retain every sibling object and
relative order. Add accepted new profiles exactly once by persisted ID; neither
replace the target with the assembled snapshot nor rebuild it from the captured
baseline. Preserve unrelated owners through the existing current outer-list
read. Apply only the existing successful submission's owned workspace/MCP-path
fields where needed; do not republish captured owner capability metadata.

If an accepted ID has already arrived through the event path, replace/deduplicate
only that ID. Reuse `compareTimestamps` in settings types for the bounded same-ID
case: a strictly newer current profile remains, otherwise the normalized accepted
profile wins. Partial MCP draft data is retained separately on that accepted ID
even if its current persisted representation is newer. This is a local callback
guard, not a new global revision or conflict policy.

If the current owner is missing, the existing-owner creation branch does not
reinsert its captured owner. Do not claim server deletion or orphan inventory
recovery; leave helper error/draft/navigation semantics unchanged. Test this
guard only at the changed publication boundary.

The existing synchronous `setSettingsAgents`/`setAgentProfiles` projection uses
the resulting catalogue and `toAgentProfileOption`. There is no await between
the read and publication, and no new atomicity promise. Global option retention
or revision merging outside this existing-owner scope remains excluded.

## Partial and rejected results

Rejected POST publishes no accepted creation. Existing errors propagate through
the real coordinator and keep newer draft edits. For accepted POST followed by
MCP failure, `reconcilePartialProfileSave` publishes only accepted new profiles
over the current owner, retains `mcp_config`, uses the existing `profileIds` map
for `mergeSavedAgentDraft`, and rethrows `partial.original`. Successful-save
navigation must not run. Retain retry behavior for a remapped persisted profile.
Do not turn a partial result into rollback or a successful shared save.

## Drafts and compatibility

Keep `cloneAgent`, `ensureProfiles`, `mergeSavedAgentDraft`, validation and permission
payloads unchanged. Publish current membership to consumers without importing
independent siblings into the submitted creation draft or rewriting its revision.
Retain current post-save encoded-name route behavior. The state-only correction
uses the same data on desktop and phone; it changes no composition, scrolling,
overlay, touch, copy or viewport interaction. Mobile-parity's narrow exception
applies. No ASCII redesign, whole-app build or untouched mobile E2E run is needed.

No backend, persistence, API/event, permission, feature-flag or observability
change is required. This local callback boundary does not warrant a new ADR.
The extracted `useAgentCreationStoreSync` hook and its guard tests live in
`hooks/domains/settings`, per web guidance. Existing save-helper functions remain. Keep sibling105's concrete-profile save
work in PR4390 separate and recheck the actual moving base before implementation.

## Verification

The [single work order](../../../plans/target-profile-creation-catalogue/task-01-preserve-current-target.md)
owns independently authored real-page causal RED and two controls before code,
then current-sibling metadata, accepted partial-MCP, same-ID and missing-owner
boundary checks. Only external transport is isolated. Existing creation coverage
provides different-owner/new-agent compatibility; helper suites cover changed
callback and draft/partial paths. Record child execution separately from ROOT's
supplied receipt.
