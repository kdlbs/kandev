---
id: "01-restore-restart-action"
title: "Restore Feature Toggles restart action"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/feature-toggles.md"
---

# Task 01: Restore Feature Toggles restart action

## Acceptance

- The SPA route table renders a wrapper that reads restart capability through a
  system-domain hook.
- Capability loading is distinct from unsupported or unavailable results.
- Supported supervisors show the in-app restart action.
- Unsupported or unavailable capability results show manual guidance.
- Desktop and mobile Playwright coverage proves the supported action and mobile
  touch/viewport behavior.

## Verification

Use TDD: first prove the pending capability state shows no manual guidance, then
run the focused Feature Toggles Vitest suite. Run the desktop and mobile
Playwright specs against a rebuilt production bundle when E2E verification is
available.

Recorded verification: focused route, component, and capability-hook Vitest
suite — 3 files, 14 tests passed; changed-file ESLint — clean. Playwright E2E
was not run locally.

## Files

- `apps/web/src/settings-routes.tsx`
- `apps/web/components/settings/system/feature-toggles-route.tsx`
- `apps/web/components/settings/system/feature-toggles-settings.tsx`
- `apps/web/hooks/domains/system/use-restart-capability.ts`
- `apps/web/hooks/domains/system/use-restart-capability.test.ts`
- `apps/web/src/settings-routes.feature-toggles.test.tsx`
- `apps/web/e2e/tests/settings/feature-toggles-restart.spec.ts`
- `apps/web/e2e/tests/settings/mobile-feature-toggles-restart.spec.ts`

## Dependencies

None.

## Risks

- Keep the restart action disabled until capability detection resolves.
- Preserve backend authorization; the UI must not be treated as an access
  control.
