---
id: "02-update-plugin-shortcut-documentation"
title: "Update plugin shortcut documentation"
status: done
wave: 2
depends_on:
  - "01-relocate-plugin-shortcut-editor"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001
acceptance_criteria:
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.1
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.2
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.3
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.4
  - AC-PLUGINS-PLUGIN-SHORTCUT-SETTINGS-001.6
system_design:
  - ../../specs/plugins/system-design/plugin-shortcut-settings.md
---

# Task 02: Update Plugin Shortcut Documentation

## Summary

Reconcile public plugin explanation, authoring, manifest reference, and the
maintained frontend plugin contract with the implemented per-plugin shortcut
location. Clarify that plugin lifecycle/configuration remains install-wide and
administrator-owned while shortcut overrides are personal.

## In scope

- Replace plugin-specific references to the global Keyboard Shortcuts page
  with Settings > Plugins > the installed plugin.
- Explain the personal shortcut versus administrator configuration boundary.
- Keep manifest description and `registerKeybinding` reference text aligned.
- Validate the complete public docs set and internal links.

## Out of scope

- Changing the core shortcut guide or its screenshot.
- Adding a new public page, diagram, or screenshot.
- Restating manifest grammar or dispatch rules beyond the location correction.

## Acceptance

- Plugin users and authors are consistently directed to the installed
  plugin's detail page for shortcut overrides, and the permission distinction
  is explicit where plugin management is described.
- The maintained plugin API contract and manifest reference no longer claim
  that plugin binding descriptions or overrides live on the global Keyboard
  Shortcuts page.
- Public docs validation and whitespace checks pass without a new screenshot
  or navigation entry.

## Verification

```bash
rg -n "Settings > Plugins" docs/public/plugins.md docs/public/plugins-manifest.md docs/public/plugins-authoring.md docs/plans/plugins/PLUGIN-API.md
! rg -n 'Settings > (\*\*)?Keyboard Shortcuts' docs/public/plugins.md docs/public/plugins-manifest.md docs/public/plugins-authoring.md docs/plans/plugins/PLUGIN-API.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/public docs/plans/plugins/PLUGIN-API.md
```

## Files likely touched

- `docs/public/plugins.md`
- `docs/public/plugins-manifest.md`
- `docs/public/plugins-authoring.md`
- `docs/plans/plugins/PLUGIN-API.md`

## Dependencies

- Task 01 must establish the final rendered location and permission behavior.

## Risks

- General shortcut documentation still legitimately names the core Keyboard
  Shortcuts page; checks must stay scoped to plugin-owned references.

## Parallelism

`sequential`

## Inputs

- `docs/specs/plugins/requirements/plugin-shortcut-settings.md`
- `docs/specs/plugins/system-design/plugin-shortcut-settings.md`
- The completed Task 01 UI and E2E evidence.

## Results

- Updated the public plugin overview, manifest reference, authoring guide, and
  maintained frontend plugin API contract to point users to the installed
  plugin detail page and distinguish personal shortcuts from administrator
  configuration and lifecycle controls.
- Public documentation checks passed: scoped location/reference searches,
  `node --test scripts/validate-public-docs.test.mjs`,
  `node scripts/validate-public-docs.mjs`, and `git diff --check`.
