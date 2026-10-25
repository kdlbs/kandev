# MCP UX consistency review

Reviewed on 2026-10-12: current working tree over
`ffa6d403802250dc8ff9246fe0f827e68d28c519`, including the untracked follow-up
implementation. This is a local UX review, not a verdict on the current GitHub
PR head or on backend security. Existing user edits remain intact.

Scope: workspace catalog, definition editor and marketplace; profile,
repository, task-creation and new-session selectors; composer Connections,
Customize, hover summaries and pending changes. Read the existing scope and
lifecycle requirements, relevant frontend tests, source and locale copy.

## Corrected suggestions

| Before | After |
| --- | --- |
| [Workspace navigation](../../../apps/web/lib/settings/workspace-settings-tabs.ts:60) and [catalog heading](../../../apps/web/components/settings/workspaces/mcp-settings.tsx:357) repeated “Workspace MCP servers” inside an already named workspace. | Navigation, heading and breadcrumb use the shared “MCP servers” label, also used by the pickers. The breadcrumb no longer derives untranslated “Mcp Servers” from the URL and now includes the owning workspace, matching other tabs. |
| [Workspace-profile card](../../../apps/web/app/settings/agents/[agentId]/profile-mcp-selection-card.tsx:169) used “MCP Configuration” plus “Workspace selection,” while other selectors said “MCP servers.” Its help referred to a workspace “context.” | Shared “MCP servers” title; redundant migration badge removed. Help explains choosing the workspace whose servers the profile uses. |
| [Generic picker](../../../apps/web/components/mcp/mcp-selection-picker.tsx:217) said “This scope” and used the same vague description for profiles and repositories. | Shared “Additional servers” section. Profile/repository descriptions name the target and explain where the choice applies. |
| [Catalog/setup help](../../../apps/web/src/locales/en/settings.json:465) exposed “materialized lazily,” executor internals and duplicate endpoint-probing explanations. | Short catalog introduction. Setup modes explain installation timing, executable prerequisites and save-versus-connect behavior in plain language. Exact-version and secret constraints remain visible. |
| [Marketplace feedback](../../../apps/web/src/locales/en/settings.json:519) used “degraded,” “last-good cache,” “Active” and “Save setup,” which blur discovery, runtime status and adding a reusable definition. | Previously saved results, “Available” listing status and the shared “Add MCP server” action. Connection status and server startup semantics are unchanged. |
| [Picker empty state](../../../apps/web/src/locales/en/settings.json:543) implied search excluded disabled definitions, although disabled definitions remain visible in the list. | “No MCP servers match this search.” Disabled rows retain their explicit disabled badge and control. |
| [Selection impact](../../../apps/web/components/settings/workspaces/mcp-settings.tsx:57) rendered “1 profiles,” “1 repositories” and “1 tasks.” | Each count uses localized singular/plural forms before composing the sentence. Zero selections retain the existing separate message. |
| [Catalog actions](../../../apps/web/components/settings/workspaces/mcp-settings.tsx:32), [marketplace actions](../../../apps/web/components/settings/workspaces/mcp-marketplace.tsx:79) and the setup back button mixed 32px/44px desktop overrides with native 28px fields. | Shared control sizing: 28px ordinary desktop actions and at least 44px on phones/coarse pointers. Content-sized setup choices and the intentionally 44px repository disclosure are retained. |

All new/changed copy is localized in seven shipped languages and pseudo.
Traditional Chinese was generated from Simplified Chinese; unrelated generator
changes were restored from the exact pre-edit locale snapshot. Removed unused
keys only after checking all call sites. Public docs use the actual button names
and distinguish adding a definition from installing/starting a server.

## Functional finding: global profile catalog picker is absent

**Delivery disposition (2026-10-19):** the user deferred this UI to unstarted
follow-up `ee292a1d-3d53-4b86-965c-6fe4a8ae217e`. The reproduction below remains
valid; this PR does not claim to complete the global-profile picker. Existing
workspace-scoped global-profile association/resolution contracts remain intact.

[Profile MCP routing](../../../apps/web/app/settings/agents/[agentId]/profile-mcp-config-card.tsx:450)
returns the catalog picker only when a saved profile has `workspaceId` supplied.
[The profile page](../../../apps/web/components/settings/agent-profile-page.tsx:314)
supplies the agent's workspace ID; built-in/global agents have none. Their
saved profiles therefore still show the legacy JSON editor and no workspace
catalog selector, while workspace-owned profiles get the new picker.

Reproduced without editing any data in the owned preview: the global Mock
profile page displayed “MCP Configuration” and “MCP servers (JSON),” with zero
`profile-mcp-selection-<id>` elements. Evidence is recorded in
`/tmp/kandev-pr3293-test-runtime/ux-consistency-global-profile.json` and its
screenshot. An initial diagnostic used the agent UUID instead of the route's
agent-name slug; after using the real route, the gap reproduced.

This leaves AC-003.1/.2 and AC-006.2's global profile selection UI incomplete.
Route saved MCP-capable global profiles to the shared selector with its workspace
picker, while retaining the legacy import/fallback contract. Before changing
routing, verify org/workspace permissions and add global-profile workspace A/B,
reload, save rejection and legacy-upgrade/fallback coverage. This functional
remediation is separate from the cosmetic corrections made in this pass.

## Repository and picker follow-up

The [2026-10-19 repair package](../mcp-picker-mobile-repair/plan.md) removes the
nested repository disclosure: its existing section directly renders shared
selection content. Repository, task-create, profile and new-session choices
reuse the description and mode/transport/source metadata used in Customize.
Mobile Connections rows use content height and a centered header with a
separate close target. The package records targeted verification and deployment.

The different save models are intentional: profile/repository/catalog settings
use shared Save changes; task-session changes save automatically and apply at
the existing idle/next-start gate. Do not unify those by adding another Apply
button. Keep configuration status separate from connection/tool evidence and
keep named inheritance sources visible. Inherited disabling remains deferred.

## Verification

The heading regression failed against the old production bundle (RED).
Formatting, scoped ESLint, TypeScript, all locale checks and the production web
build passed after the changes. Public-doc checks passed all 62 tests and 47
pages. All four desktop catalog/CRUD/marketplace/error/delete journeys and both phone
setup/marketplace journeys passed sequentially with one worker. They verify
header/navigation labels, 28px desktop actions, 44px phone controls and corrected
singular counts. Translation generation also rewrote a byte-identical backend
pseudo-locale, so the runtime freshness guard required rebuilding the native
host artifacts; the tests used freshly rebuilt artifacts rather than bypassing
that guard. Owned-preview desktop dark/light, phone, narrow fine-pointer and coarse-tablet
checks passed with consistent headings, 28/44px controls, readable setup hints
and no document horizontal overflow. Screenshots were inspected. That inspection
caught the remaining breadcrumb spelling/translation fallback. Its focused
regression failed before the correction; all 68 breadcrumb/route unit tests,
scoped lint, fresh web build and the final desktop catalog regression passed.
The final breadcrumb rendering was rechecked on all five viewport/pointer combinations.
The final preview assets retain prior hashed files and preserve all saved
selection tables and backend/proxy/main process identities. No commit or push
was performed. Evidence is in `/tmp/kandev-pr3293-test-runtime/ux-consistency-*`.

Verdict for this scoped review: wording/control suggestions corrected; global
profile selection coverage remains a follow-up.
This is not a merge-readiness verdict for the complete PR.
