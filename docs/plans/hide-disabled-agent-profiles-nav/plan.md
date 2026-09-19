---
spec: docs/specs/agents/requirements/hide-disabled-profiles-nav.md
created: 2026-08-11
status: complete
---

# Implementation Plan: Hide Disabled Agent Profiles from Left Panel Navigation

## Overview

Mirror the shipped integrations feature ("Hide disabled integrations from
left panel navigation", `docs/specs/integrations/requirements/enable-disable-toggle.md`)
for agent profiles: a single install-wide, `localStorage`-backed boolean,
toggled from the agents settings page, that filters the Settings left
panel's Agents tree. No backend, HTTP, or WS change.

Surface scoped by the spec: the Settings left panel's Agents group
(`components/app-sidebar/sections/settings/agents-group.tsx`) — the only
left-panel surface where `/settings/agents`-flavor profiles (the ones with
the `enabled` flag) appear. The office sidebar's Agents section is out of
scope (office rows have no enabled concept).

## Frontend

### 1. Hide-setting hook (mirrors the integrations hook)

- Add `apps/web/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav.ts`,
  exporting `useHideDisabledAgentProfilesInNav()` →
  `{ hideDisabled: boolean; setHideDisabled: (next: boolean) => void }`.
- Storage key `kandev:agents:hideDisabledInNav:v1`, default `false`,
  custom sync event `kandev:agents:hide-disabled-in-nav-changed`.
- Same `useSyncExternalStore` + `storage`-event + custom-event shape as
  `apps/web/hooks/domains/integrations/use-hide-disabled-integrations-in-nav.ts`:
  `readHideDisabled()` reads `localStorage.getItem(...) === "true"`,
  swallows read errors (default `false`), and `setHideDisabled` writes then
  dispatches the custom event, throwing on a failed write.

### 2. Settings-page toggle

- Add `apps/web/app/settings/agents/hide-disabled-agent-profiles-setting.tsx`,
  exporting `HideDisabledAgentProfilesSetting`: a bordered row with
  `<Label htmlFor="hide-disabled-agent-profiles-in-nav">` +
  description + `<Switch id="hide-disabled-agent-profiles-in-nav">`,
  checked from the hook and toggling `setHideDisabled` directly (immediate
  save — the agents page has no floating save bar; its profile toggles are
  immediate-save too). Rendered on the agents settings page; with zero
  profiles there is no profile list to hide from, so the row is absent until
  the first profile exists (recorded in the spec).
- Render it on `apps/web/app/settings/agents/page.tsx` directly below the
  page header separator and above the "Installed Agents" list, so the
  setting appears before the first agent profile on the page (the
  restructured page lists profiles inside the installed-agent cards; page
  already at its lint line budget; the component is a separate file imported
  by the page).
  separate file imported by the page).
- Locale keys (all four catalogs, `src/locales/{en,pseudo,pt-pt,zh-cn}/settings.json`):
  - `hideDisabledAgentProfilesFromNav` — "Hide disabled agent profiles from
    left panel navigation"
  - `hideDisabledAgentProfilesFromNavDescription` — "When on, a disabled
    agent profile is removed from the left panel navigation even if it's
    still configured."
  - No em dashes (U+2014); mirror each locale's existing
    `hideDisabledIntegrationsFromNav*` phrasing for pt-pt/zh-cn.

### 3. Settings-tree filter

- In `apps/web/components/app-sidebar/sections/settings/agents-group.tsx`,
  read `const { hideDisabled } = useHideDisabledAgentProfilesInNav();` and
  filter the flat-mapped profiles:
  `agent.profiles.filter((p) => !hideDisabled || (p.enabled ?? true))`.
- `enabled` absent → treated as enabled (never hidden). The `DisabledBadge`
  logic is untouched.
- `useAvailableAgents()` already hydrates `settingsAgents.items`; no store
  change.

## Tests

