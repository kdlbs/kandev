---
status: current
system: ui
requirements:
  - REQ-UI-SETTINGS-MANUAL-SAVE-001
---

# Settings Manual Save System Design

## Purpose and boundaries

UI owns the reusable route-level interaction in
[Settings Manual Save](../requirements/settings-manual-save.md).
[ADR 0046](../../../decisions/0046-settings-route-save-coordinator.md) assigns
coordination to the settings shell and drafts, validation, and API payloads to
domain contributors. This design records that boundary, the terminal-font
editor's registration contract, and Jira/Linear baseline publication. Backend settings
schemas, font rendering and library loading, and unrelated contributor
reconciliation remain outside it.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-SETTINGS-MANUAL-SAVE-001 | Contributor registration; Submission and reconciliation; Workflow store projections; Reset and recovery; Desktop and phone |

The terminal-font repair uses existing AC-UI-SETTINGS-MANUAL-SAVE-001.1,
.2, and .4. It adds no product requirement.

## Components and responsibilities

- `apps/web/components/settings/settings-save-provider.tsx` owns the route-local
  registry, stable save order, duplicate-submission protection, reset, and dirty
  navigation guard. `useSettingsSaveContributor` registers the domain snapshot;
  `useSettingsSaveCoordinator` exposes actual dirty state. The rendered
  `SettingsFloatingSave` derives availability from this registry.
- `apps/web/components/settings/terminal-settings.tsx` owns the
  `general-terminal` contributor, its accepted baseline, and its complete
  editable draft. `TerminalFontCard` presents font selection and custom input.
  Font size, link behavior, and preferred shell retain their existing paths.
- `StateProvider` supplies the real application store. `updateUserSettings` in
  `apps/web/lib/api/domains/settings-api.ts` uses the existing PATCH
  `/api/v1/user/settings` client. An acknowledged submission advances only the
  submitted terminal values in the store, independently of newer local edits.

## Contributor registration

A contributor revision covers every editable value that can affect its next
submission, including raw custom-font input before blur. Do not hide such edits
in child-local state outside the registered revision. No input event persists
settings. Changes to raw input immediately refresh the parent draft and revision.

The terminal font editor represents three distinct concepts: the accepted family
candidate, the selection mode, and raw custom text. A small parent-owned editor
representation keeps these alongside the terminal draft. Mode determines which
control is visible; it is not inferred anew from each raw character. Canonical
editor state can be derived from the accepted family at initialization, explicit
selection, reset, or settlement of an unchanged submission.

The full revision includes presentation mode for reconciliation. Dirty state
compares editable setting values and raw text against their accepted baseline;
entering Custom mode alone is not a durable configuration edit. Custom text
equal to its baseline is clean. Equivalent local representations are acceptable
if they preserve these distinctions without a global coordinator change.

## Submission and reconciliation

| Input or boundary | Draft and persistence behavior |
| --- | --- |
| Choose Custom | Show the custom editor without persisting or clearing the accepted family. |
| Type custom text | Register raw text immediately; keep Custom mode, including blank text or text exactly matching a preset stack. |
| Blur or Enter with nonempty trimmed text | Accept the trimmed family candidate locally; derive canonical custom or preset presentation at this boundary. No PATCH. |
| Blur or Enter with blank text | Keep the raw blank editor and last accepted family candidate. Blank text does not implicitly choose Default. |
| Choose preset or Default | Explicitly stage that selection, replace the raw editor representation, and leave Custom mode. Default retains the existing null/empty-wire convention. |
| Save | Capture the entire draft revision. Resolve nonempty custom text to its trimmed family even if it never blurred; blank text uses the last accepted family candidate. Clamp size as today and use the existing API payload. |
| Successful ACK, captured revision still current | Advance the accepted baseline and replace the draft with the canonical submitted representation. |
| Successful ACK, a newer revision exists | Advance only the accepted baseline/store. Preserve all newer raw text, mode, and sibling fields; contributor remains dirty where it differs from that baseline. |

Keep the capture immutable. Normalize the captured snapshot, never the later live
input. Compare the latest complete draft with that capture before replacing any
editor state; guarding only the accepted family misses pre-blur raw edits.
The second explicit Save submits the newer raw value and becomes clean only when
that submission is still current. A blank unchanged snapshot may canonicalize to
its retained family after successful Save; it never erases that family.

## Workflow store projections

The workflow settings contributor reconciles its clean drafts and baselines with
the shared workflow store, and its Save sends name, description, prompt and
default profile together. Every route that hydrates that store from a workflow
list therefore writes the full editable projection through the shared
`mapWorkflowItem` in `lib/routing/route-bootstrap.ts`, and the Go boot payload's
`mapWorkflowItemState` (`backendapp/boot_state_routes.go`) emits the same
fields for the first render. A projection
that omits a field would read as a cleared server value after in-app navigation,
and the next unrelated step Save would persist it (AC-UI-SETTINGS-MANUAL-SAVE-001.9).
The list API omits an empty prompt, so the projection maps an absent prompt to "".

## Reset and recovery

Reset restores the entire editor from the latest acknowledged baseline, including
mode and raw text, even when the family string itself did not change. It issues
no PATCH. A failed request leaves the complete draft registered and retryable;
it does not normalize live raw text or advance the saved baseline. Existing
route guard and failure presentation continue using the shared contributor.

## Jira configuration read publication

