---
id: "03-show-global-reload-alert"
title: "Show the global reload alert"
status: done
wave: 3
depends_on:
  - "02-detect-backend-generation"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.5
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.6
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.7
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.8
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.9
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.10
  - AC-PLATFORM-BACKEND-RESTART-PAGE-RECOVERY-001.12
system_design:
  - ../../specs/platform/system-design/backend-restart-page-recovery.md
---

# Task 03: Show the Global Reload Alert

## Summary

Render one persistent reload alert above all application route content. Connect
intentional restart owners, suppress duplicate settings errors, and prove the
flow on desktop and phone.

## In scope

- Mount the generation guard and alert in the shared status surface provider.
- Add the explicit current-document reload action.
- Preserve route use until the user chooses to reload.
- Register reload ownership in generic restart and self-update hooks.
- Suppress local agent settings errors for a handled stale-interlock response.
- Add all six locale values.
- Add component, hook, and real-restart E2E tests with TDD.

## Out of scope

- Automatic reload and draft persistence.
- Alerts outside the authenticated application shell.
- A new toast, modal, route, drawer, or mobile-only workflow.

## Acceptance

- Every authenticated route shows the same non-dismissible in-flow alert after
  a proven restart.
- Client-side navigation and repeated signals keep one alert.
- Intentional restart flows expose one reload-required surface.
- The action reloads only after user activation and keeps the current location.
- Phone layout provides a 44px action without overflow or covered navigation.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/app-status-bar/backend-reload-required-alert.test.tsx hooks/domains/system/use-kandev-restart.test.ts hooks/domains/system/use-self-update.test.ts
cd apps/web && pnpm run i18n:check && pnpm run typecheck
make build-backend build-web
cd apps/web && pnpm e2e:run --project=chromium tests/layout/backend-restart-page-recovery.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/layout/mobile-backend-restart-page-recovery.spec.ts
```

## Files likely touched

- `apps/web/components/app-status-bar/backend-reload-required-alert.tsx`
- `apps/web/components/app-status-bar/backend-reload-required-alert.test.tsx`
- `apps/web/components/app-status-bar/app-status-surface-provider.tsx`
- `apps/web/hooks/domains/system/use-kandev-restart.ts`
- `apps/web/hooks/domains/system/use-kandev-restart.test.ts`
- `apps/web/hooks/domains/system/use-self-update.ts`
- `apps/web/hooks/domains/system/use-self-update.test.ts`
- current agent settings error presenters
- `apps/web/src/locales/*/system.json`
- `apps/web/e2e/tests/layout/backend-restart-page-recovery.spec.ts`
- `apps/web/e2e/tests/layout/mobile-backend-restart-page-recovery.spec.ts`

## Dependencies

- Task 02 supplies the coordinator and restart signals.

## Risks

- A modal restart flow can hide a second in-flow action. Ownership must keep one
  reachable recovery surface.
- Agent settings use several local error presenters. Each protected mutation
  needs the exact handled-error predicate.
- The alert must stay above route data without creating another scroll owner.

## Parallelism

`sequential`

## Inputs

- Acceptance criteria 001.5 through 001.10 and 001.12.
- System-design sections `Recovery coordinator`, `Alert surface`, and
  `Responsive and accessible behavior`.
- Existing agent runtime alert and restart progress patterns.

## Results

Implemented the shared alert, explicit current-location reload action,
intentional restart and self-update ownership, localized copy, and desktop and
mobile real-restart coverage.

Verification passed:

- Alert and restart/self-update hook suites: 24 tests passed.
- `pnpm run lint`, `pnpm run typecheck`, and `pnpm run i18n:check`: passed.
- `make build-backend build-web`: passed.
- Managed `chromium` and `mobile-chrome` restart E2E suites: one test passed in
  each project.