- **Hook unit tests** — `apps/web/hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav.test.ts`
  mirroring `use-hide-disabled-integrations-in-nav.test.ts` (uses the same
  `makeLocalStorageMock` helper): defaults to `false`; reads `"true"`
  literally; treats `"false"`/`"1"`/`"yes"`/`""` as off; `setHideDisabled`
  persists and updates state; custom-event propagation; `getItem` throwing →
  `false`; `setItem` throwing → throws.
- **Toggle component tests** — `apps/web/app/settings/agents/hide-disabled-agent-profiles-setting.test.tsx`
  (mock the hook): renders label/description and an off switch when
  `hideDisabled` is `false`; switch `checked` mirrors `hideDisabled`;
  `onCheckedChange` calls `setHideDisabled`.
- **Settings-tree filter tests** — extend the "SettingsTree agents group"
  describe in
  `apps/web/components/app-sidebar/sections/settings/settings-tree-render.test.tsx`
  (add a hoisted `hideAgentProfiles` mock for
  `useHideDisabledAgentProfilesInNav`, default `false`):
  - default off: disabled profile link still rendered (with "Disabled"
    badge) — existing test stays green;
  - on: disabled profile link gone; enabled and legacy (no `enabled` field)
    profiles still rendered;
  - on, then profile re-enabled (`enabled: true`): link rendered again.
- **E2E** — `apps/web/e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts`
  mirroring `e2e/tests/integrations/hide-disabled-integrations-nav.spec.ts`:
  - disable a seeded profile via `apiClient.updateAgentProfile(profile.id, { enabled: false })`;
  - `/settings/agents` → Settings left panel (`app-sidebar-settings-mode`)
    Agents tree still lists the disabled profile (setting default off);
  - toggle `#hide-disabled-agent-profiles-in-nav` on → tree entry
    disappears (no reload), enabled/legacy entries remain;
  - re-enable the profile via `apiClient` → tree entry reappears with the
    setting still on;
  - `finally` restore `{ enabled: true }`; the localStorage flag is
    per-test-context so it resets automatically.

## Verification commands

Per task, then once at the end (sequential; web only — no backend change):

```bash
cd apps && pnpm --filter @kandev/web test -- \
  hooks/domains/settings/use-hide-disabled-agent-profiles-in-nav.test.ts \
  app/settings/agents/hide-disabled-agent-profiles-setting.test.tsx \
  components/app-sidebar/sections/settings/settings-tree-render.test.tsx
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:ratchet && pnpm run i18n:check
```

E2E (task 03):

```bash
cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts
```

## Implementation Waves

Execution remains sequential in the primary conversation.

Wave 1:

- [x] [task-01-hide-setting-hook-and-toggle](task-01-hide-setting-hook-and-toggle.md)

Wave 2:

- [x] [task-02-settings-tree-filter](task-02-settings-tree-filter.md)

Wave 3:

- [x] [task-03-e2e](task-03-e2e.md)

Task 01 owns the hook contract the other two consume; task 02 filters the
tree; task 03 proves the user-facing flow end to end.

## Risks

- **Wrong surface.** The office sidebar's Agents section must NOT be
  filtered — office rows lack `enabled`. The hook is consumed only by
  `agents-group.tsx` (settings tree) and the settings-page toggle; do not
  import it into `agents-section.tsx`.
- **Legacy profiles.** `enabled === false` is the only "disabled" value;
  `undefined` must stay visible, or users with legacy profiles lose nav
  entries they cannot explain.
- **i18n ratchets.** The new component lives under `app/settings/agents/**`
  (whole-file i18n guard): all copy must go through `t()`, and all four
  catalogs need the two new keys or `i18n:parity`/`i18n:ratchet` fail.
- **Settings-tree accordion.** `/settings/agents` opens the agents group via
  the route-prefix accordion (`settingsOpenGroupIdForPath`), so the E2E can
  assert on expanded tree links without extra clicks; if the e2e navigates
  to a different settings route first, it must re-enter `/settings/agents`
  to reopen the group.

## Public documentation

None. The integrations equivalent ("hide disabled ... from left panel
navigation") is not documented in `docs/public`; this setting follows the
same precedent and needs no docs change.
