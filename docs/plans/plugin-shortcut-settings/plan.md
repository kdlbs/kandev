---
created: 2026-09-04
status: complete
requirements:
  - REQ-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001
system_design:
  - ../../specs/plugins/system-design/plugin-shortcut-settings.md
legacy_specs: []
---

# Implementation Plan: Plugin Shortcut Settings

## Overview

Move dynamic plugin shortcut editors out of Preferences > Keyboard Shortcuts
and into each plugin's detail page while preserving the existing per-user
override map, shared Save behavior, full conflict comparison, and dispatch
rules. Implement the vertical frontend behavior and its desktop/member/mobile
evidence first, then reconcile the public plugin references with the shipped
location.

The plugin system owns this plan because manifest-declared keybindings and
their namespaced overrides are plugin contracts. The UI system supplies the
existing save coordinator and responsive settings shell without taking
ownership of the plugin behavior.

## Scope

### In scope

- Render Kandev-owned shortcuts only on the global keyboard-shortcuts page.
- Render one selected plugin's declared shortcuts on that plugin's detail
  route for members and administrators.
- Preserve namespaced portable overrides, draft/reset/clear semantics,
  conflict warnings, core precedence, and inactive-plugin behavior.
- Keep the phone flow within the existing direct plugin detail route, with
  touch-usable controls and no page-level horizontal overflow.
- Update user-facing plugin docs, the manifest reference, and the maintained
  frontend plugin API contract to name the new configuration location and
  permission split.

### Out of scope

- Backend, manifest-schema, plugin SDK, registry, or shortcut-dispatch changes.
- A new Settings route, drawer, plugin shortcut search index, or plugin-list
  shortcut badge/count.
- Changing access to plugin-authored settings, plugin config, or lifecycle
  actions.
- Cleaning up stored shortcut overrides during uninstall.
- Updating the core keyboard-shortcuts documentation screenshot, which remains
  accurate as a core-only settings view.

## Technical approach

### Core and plugin shortcut presentation

Keep `KeyboardShortcutsSettings` responsible for the chat-submit preference and
the Kandev shortcut draft. Continue loading plugin declarations there only so
core rows can report cross-source conflicts. Refine
`KeyboardShortcutsCard` so its rendered entries are always
`CONFIGURABLE_SHORTCUTS`, while its conflict input can still include every
plugin entry.

Add a plugin-focused shortcut settings component under
`apps/web/components/settings/plugins/`. It receives the selected plugin and
complete installed plugin list, filters visible entries to the selected owner,
and reuses the current recorder and conflict-label logic. Place it outside
`PluginDetail`'s `canManage` block, after operator configuration and before the
manifest card. Do not render it for a plugin without parseable keybindings.
When an active bundle replaces the exact plugin detail route, wrap its content
and append the same host-owned card so the supported route and keybinding
capabilities remain composable.

### Persistence and permissions

Register a route-local `plugin-shortcuts:{pluginId}` save contributor. Its
draft contains the existing complete `StoredShortcutOverrides` map and saves
through `updateUserSettings({ keyboard_shortcuts: ... })`; it never calls the
plugin configuration endpoint. Preserve the existing namespaced keys so no
migration or dispatcher change is needed. When an administrator changes both
plugin config and a personal shortcut, the shared coordinator owns partial
success, retry, discard, and navigation protection across the two contributors.

### Responsive composition

Reuse the existing direct phone path demonstrated by
`mobile-plugin-settings-row.spec.ts`: Settings navigation sheet > Plugins >
full plugin row > detail page. Keep the card inline rather than introducing a
drawer. On narrow/coarse-pointer layouts, let shortcut rows stack as needed,
give recorder/reset/clear controls a 44 px active dimension, retain the page's
single vertical scroll owner, and assert no document horizontal overflow.
Desktop keeps the denser inline row arrangement.

### Copy and documentation

Add localized, self-documenting copy explaining that plugin shortcuts are
personal and only run while the plugin is active. Update English, Portuguese,
Simplified Chinese, and generated Traditional Chinese catalogs, plus the
pseudo-locale through the existing scripts.

Update `docs/public/plugins.md`, `docs/public/plugins-manifest.md`, and
`docs/public/plugins-authoring.md`, whose dominant types remain explanation,
reference, and how-to respectively. Reconcile the maintained
`docs/plans/plugins/PLUGIN-API.md` contract comment. No public screenshot
currently shows plugin rows on the core page, so no image replacement is
required.

## Tests

- `apps/web/components/settings/keyboard-shortcuts-card.test.tsx` proves the
  global card renders core entries only while still reporting a conflict with
  a plugin binding.
- A focused plugin-shortcut card test under
  `apps/web/components/settings/plugins/` proves selected-owner rendering,
  no empty card, draft/reset behavior, and shortcut-only persistence without a
  plugin-config mutation.
- `apps/web/src/settings-routes.plugin.test.ts` proves an exact plugin detail
  route replacement retains both the plugin-owned content and host shortcut
  surface.
- `apps/web/lib/keyboard/plugin-shortcuts.test.ts` and
  `apps/web/lib/keyboard/shortcut-conflicts.test.ts` preserve entry identity,
  labels, defaults, and core/plugin comparison behavior.
- `apps/web/hooks/use-plugin-shortcuts.test.ts` remains targeted regression
  evidence that the relocated override still dispatches and core bindings win.

## E2E tests

- Extend `apps/web/e2e/tests/auth/plugin-settings-member.spec.ts` for
  `AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.1` through `.5`: install as an
  administrator, sign in as a member, prove the plugin recorder is absent from
  the global page and present on the plugin detail page while operator controls
  remain hidden, then save and reload the member's override.
- Extend
  `apps/web/e2e/tests/plugins/mobile-plugin-settings-row.spec.ts` for
  `AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.6` and `.7`: enter through the
  existing full-card tap after disabling the fixture plugin, edit and save its
  shortcut, reload, and verify touch geometry, internal vertical scrolling,
  and zero document horizontal overflow. Capture and restore the persisted
  shortcut baseline so the worker does not leak settings state.

Both scenarios must be written and observed failing for the old placement
before the production UI changes are made.

## Work orders

- [x] [Task 01: Relocate the plugin shortcut editor](task-01-relocate-plugin-shortcut-editor.md)
- [x] [Task 02: Update plugin shortcut documentation](task-02-update-plugin-shortcut-documentation.md)

## Verification results

- Task 01 targeted unit, typecheck, lint, i18n, E2E sleep-ratchet, authenticated
  member E2E, and mobile E2E checks passed.
- Task 02 scoped documentation searches, public-doc tests/validation, and
  whitespace checks passed.
- Review remediation removed unrelated generated locale churn and covered the
  exact-root plugin settings-route compatibility path.

## Risks

- The plugin detail page currently groups its configuration UI under an
  administrator gate; placing the new personal card inside that block would
  silently remove member access.
- Separating rendered lists can accidentally narrow conflict detection to the
  visible page and hide core-versus-plugin or plugin-versus-plugin collisions.
- Saving the complete override map from two independently mounted settings
  routes retains the existing last-write behavior; this change adds no
  concurrent-edit merge protocol.
- Mobile recorder rows currently use compact desktop geometry and need an
  explicit narrow/coarse-pointer composition to avoid clipped actions.
