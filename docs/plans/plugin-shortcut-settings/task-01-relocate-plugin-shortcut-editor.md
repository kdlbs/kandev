---
id: "01-relocate-plugin-shortcut-editor"
title: "Relocate the plugin shortcut editor"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001
acceptance_criteria:
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.1
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.2
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.3
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.4
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.5
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.6
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.7
system_design:
  - ../../specs/plugins/system-design/plugin-shortcut-settings.md
---

# Task 01: Relocate the Plugin Shortcut Editor

## Summary

Move plugin-declared shortcut controls from the global core shortcut list to
the owning plugin's detail page. Preserve the existing per-user settings and
dispatch contracts, and prove the permission split plus desktop and phone
flows with tests written red before implementation.

## In scope

- Add failing component and Playwright assertions for the new placement,
  member access, persistence, inactive-plugin editing, and mobile geometry
  before changing production components.
- Reuse shortcut recorder and conflict logic across a core-only global card
  and a selected-plugin card.
- Register a personal plugin-shortcut save contributor outside the
  administrator-only plugin configuration boundary.
- Add localized explanatory copy and update all required catalogs.
- Preserve the existing dispatcher and namespaced override storage.

## Out of scope

- Backend, manifest, registry, SDK, and shortcut-dispatch changes.
- Plugin-authored settings access, lifecycle authorization, or Settings search.
- Public documentation changes, which belong to Task 02.

## Acceptance

- The global page renders only Kandev shortcuts, and a plugin detail page
  renders only that plugin's declared shortcuts for both members and
  administrators while preserving all operator gates.
- Recording, clearing, resetting, discarding, and saving use the existing
  namespaced portable setting; a shortcut-only save does not call plugin
  configuration or lifecycle APIs, and conflict warnings still use the full
  core-plus-plugin set.
- The authenticated desktop scenario and existing mobile plugin-detail
  scenario pass after proving the old placement fails; phone controls are
  touch-usable, persistence survives reload, and the document has no
  horizontal overflow.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- components/settings/keyboard-shortcuts-card.test.tsx components/settings/plugins/plugin-shortcuts-card.test.tsx lib/keyboard/plugin-shortcuts.test.ts lib/keyboard/shortcut-conflicts.test.ts hooks/use-plugin-shortcuts.test.ts src/settings-routes.plugin.test.ts
pnpm --filter @kandev/web typecheck
pnpm --filter @kandev/web lint -- components/settings/general-settings.tsx components/settings/keyboard-shortcuts-card.tsx components/settings/keyboard-shortcuts-card.test.tsx components/settings/plugins/plugin-detail.tsx components/settings/plugins/plugin-shortcuts-card.tsx components/settings/plugins/plugin-shortcuts-card.test.tsx lib/keyboard/plugin-shortcuts.ts lib/keyboard/plugin-shortcuts.test.ts src/settings-routes.tsx src/settings-routes.plugin.test.ts e2e/tests/auth/plugin-settings-member.spec.ts e2e/tests/plugins/mobile-plugin-settings-row.spec.ts
pnpm --filter @kandev/web i18n:check
pnpm --filter @kandev/web i18n:ratchet
pnpm --filter @kandev/web e2e:sleep-ratchet
cd web
pnpm e2e:run --project auth tests/auth/plugin-settings-member.spec.ts
pnpm e2e:run --project mobile-chrome tests/plugins/mobile-plugin-settings-row.spec.ts
```

## Files likely touched

- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/settings/keyboard-shortcuts-card.tsx`
- `apps/web/components/settings/keyboard-shortcuts-card.test.tsx`
- `apps/web/components/settings/plugins/plugin-detail.tsx`
- `apps/web/components/settings/plugins/plugin-shortcuts-card.tsx`
- `apps/web/components/settings/plugins/plugin-shortcuts-card.test.tsx`
- `apps/web/lib/keyboard/plugin-shortcuts.ts`
- `apps/web/lib/keyboard/plugin-shortcuts.test.ts`
- `apps/web/src/settings-routes.tsx`
- `apps/web/src/settings-routes.plugin.test.ts`
- `apps/web/e2e/tests/auth/plugin-settings-member.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-plugin-settings-row.spec.ts`
- `apps/web/src/locales/en/plugins.json`
- `apps/web/src/locales/pt-pt/plugins.json`
- `apps/web/src/locales/zh-cn/plugins.json`
- `apps/web/src/locales/zh-hk/plugins.json`
- `apps/web/src/locales/zh-tw/plugins.json`
- `apps/web/src/locales/pseudo/plugins.json`

## Dependencies

None.

## Risks

- Hook placement must remain unconditional when a plugin has no keybindings;
  hide only the rendered card, not hook execution.
- A full-map user-settings save can overwrite a newer concurrent shortcut
  edit unless local changes are rebased onto the newest authoritative map and
  stale responses are prevented from replacing higher store revisions.
- Auth and mobile Playwright projects use different fixtures and must be run
  separately so test discovery is not mistaken for passing coverage.

## Parallelism

`sequential`

## Inputs

- `docs/specs/plugins/requirements/plugin-shortcut-settings.md`
- `docs/specs/plugins/system-design/plugin-shortcut-settings.md`
- `apps/web/components/settings/general-settings.tsx`
- `apps/web/components/settings/keyboard-shortcuts-card.tsx`
- `apps/web/components/settings/plugins/plugin-detail.tsx`
- `apps/web/e2e/tests/auth/plugin-settings-member.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-plugin-settings-row.spec.ts`

## Results

- Added and observed the authenticated and mobile Playwright assertions fail
  against the old placement before implementing the production UI.
- Moved plugin shortcut rows to the selected plugin detail page, kept the
  global card core-only, and reused full core-plus-plugin conflict calculation.
- Added route-local personal shortcut persistence, reset/clear behavior, and
  touch-sized stacked rows without changing plugin configuration or dispatch.
- Kept the host shortcut card beside plugin-owned content when a bundle
  replaces its exact detail route.
- Added localized English, Portuguese, Simplified Chinese, Traditional
  Chinese, and pseudo-locale copy.
- Rebased local shortcut edits and deletions across initial hydration, later
  portable-settings updates, and in-flight saves; stale responses no longer
  replace a higher store revision.
- Hardened desktop and mobile persistence coverage to await the successful
  settings PATCH and assert the complete non-default binding after reload.
- Verification passed: targeted Vitest (6 files, 52 tests), web typecheck,
  targeted ESLint, `i18n:check`, `i18n:ratchet`, `e2e:sleep-ratchet`, the
  authenticated member E2E (1 passed), and the mobile plugin E2E (1 passed).
