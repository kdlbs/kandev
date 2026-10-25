---
created: 2026-09-04
status: completed
requirements:
  - REQ-UI-SETTINGS-MENU-DEFAULT-001
system_design:
  - ../../specs/ui/system-design/settings-menu-default.md
legacy_specs: []
---

# Implementation Plan: Settings Menu Default

## Overview

Make Accordion tree the fallback Settings Menu Shape for devices without a
valid saved choice. Preserve explicit device-local choices, align localized
descriptions and code comments, and prove the behavior on desktop and phone.

## Scope

### In scope

- Change the device-local default and invalid-value fallback to Accordion tree.
- Preserve valid stored modes and the existing Appearance preview/save/discard
  flow.
- Update the five supported locale descriptions and stale test documentation.
- Add focused unit and desktop/mobile browser evidence.

### Out of scope

- Migrating existing local storage values.
- Changing the three menu mode implementations or their responsive geometry.
- Adding account, backend, or database persistence.

## Technical approach

- Update `DEFAULT_SETTINGS_MENU_MODE` in
  `apps/web/lib/settings/settings-menu-mode.ts`; the existing validated read
  and UI-slice initialization will carry the fallback through the application.
- Update `apps/web/lib/settings/settings-menu-mode.test.ts` for missing,
  malformed, and unsupported storage values.
- Update the Settings Menu Mode desktop spec, the phone Settings index proof,
  and comments in helpers/specs that currently describe Flat list as default.
- Move the default wording from `settingsMenuFlatDescription` to
  `settingsMenuAccordionDescription` in `en`, `pt-pt`, `zh-cn`, `zh-hk`, and
  `zh-tw` settings catalogs.

## Tests

- `apps/web/lib/settings/settings-menu-mode.test.ts` maps to
  `AC-UI-SETTINGS-MENU-DEFAULT-001.1`, `.2`, and `.3`.
- Existing settings tree tests continue to cover explicit mode behavior.

## E2E tests

- `apps/web/e2e/tests/settings/settings-menu-mode.spec.ts` maps to
  `AC-UI-SETTINGS-MENU-DEFAULT-001.1` and `.2` on desktop.
- `apps/web/e2e/tests/settings/mobile-settings-index.spec.ts` maps to
  `AC-UI-SETTINGS-MENU-DEFAULT-001.1` on phone and proves navigation remains
  usable.
- The localized card descriptions are covered by the existing Settings Menu
  Shape rendering path and catalog checks.

## Work orders

- [x] [Task 01: Make Accordion tree the default](task-01-accordion-default.md)

## Verification results

Completed.

- `pnpm --filter @kandev/web test -- --run lib/settings/settings-menu-mode.test.ts`: 8 tests passed.
- `pnpm --filter @kandev/web run typecheck`: passed.
- `pnpm --filter @kandev/web run lint`: passed.
- `pnpm --filter @kandev/web run i18n:check`: passed for all supported locales.
- `pnpm --filter @kandev/web run i18n:ratchet`: passed with 0 violations.
- `pnpm e2e:run --project chromium tests/settings/settings-menu-mode.spec.ts`: 5 tests passed.
- `pnpm e2e:run --project mobile-chrome tests/settings/mobile-settings-index.spec.ts`: 3 tests passed.
- `python3 scripts/lint-spec-files.test.py`: 30 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

## Risks

- Existing browser tests may rely on a clean device-local storage context and
  need explicit mode setup if they assert a particular branch shape.
- Locale catalog checks require all five supported translations to change
  together.
