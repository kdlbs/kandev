---
id: "01-accordion-default"
title: "Make Accordion tree the default"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-MENU-DEFAULT-001
acceptance_criteria:
  - AC-UI-SETTINGS-MENU-DEFAULT-001.1
  - AC-UI-SETTINGS-MENU-DEFAULT-001.2
  - AC-UI-SETTINGS-MENU-DEFAULT-001.3
  - AC-UI-SETTINGS-MENU-DEFAULT-001.4
system_design:
  - ../../specs/ui/system-design/settings-menu-default.md
---

# Task 01: Make Accordion tree the default

## Summary

Change the Settings Menu Shape fallback from Flat list to Accordion tree for
devices without a valid saved choice. Keep explicit local choices intact,
align all supported locale descriptions, and update focused desktop and phone
coverage.

## In scope

- Change the default constant and its unit-test expectations.
- Update user-facing default descriptions in all supported locales.
- Update stale comments and existing Settings Menu Mode browser assertions.
- Verify the desktop and phone default paths.

## Out of scope

- Migrating existing local storage values.
- Changing branch expansion, navigation, save coordination, or phone layout.

## Acceptance

- A missing, malformed, or unsupported stored mode resolves to Accordion tree;
  valid stored modes still round-trip unchanged.
- The Settings Menu Shape card identifies Accordion tree as the default in all
  supported locales, and Flat list no longer claims that status.
- Desktop and phone focused E2E tests prove the fresh-device default and
  existing navigation behavior.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/settings/settings-menu-mode.test.ts
cd apps/web && pnpm e2e:run --project chromium tests/settings/settings-menu-mode.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-settings-index.spec.ts
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans
```

## Files likely touched

- `apps/web/lib/settings/settings-menu-mode.ts`
- `apps/web/lib/settings/settings-menu-mode.test.ts`
- `apps/web/src/locales/*/settings.json`
- `apps/web/e2e/tests/settings/settings-menu-mode.spec.ts`
- `apps/web/e2e/tests/settings/mobile-settings-index.spec.ts`
- `apps/web/e2e/helpers/settings-menu.ts`
- `docs/specs/ui/README.md`
- `docs/specs/ui/requirements/settings-menu-default.md`
- `docs/specs/ui/system-design/settings-menu-default.md`
- `docs/plans/settings-menu-default/plan.md`

## Dependencies

None.

## Risks

Tests that depend on a clean device-local mode may need explicit setup so they
remain independent of the new default.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/settings-menu-default.md`
- `docs/specs/ui/system-design/settings-menu-default.md`
- Existing Settings Menu Mode unit, component, and browser tests.

## Results

Completed.

- `pnpm --filter @kandev/web test -- --run lib/settings/settings-menu-mode.test.ts`: 8 tests passed.
- `pnpm e2e:run --project chromium tests/settings/settings-menu-mode.spec.ts`: 5 tests passed.
- `pnpm e2e:run --project mobile-chrome tests/settings/mobile-settings-index.spec.ts`: 3 tests passed.
- `pnpm --filter @kandev/web run typecheck`: passed.
- `pnpm --filter @kandev/web run lint`: passed.
- `pnpm --filter @kandev/web run i18n:check`: passed for all supported locales.
- `python3 scripts/lint-spec-files.test.py`: 30 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
