---
status: active
system: integrations
created: 2026-08-06
owners:
  - platform
---
# Integration Enable/Disable Toggle & Nav Visibility Requirements

## Overview

Every third-party integration (Azure DevOps, GitHub, GitLab, Jira, Linear,
Sentry) can be connected with credentials, but only Jira, Linear and Sentry
currently expose a user-facing on/off switch for their UI surfaces. Azure
DevOps, GitHub and GitLab have no such switch. Users who want to temporarily
silence an integration (e.g. while its credentials are being rotated, or
because a workspace doesn't use it) have no consistent, discoverable way to do
that across all six integrations, and no way to control whether a
disabled-but-still-configured integration continues to clutter the left panel
navigation. (Slack was retired from the core app to an external plugin after
this spec was written and is out of scope.)

"Because a workspace doesn't use it" was one of the reasons above, but the
toggle first shipped install-wide: turning GitHub off for one workspace turned
it off for every workspace, while the credentials it gates stay per workspace.
That is corrected here, and the requirements below now describe the
per-workspace toggle.

## Requirements

### REQ-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001: Integration Enable/Disable Toggle & Nav Visibility

**Intent:** Every third-party integration (Azure DevOps, GitHub, GitLab, Jira, Linear, Sentry) can
be connected with credentials, but only Jira, Linear and Sentry currently expose a user-facing
on/off switch for their UI surfaces. Azure DevOps, GitHub and GitLab have no such switch. Users who
want to temporarily silence an integration (e.g. while its credentials are being rotated, or because
a workspace doesn't use it) have no consistent, discoverable way to do that across all six
integrations, and no way to control whether a disabled-but-still-configured integration continues to
clutter the left panel navigation. (Slack was retired from the core app to an external plugin after
this spec was written and is out of scope.) "Because a workspace doesn't use it" was one of the
reasons above, but the toggle first shipped install-wide: turning GitHub off for one workspace
turned it off for every workspace, while the credentials it gates stay per workspace. That is
corrected here, and the requirements below now describe the per-workspace toggle.

#### Acceptance criteria

- **AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.1:** Every integration (Azure DevOps, GitHub, GitLab, Jira, Linear, Sentry) SHALL expose an enable/disable slider on its own settings page (`/settings/integrations/<slug>`, and the per-workspace equivalent `/settings/workspaces/<id>/integrations/<slug>`). Jira, Linear and Sentry already have this; Azure DevOps, GitHub and GitLab gain it.
- **AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.2:** The same slider SHALL also appear on each integration's row/card on the integrations index page (`/settings/integrations` and its per-workspace equivalent), so a user can enable or disable any integration without opening its detail page.
- **AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.3:** The enabled state SHALL be scoped to a workspace. Everything else about an integration already is (credentials, health, and the settings page the slider lives on, `/settings/workspaces/<id>/integrations/<slug>`), so disabling an integration in one workspace MUST NOT disable it in any other. Every surface that reads the toggle reads it for the workspace it is showing: the sliders for the workspace whose settings page they are on, the left-panel nav and its "hide disabled" filter for the active workspace, and the Settings menu tree for each workspace it lists.
- **AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.4:** Toggling the slider in either location SHALL keep both locations in sync for that workspace (same underlying per-integration, per-workspace enabled state).
- **AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.5:** The enabled state for Azure DevOps, GitHub and GitLab is purely a presentation/navigation-visibility switch: it MUST NOT change whether their existing PR/work-item/board/MR features function. This mirrors that those three integrations have no existing UI surface gated on an "available" concept beyond navigation and their own settings page banner.
- **AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.6:** The integrations index page SHALL gain one new setting, **"Hide disabled integrations from left panel navigation"**, disabled (off) by default.
- **AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.7:** When that setting is **off** (default), a disabled-but-configured integration MUST still appear in the left panel navigation (the sidebar's Integrations section, its mobile-menu equivalent, and — whenever a tree menu mode renders it; the default flat menu lists no per-workspace rows at all — the Settings left panel's per-workspace Integrations list under Settings → Workspaces → <workspace>) exactly as it does today for an enabled integration — only credential/health status controls nav visibility.
- **AC-INTEGRATIONS-ENABLE-DISABLE-TOGGLE-001.8:** When that setting is **on**, a disabled integration MUST be hidden from all of those left-panel surfaces regardless of its credential/health status. An enabled, healthy integration is unaffected. Only integrations whose enable/disable toggle is off are hidden — an enabled-but-unconfigured integration stays listed (the Settings tree keeps its own configured-status badge convention).

## Migrated source detail

## What

- Every integration (Azure DevOps, GitHub, GitLab, Jira, Linear, Sentry)
  SHALL expose an enable/disable slider on its own settings page
  (`/settings/integrations/<slug>`, and the per-workspace equivalent
  `/settings/workspaces/<id>/integrations/<slug>`). Jira, Linear and Sentry
  already have this; Azure DevOps, GitHub and GitLab gain it.
- The same slider SHALL also appear on each integration's row/card on the
  integrations index page (`/settings/integrations` and its per-workspace
  equivalent), so a user can enable or disable any integration without
  opening its detail page.
- The enabled state SHALL be scoped to a workspace. Everything else about an
  integration already is (credentials, health, and the settings page the
  slider lives on, `/settings/workspaces/<id>/integrations/<slug>`), so
  disabling an integration in one workspace MUST NOT disable it in any other.
  Every surface that reads the toggle reads it for the workspace it is
  showing: the sliders for the workspace whose settings page they are on, the
  left-panel nav and its "hide disabled" filter for the active workspace, and
  the Settings menu tree for each workspace it lists.
- Toggling the slider in either location SHALL keep both locations in sync for
  that workspace (same underlying per-integration, per-workspace enabled
  state).
- The enabled state for Azure DevOps, GitHub and GitLab is purely a
  presentation/navigation-visibility switch: it MUST NOT change whether their
  existing PR/work-item/board/MR features function. This mirrors that those
  three integrations have no existing UI surface gated on an "available"
  concept beyond navigation and their own settings page banner.
- The integrations index page SHALL gain one new setting, **"Hide disabled
  integrations from left panel navigation"**, disabled (off) by default.
- When that setting is **off** (default), a disabled-but-configured
  integration MUST still appear in the left panel navigation (the sidebar's
  Integrations section, its mobile-menu equivalent, and — whenever a tree
  menu mode renders it; the default flat menu lists no per-workspace rows at
  all — the Settings left panel's per-workspace Integrations list under
  Settings → Workspaces → <workspace>) exactly as it does today for an
  enabled integration — only credential/health status controls nav
  visibility.
- When that setting is **on**, a disabled integration MUST be hidden from all
  of those left-panel surfaces regardless of its credential/health status. An
  enabled, healthy integration is unaffected. Only integrations whose
  enable/disable toggle is off are hidden — an enabled-but-unconfigured
  integration stays listed (the Settings tree keeps its own configured-status
  badge convention).
- The new setting SHALL NOT change any other behavior gated on an
  integration's existing "available" signal (e.g. Jira/Linear import
  popovers, Kanban external-link buttons, task-top-bar issue buttons) — those
  keep gating on enabled-and-authed exactly as they do today, independent of
  left-panel nav visibility.
- All new/changed toggles persist and sync the same way the existing
  Jira/Linear/Sentry "enabled" toggle does: a `localStorage`-backed,
  per-workspace boolean, synced across browser tabs, with a manual-save
  affordance consistent with the rest of the Settings UI (`data-settings-dirty`
  + the shared save bar) where the control lives on a settings page.

## Data model

No new persistent (backend/database) state. All state introduced by this
feature lives in the browser's `localStorage`, matching the existing pattern
in `hooks/domains/integrations/use-integration-enabled.ts`:

| Key | Type | Default | Notes |
|---|---|---|---|
| `kandev:<slug>:enabled:v1:<workspaceId>` | boolean | `true` | One per (integration, workspace), for all six slugs. |
| `kandev:integrations:hideDisabledInNav:v1` | boolean | `false` | Not per-integration and not per-workspace; one flag for the whole nav-filtering behavior. |

The unsuffixed `kandev:<slug>:enabled:v1` keys hold the value written while the
toggle was install-wide. They are never written again, and are read as the
default for a workspace that has never been toggled, so upgrading does not
silently re-enable an integration a user had turned off.

Keys from before that install-wide period (`kandev:<slug>:enabled:<workspaceId>:v1`)
were themselves per workspace. Each SHALL be restored to that workspace's own
key, never folded into the unsuffixed one: folding would let a single
workspace's stored preference answer for every other workspace, which is the
behavior this scoping exists to remove. A workspace that already has a value
keeps it, the legacy entry is then dropped either way, and a legacy key with no
recoverable workspace id is left untouched rather than guessed at. The scan
skips the current suffixed keys (they share the prefix) and runs at most once
per integration per page load, so a browser holding no legacy keys pays for one
pass rather than one per render.

## API surface

No backend/HTTP/WS changes. This is entirely a frontend feature.

Frontend primitives (new):

- `hooks/domains/azure-devops/use-azure-devops-enabled.ts` — exports
  `useAzureDevOpsEnabled(workspaceId?)`, a one-line wrapper over
  `useIntegrationEnabled(storageKey, legacyKeyPrefix, syncEvent, workspaceId)`,
  mirroring `hooks/domains/jira/use-jira-enabled.ts`. No legacy key prefix
  existed before, so the legacy-migration argument is inert (no prior keys to
  migrate) but kept for signature symmetry.
- `hooks/domains/github/use-github-enabled.ts` — same shape, for GitHub.
- `hooks/domains/gitlab/use-gitlab-enabled.ts` — same shape, for GitLab.
- `lib/integrations/integration-enabled-keys.ts` — the per-slug storage
  key / legacy prefix / sync event catalog, so the naming scheme is declared
  once and surfaces that must read several workspaces' toggles at a time have
  something to read through.
- `hooks/domains/integrations/use-integration-enabled.ts` also exports
  `readIntegrationEnabled` (an unsubscribed read for one workspace) and
  `useIntegrationEnabledReader` (the same read bound to a live subscription).
  The Settings menu tree lists every workspace and rules of hooks forbid
  calling `useXEnabled` per workspace, so it uses the reader.
- `components/integrations/integration-enabled-control-props.ts` — the props
  every slider takes. `workspaceId` is a required (though nullable) prop so a
  render site that forgets to name its workspace fails to compile.
- `hooks/domains/integrations/use-hide-disabled-integrations-in-nav.ts` —
  exports `useHideDisabledIntegrationsInNav()` returning
  `{ hideDisabled: boolean; setHideDisabled: (next: boolean) => void }`,
  backed by `localStorage` key `kandev:integrations:hideDisabledInNav:v1`
  (default `false`), using the same `useSyncExternalStore` +
  `storage`-event + custom-event-broadcast shape as
  `use-integration-enabled.ts` (custom event name
  `kandev:integrations:hide-disabled-in-nav-changed`), so nav consumers
  re-render immediately in the same tab, and other tabs update via the
  browser `storage` event.

Existing primitives reused unchanged:

- `hooks/domains/integrations/use-integration-enabled.ts` (`useIntegrationEnabled`)
- `hooks/domains/jira/use-jira-enabled.ts`, `use-linear-enabled.ts`,
  `use-sentry-enabled.ts`
- `hooks/domains/jira/use-jira-availability.ts`'s `useJiraAuthed` and
  `hooks/domains/linear/use-linear-availability.ts`'s `useLinearAuthed` —
  these already expose the "configured and healthy, independent of the
  enabled toggle" signal and are the building block for decoupled nav
  filtering.
- `components/integrations/use-drafted-integration-enabled.ts` +
  `components/integrations/drafted-integration-enabled-control.tsx` — the
  existing draft/save-wired slider component
  (`<DraftedIntegrationEnabledControl id={slug} enabled={enabled} persist={setEnabled} />`),
  reused as-is for Azure DevOps/GitHub/GitLab's own-page slider (new
  `<AzureDevOpsEnabledControl>`, `<GitHubEnabledControl>`,
  `<GitLabEnabledControl>` wrapper components, one per integration,
  mirroring `components/jira/jira-enabled-control.tsx`) and reused again for
  the index-page per-row sliders (all six integrations) and for the new
  "hide disabled in nav" row.

