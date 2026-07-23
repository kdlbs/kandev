---
id: "03-responsive-e2e"
title: "Prove desktop and mobile flows"
status: done
wave: 3
depends_on: ["01-persist-preference", "02-settings-and-rendering"]
plan: "plan.md"
spec: "../../specs/ui/app-status-bar.md"
---

# Task 03: Prove desktop and mobile flows

## Acceptance

- Desktop E2E proves the user can save simplified metrics and see icon/value-only status metrics after reload.
- Mobile E2E proves the same saved choice through the existing Settings and Status drawer composition.
- Tests restore the backend-owned user setting and assert relevant mobile containment/overflow behavior.

## Files likely touched

- `apps/web/e2e/tests/settings/resource-metrics-display.spec.ts`
- `apps/web/e2e/tests/settings/mobile-resource-metrics-display.spec.ts`
- Existing page objects/helpers only if a genuinely reusable settings/status action is missing.

## Inputs

- Spec: detailed/simplified and phone Status scenarios.
- Plan: **E2E Tests** and **Mobile design contract**.
- Patterns: `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`, `apps/web/e2e/tests/layout/app-status-bar.spec.ts`, and `apps/web/e2e/tests/plugins/mobile-status-drawer.spec.ts`.

## TDD sequence

1. Write the desktop and `mobile-*.spec.ts` scenarios and confirm they fail because the preference/control is absent.
2. Run against the integrated implementation and correct only selectors or genuine implementation gaps.
3. Confirm both tests pass against a freshly built production Vite bundle served by the Go backend.

## Verification

- `rtk pnpm e2e:run tests/settings/resource-metrics-display.spec.ts tests/settings/mobile-resource-metrics-display.spec.ts` from `apps/web`

## Dependencies

- `01-persist-preference`
- `02-settings-and-rendering`

## Output contract

Return a compact handoff capsule containing intent/acceptance, base/head SHA, changed files, named spec sections, `user-flow`, `persistence`, and `integration` risk tags, exact command/result, screenshot or rendered evidence paths when available, uncertainties, and this task file updated to `done`. Do not edit `plan.md`.