`apps/web/components/jira/jira-settings.tsx` owns the mounted workspace's Jira
configuration baseline and draft. The existing `getJiraConfig` producer maps
GET `/api/v1/jira/config?workspace_id=...` 204 responses to null. Initial/OAuth
loads seed the form; the actual 90-second background read updates configuration
and auth-health metadata without replacing the draft. Its 90-second interval
starts after the current form-seeding load settles, preventing background reads
from superseding initial or OAuth form initialization.

For AC-UI-SETTINGS-MANUAL-SAVE-001.4 and the existing Reset guarantee, reads
capture their mounted owner, request sequence, and acknowledged-write revision
before awaiting the API. Only the latest read from that live owner and revision
may publish. Successful Save and removal advance the revision before admitting
the canonical POST result or null baseline, fencing every read already started.
A failed mutation leaves this revision and the accepted baseline unchanged.
Reads started after an acknowledgement remain eligible, including fresh null
and auth-health results; transient background errors retain the current baseline.

Write acknowledgements retain their existing semantics: successful writes are
admitted independently of read sequence. An unchanged submitted form is seeded
through `configToForm`; edits made during Save remain intact and dirty. Reset
uses the latest accepted baseline. Read ownership ends on workspace replacement
or unmount, including StrictMode effect cleanup. This is local response
publication, with no server ordering, cross-tab guarantee, shared cache, retained
response history, backend/API change, or credential-policy change.

Desktop and phone reuse the existing Jira settings card and shared Save/Reset
surface with identical state semantics. The state-only correction introduces no
new responsive composition, navigation, scrolling, or touch interaction.

## Linear configuration read publication

`apps/web/components/linear/linear-settings.tsx` owns the mounted workspace's
config/health projection, separate acknowledged Reset baseline, and raw draft.
GET `/api/v1/linear/config` maps 204 to null; POST returns canonical configuration
without exposing the secret. Teams requests consume saved-secret/health state
and do not participate in config-read ordering.

Only the active workspace initial-load effect may finish loading. Locale
changes do not restart it. A native React effect event provides current-locale
error feedback without making translation identity a seeding dependency. A write ACK can fence its data while the effect
still completes loading; config-read sequence does not own loading lifetime. Block new Test probes while a
configuration mutation is pending; previously admitted probes retain their
existing behavior.

The domain hooks live in `hooks/domains/linear/use-linear-config-publication.ts`.
A bounded domain-local `useLinearConfigPublication` follows the existing Jira
owner/sequence pattern. Each initial or 90-second read captures the current
mounted owner and advances its sequence. Only that owner's latest read may
publish; cleanup invalidates ownership on workspace change, unmount, and
StrictMode replay. Initial loading starts the interval only after settlement,
so a slow initial response cannot lose form seeding to a background read.

Save and Remove capture their owner before awaiting transport. On successful
acknowledgement they advance its sequence before publishing the canonical
config or null, independently of intervening reads. Failure does not advance
publication. The initial read alone seeds config, baseline, and form; periodic
reads update only config/health. Save advances baseline and canonicalizes only
an unchanged submitted draft. Newer raw edits survive; Reset uses the accepted
baseline. Later reads, including null and changed health, remain eligible.

Confirmed Remove runs through the existing save coordinator `runExclusive`,
which excludes active saves and other exclusive operations before transport.
The Save contributor and Remove/confirmation controls observe route mutation
activity. Release on settlement, including failure; retain the existing draft
and feedback rules. Capture the form at Remove admission and clear it only
if unchanged at ACK; later edits remain dirty against the new null baseline.
This serializes same-route admission, not cross-tab or
server-side ordering.

Desktop and phone reuse the shipped Linear connection card and shared Save/Reset
surface, including the phone removal sheet. No composition or navigation changes,
shared revision framework, cache migration, backend/API changes, or credential
policy changes are needed. This repairs local publication, not server ordering.

## Desktop and phone

`/settings/preferences/terminal-editors` reuses the existing terminal cards and
fixed centered shared save surface. Phone users reach it from the settings index
as a dedicated preferences page, the shipped mobile settings exemplar. Both
viewports use the same parent editor and contributor, not separate save logic.
Keep the existing single settings-content scroll owner, safe-area-aware action
positioning, and primitives. This repair needs no new markup, overlay, responsive
branch, touch-sizing rule, or navigation. Desktop clicks and phone taps must both
leave Save available after an older ACK when raw custom text is newer.

## Persistence, security, and diagnostics

Drafts remain route-local and vanish on reload or discard. The existing backend
is the durable settings owner; there is no schema migration, new storage, or
change to authentication or authorization. Tests observe the real coordinator,
rendered action, store, and PATCH payloads; no new telemetry is needed.

## Implementation records

- [Linear connection freshness repair](../../../plans/linear-settings-refresh-ordering/plan.md)

- [Jira baseline freshness repair](../../../plans/jira-settings-refresh-ordering/plan.md)
- [Original rollout](../../../plans/settings-manual-save/plan.md)
- [Save-surface refinement](../../../plans/settings-save-action-redesign/plan.md)
- [Custom-font draft repair](../../../plans/preserve-custom-terminal-font/plan.md)
- [Workflow prompt hydration repair](../../../plans/workflow-store-prompt-hydration/plan.md)

This design fills the missing vertical design paired with the existing active
requirement. Its terminal-editor refinement was reviewed, implemented, and locally validated.