Modified:

- `hooks/use-nav-availability.ts`'s `useNavAvailability()` — see Scenarios
  below for the exact filtering contract. Its return shape
  (`AvailabilityMap`) is unchanged; only how each key's boolean is derived
  changes.

## Permissions

No change. The toggle is a per-browser-profile UI preference with no
authorization dimension; scoping it to a workspace is a scoping change, not an
access-control one.

## Failure modes

- `localStorage` unavailable or throwing (private browsing, quota) — the
  existing `useIntegrationEnabled` implementation already degrades to
  `enabled: true` and swallows write errors; the new hooks reuse this
  behavior. `useHideDisabledIntegrationsInNav` degrades to `hideDisabled:
  false` (its documented default) under the same failure, so a storage
  failure never hides an integration a user can't otherwise see or control.
- No integration ever hides from a *settings* page based on the new nav
  setting — `hideDisabled` affects only left-panel navigation surfaces
  (`useNavAvailability`'s output for the main sidebar and mobile menu, and
  the settings menu's visible-slug filter — `useVisibleIntegrationSlugs` in
  `use-settings-menu-branches.ts` feeding `integrationNodes` — for the
  Settings left panel's per-workspace Integrations branch), never a settings
  route's reachability.
  A user can always reach a disabled integration's own settings page
  directly (by URL, from the index page, or from the sidebar's Settings
  section) and re-enable it even while the left-panel nav entry is hidden.

## Persistence guarantees

State lives only in `localStorage`, keyed by workspace id; it is not part of
any backend restart or
task-execution durability guarantee, matching the existing enabled-toggle
convention. It survives a browser restart but not a `localStorage` clear, and
is not synced across devices/browsers.

## Scenarios

- **GIVEN** GitHub has valid, healthy credentials and the "hide disabled"
  setting is off (default), **WHEN** the user disables GitHub via either
  slider (own page or index-page row), **THEN** GitHub's sidebar Integrations
  entry and mobile-menu entry remain visible (still reachable), while the
  toggle itself reads "Disabled" on both the index-page row and the GitHub
  settings page.
- **GIVEN** GitHub has valid, healthy credentials and is disabled, **WHEN**
  the user turns on "Hide disabled integrations from left panel navigation"
  on the integrations index page, **THEN** GitHub's sidebar Integrations
  entry and mobile-menu entry disappear immediately (no reload required, no
  change to GitHub's health/configured status).
- **GIVEN** the "hide disabled" setting is on and GitHub is disabled and
  hidden from the nav, **WHEN** the user re-enables GitHub (either slider),
  **THEN** GitHub's sidebar entry reappears immediately.
- **GIVEN** the "hide disabled" setting is on, **WHEN** an integration is
  enabled but not configured/healthy (e.g. no credentials saved), **THEN**
  it stays hidden from the nav exactly as it does today — the new setting
  never *reveals* an unconfigured integration.
- **GIVEN** Jira is disabled and the "hide disabled" setting is on, **WHEN**
  the user opens the Jira import popover on a task (an existing surface
  gated on Jira's combined enabled+authed "available" signal), **THEN** the
  import popover remains hidden exactly as it does today when Jira is
  disabled — the new setting does not change that unrelated gate.
- **GIVEN** the integrations index page, **WHEN** it renders, **THEN** every
  one of the six integration rows/cards shows its own enable/disable
  slider reflecting that integration's current stored state, and toggling a
  slider does not navigate to that integration's detail page.
- **GIVEN** two workspaces and GitHub connected in both, **WHEN** the user
  disables GitHub from workspace A's integrations page, **THEN** workspace B's
  GitHub slider still reads "Enabled", and every surface gated on GitHub's
  enabled state (nav filtering while "hide disabled" is on, the Settings menu
  tree's per-workspace Integrations list) treats GitHub as enabled while B is
  the workspace in view.
- **GIVEN** a user who had disabled an integration while the toggle was
  install-wide, **WHEN** they upgrade to the per-workspace toggle, **THEN**
  every workspace still reads it as disabled until one of them toggles it,
  after which only that workspace's value changes.
- **GIVEN** the integrations index page and the Azure DevOps settings page
  open in two browser tabs, **WHEN** the user disables Azure DevOps on the
  index page in tab A, **THEN** tab B's Azure DevOps settings-page slider
  updates to "Disabled" after the next `storage` event (existing
  cross-tab-sync behavior, unchanged mechanism, extended to the three new
  integrations).

## Out of scope

- No backend/database changes; the toggle and the nav-filter setting are
  both pure frontend, `localStorage`-backed preferences.
- No change to Azure DevOps/GitHub/GitLab's PR, work-item, board, or MR
  features when disabled — the new toggle for these three integrations only
  controls settings-page/index-page slider state and left-panel nav
  visibility, not functional gating (unlike Jira/Linear/Sentry's
  existing toggle, which already gates other surfaces and is unchanged by
  this feature).
- No change to the Settings-page navigation tree's status-badge convention
  (`components/app-sidebar/sections/settings/workspaces-group.tsx`): its
  per-workspace Integrations list keeps the configured-status badge
  ("Enabled" = credentials/health present), which is unrelated to the
  enable/disable toggle. The "hide disabled" setting DOES filter that list
  (a disabled integration is hidden from it when the setting is on), since
  users reach it through left-panel navigation; the badge convention itself
  is unchanged.
- Sentry has no main-sidebar nav destination today (see
  `lib/navigation/core-destinations.ts` — only Azure DevOps, GitHub, GitLab,
  Jira and Linear are nav-gated), so the setting has no effect for Sentry in
  the main sidebar or mobile menu. Sentry DOES appear in the Settings left
  panel's per-workspace Integrations tree (all six integrations are always
  listed there), so with the setting on and Sentry's toggle off it is hidden
  from that tree like any other disabled integration; its existing slider
  (own page + new index-page row) still works for the enable/disable state
  itself.
- No new command-palette entries or keyboard shortcuts for the new toggles.

## Open questions

(none)
