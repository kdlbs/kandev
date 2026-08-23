---
status: active
system: agents
created: 2026-08-11
owners:
  - platform
---
# Hide Disabled Agent Profiles from Left Panel Navigation Requirements

## Overview

An agent profile can be disabled via the `/settings/agents` profile list
(`docs/specs/agents/requirements/profile-disable.md`) — the `enabled` toggle takes it out
of task/session creation while keeping it configured and editable. A disabled
profile still appears in the Settings left panel's Agents tree (under
Settings → Agents) with a "Disabled" badge. Users who disable several
profiles (retired models, temporarily broken agent CLIs) get a cluttered
navigation tree with no way to suppress those entries, exactly the problem
the integrations feature solved for disabled integrations
(`docs/specs/integrations/requirements/enable-disable-toggle.md`, setting "Hide disabled
integrations from left panel navigation"). This feature adds the analogous
setting for agent profiles.

## Requirements

### REQ-AGENTS-HIDE-DISABLED-PROFILES-NAV-001: Hide Disabled Agent Profiles from Left Panel Navigation

**Intent:** An agent profile can be disabled via the `/settings/agents` profile list
(`docs/specs/agents/requirements/profile-disable.md`) — the `enabled` toggle takes it out of task/session
creation while keeping it configured and editable. A disabled profile still appears in the Settings
left panel's Agents tree (under Settings → Agents) with a "Disabled" badge. Users who disable
several profiles (retired models, temporarily broken agent CLIs) get a cluttered navigation tree
with no way to suppress those entries, exactly the problem the integrations feature solved for
disabled integrations (`docs/specs/integrations/requirements/enable-disable-toggle.md`, setting "Hide disabled
integrations from left panel navigation"). This feature adds the analogous setting for agent
profiles.

#### Acceptance criteria

- **AC-AGENTS-HIDE-DISABLED-PROFILES-NAV-001.1:** The agents settings page (`/settings/agents`) SHALL gain one new setting, **"Hide disabled agent profiles from left panel navigation"**, off by default. It is a single, install-wide preference — not per profile.
- **AC-AGENTS-HIDE-DISABLED-PROFILES-NAV-001.2:** When that setting is **off** (default), a disabled profile MUST still appear in the Settings left panel's Agents tree exactly as it does today, with its "Disabled" badge — only the profile's own `enabled` state and reachability gate selection surfaces, not nav visibility.
- **AC-AGENTS-HIDE-DISABLED-PROFILES-NAV-001.3:** When that setting is **on**, a profile whose `enabled` is `false` MUST be hidden from the Settings left panel's Agents tree. An enabled profile, and a legacy profile whose `enabled` field is absent (treated as enabled), are unaffected and stay listed.
- **AC-AGENTS-HIDE-DISABLED-PROFILES-NAV-001.4:** The setting SHALL NOT change any other behavior gated on a profile's `enabled` state (task/session/session-handoff/quick-chat pickers, the "no compatible agent" empty states, pre-selection defaults). Those keep gating on `enabled !== false` exactly as they do today.
- **AC-AGENTS-HIDE-DISABLED-PROFILES-NAV-001.5:** The setting SHALL NOT hide anything from a settings *page* itself: the `/settings/agents` page keeps listing every profile, and the profile editor page (which owns the enable/disable toggle after the PageShell restructure) stays directly reachable by URL even while its left-panel nav entry is hidden.
- **AC-AGENTS-HIDE-DISABLED-PROFILES-NAV-001.6:** Toggling the setting SHALL take effect immediately (no reload, no save bar) and SHALL sync across browser tabs.
- **AC-AGENTS-HIDE-DISABLED-PROFILES-NAV-001.7:** **GIVEN** a profile with `enabled: false` and the "hide disabled" setting off (default), **WHEN** the user opens the Settings left panel's Agents tree, **THEN** the profile is listed with its "Disabled" badge.
- **AC-AGENTS-HIDE-DISABLED-PROFILES-NAV-001.8:** **GIVEN** a profile with `enabled: false`, **WHEN** the user turns on "Hide disabled agent profiles from left panel navigation" on `/settings/agents`, **THEN** the profile's entry disappears from the Settings left panel's Agents tree immediately (no reload required), while enabled profiles and legacy profiles without the flag stay listed.

## Migrated source detail

## Why

An agent profile can be disabled via the `/settings/agents` profile list
(`docs/specs/agents/requirements/profile-disable.md`) — the `enabled` toggle takes it out
of task/session creation while keeping it configured and editable. A disabled
profile still appears in the Settings left panel's Agents tree (under
Settings → Agents) with a "Disabled" badge. Users who disable several
profiles (retired models, temporarily broken agent CLIs) get a cluttered
navigation tree with no way to suppress those entries, exactly the problem
the integrations feature solved for disabled integrations
(`docs/specs/integrations/requirements/enable-disable-toggle.md`, setting "Hide disabled
integrations from left panel navigation"). This feature adds the analogous
setting for agent profiles.

## What

- The agents settings page (`/settings/agents`) SHALL gain one new setting,
  **"Hide disabled agent profiles from left panel navigation"**, off by
  default. It is a single, install-wide preference — not per profile.
- When that setting is **off** (default), a disabled profile MUST still
  appear in the Settings left panel's Agents tree exactly as it does today,
  with its "Disabled" badge — only the profile's own `enabled` state and
  reachability gate selection surfaces, not nav visibility.
- When that setting is **on**, a profile whose `enabled` is `false` MUST be
  hidden from the Settings left panel's Agents tree. An enabled profile, and
  a legacy profile whose `enabled` field is absent (treated as enabled), are
  unaffected and stay listed.
- The setting SHALL NOT change any other behavior gated on a profile's
  `enabled` state (task/session/session-handoff/quick-chat pickers, the
  "no compatible agent" empty states, pre-selection defaults). Those keep
  gating on `enabled !== false` exactly as they do today.
- The setting SHALL NOT hide anything from a settings *page* itself: the
  `/settings/agents` page keeps listing every profile, and the profile
  editor page (which owns the enable/disable toggle after the PageShell
  restructure) stays directly reachable by URL even while its left-panel nav
  entry is hidden.
- Toggling the setting SHALL take effect immediately (no reload, no save
  bar) and SHALL sync across browser tabs.

## Data model

No new persistent (backend/database) state. The setting lives in the
browser's `localStorage`, matching
`hooks/domains/integrations/use-hide-disabled-integrations-in-nav.ts`:

| Key | Type | Default | Notes |
|---|---|---|---|
| `kandev:agents:hideDisabledInNav:v1` | boolean | `false` | Not per-profile; one flag for the whole nav-filtering behavior. |

The per-profile `enabled` state is the existing
`agent_profiles.enabled` column (SQLite), exposed on every profile in the
`GET /api/v1/agents` / `GET /api/v1/agent-profiles/:id` responses and
normalized to `enabled?: boolean` on the canonical frontend `AgentProfile`
(`apps/web/lib/types/agent-profile.ts`). Absent on legacy payloads — treat
as enabled.

## API surface

No backend/HTTP/WS changes. This is entirely a frontend feature.

Frontend primitives (new):

- `hooks/use-local-storage-boolean.ts` — the shared
  `useLocalStorageBoolean(storageKey, syncEvent, defaultValue = false)`
  primitive: install-wide, `localStorage`-backed boolean with the
  `useSyncExternalStore` + `storage`-event + custom-event-broadcast shape
  both nav-visibility toggles use. Absent keys (and read failures) resolve
  to `defaultValue` on both the client snapshot and the server snapshot, so
  SSR and first client paint agree; a failed write throws.
- `hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav.ts` —
  exports `useHideDisabledAgentProfilesInNav()` returning
  `{ hideDisabled: boolean; setHideDisabled: (next: boolean) => void }`,
  backed by `localStorage` key `kandev:agents:hideDisabledInNav:v1`
  (default `false`), delegating to the shared primitive with custom event
  name `kandev:agents:hide-disabled-in-nav-changed`, so the settings tree
  re-renders immediately in the same tab and other tabs update via the
  browser `storage` event. The integrations sibling
  (`useHideDisabledIntegrationsInNav`) delegates to the same primitive with
  its own key/event, so the two features cannot drift in mechanism.
- `app/settings/agents/hide-disabled-agent-profiles-setting.tsx` — the
  settings-page row (label + description + `<Switch
  id="hide-disabled-agent-profiles-in-nav">`), rendered on the agents
  settings page directly below the header separator and above the
  "Installed Agents" list (the restructured page's profile list lives inside
  the installed-agent cards), so the setting appears before the first agent
  profile on the page.
  The switch saves immediately on toggle (the agents page has no
  settings-floating-save bar; its profile enabled toggles are
  immediate-save, and this setting follows that page convention).

Modified:

- `components/app-sidebar/sections/settings/settings-menu-branches.ts` —
  the Settings left panel's Agents branch (`buildAgentsBranch`) filters its
  profile children: when `hideDisabled` is on, a profile with
  `enabled === false` is omitted. Profiles with `enabled` absent/`true` are
  always listed. The existing "disabled" badge convention is unchanged (it
  still renders on any disabled profile that is listed, i.e. when the
  setting is off).
- `components/app-sidebar/sections/settings/use-settings-menu-branches.ts` —
  the live branch builder reads `useHideDisabledAgentProfilesInNav()` and
  threads the flag into `buildAgentsBranch`, mirroring how
  `useVisibleIntegrationSlugs` gates the integrations branch (both skip
  filtering entirely on the default path).

## Permissions

No change. The setting is a per-browser-profile, install-wide UI preference
with no authorization dimension (same as the integrations nav-hiding
setting).

## Failure modes

- `localStorage` unavailable or throwing (private browsing, quota) — the
  hook degrades to `hideDisabled: false` (its documented default), so a
  storage failure never hides a profile the user cannot otherwise see or
  control. `setHideDisabled` throws on a failed write instead of reporting
  a successful save, mirroring the integrations hook.
- A profile with `enabled` absent (legacy payload) is never hidden —
  absence means enabled, consistent with
  `lib/state/slices/settings/types.ts`'s `isSelectableAgentProfile` and the
  normalizer default in `lib/api/domains/agent-profile-normalize.ts`.

## Persistence guarantees

State lives only in `localStorage`; it is not part of any backend restart or
task-execution durability guarantee, matching the existing integrations
nav-setting convention. It survives a browser restart but not a
`localStorage` clear, and is not synced across devices/browsers.

## Scenarios

- **GIVEN** a profile with `enabled: false` and the "hide disabled" setting
  off (default), **WHEN** the user opens the Settings left panel's Agents
  tree, **THEN** the profile is listed with its "Disabled" badge.
- **GIVEN** a profile with `enabled: false`, **WHEN** the user turns on
  "Hide disabled agent profiles from left panel navigation" on
  `/settings/agents`, **THEN** the profile's entry disappears from the
  Settings left panel's Agents tree immediately (no reload required), while
  enabled profiles and legacy profiles without the flag stay listed.
- **GIVEN** the "hide disabled" setting is on and a profile is hidden,
  **WHEN** the user re-enables the profile via the profile editor header
  toggle (`/settings/agents/<agent>/profiles/<id>`), **THEN** the profile's
  nav entry reappears immediately.
- **GIVEN** the "hide disabled" setting is on, **WHEN** the user opens
  `/settings/agents` (the page itself), **THEN** every profile — disabled or
  not — is still listed, and the profile editor (which owns the
  enable/disable toggle) remains reachable by URL.
- **GIVEN** the "hide disabled" setting is on and a profile is disabled,
  **WHEN** the user opens the new task / new session / handoff / quick-chat
  selectors, **THEN** the disabled profile stays excluded exactly as it is
  today — the setting does not change selection gating.
- **GIVEN** the "hide disabled" setting is on, **WHEN** the user toggles it
  off again on `/settings/agents`, **THEN** disabled profiles reappear in the
  Settings left panel's Agents tree immediately.

## Out of scope

- The main (office) sidebar's Agents section
  (`components/app-sidebar/sections/agents-section.tsx`). Office agents are
  workspace-scoped rows with their own `status`/pause semantics and no
  user-facing enable/disable toggle; the `enabled` flag is a
  `/settings/agents` profile-flavor concept (see
  `docs/specs/agents/requirements/profile-disable.md`'s Out of scope). This setting does
  not touch office rows.
- The mobile menu — it has no agent-profile surface today.
- No change to profile selection gating, pickers, empty states, or
  pre-selection defaults (unchanged by this setting).
- No change to the `DisabledBadge` convention itself; it keeps rendering on
  any disabled profile that is listed.
- No new command-palette entries or keyboard shortcuts.

## Open questions

(none)
