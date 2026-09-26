---
id: "01-live-sidebar-indicator"
title: "Restore the live sidebar indicator"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-RUNS-001.9
system_design:
  - ../../specs/office/system-design/automation-runs.md
---

# Task 01: Restore the Live Sidebar Indicator

## Summary

Make the visible Automations sidebar section discover scheduled and manual runs
that start after the section opens. Render the established running state as an
animated loader and preserve the row's accessible state, compact geometry, and
non-running health dots.

## In scope

- Add the component regression test first and capture its idle-to-running
  failure.
- Gate the established live-summary refresh on the section's visible state.
- Render and test the automation-scoped running loader.
- Extend the existing desktop automation-sidebar Playwright spec with the live
  transition.

## Out of scope

- Backend automation lifecycle events or API changes.
- Automation schedule or run-state changes.
- Mobile navigation or layout changes.
- Refactoring the agenda, detail view, or shared automation row derivation.

## Acceptance

- An idle automation already visible in the sidebar changes to the animated
  running indicator within the next live-refresh round trip, without a reload.
- The final open run disappearing restores the row's idle or paused dot, and
  folded/collapsed sidebar states issue no repeat summary requests.
- The animated indicator is decorative, has a stable test ID, and retains the
  localized Running state for assistive technology without changing row
  geometry.

## Verification

```bash
cd apps
pnpm --filter @kandev/web exec vitest run \
  components/app-sidebar/sections/automations-section.test.tsx \
  components/runs/use-live-refresh.test.ts
pnpm --filter @kandev/web run typecheck
pnpm --filter @kandev/web exec eslint \
  components/app-sidebar/sections/automations-section.tsx \
  components/app-sidebar/sections/automations-section.test.tsx

cd web
pnpm e2e:run --project chromium tests/automations-sidebar.spec.ts
```

## Files likely touched

- `apps/web/components/app-sidebar/sections/automations-section.tsx`
- `apps/web/components/app-sidebar/sections/automations-section.test.tsx`
- `apps/web/e2e/tests/automations-sidebar.spec.ts`

## Dependencies

None.

## Risks

- The live-response E2E wait can match initial hydration if it is armed before
  the section has completed its first authoritative summary read.
- Reusing `useLiveRefresh` must preserve its latest-callback ref behavior and
  cleanup when visibility changes.

## Parallelism

`sequential`

## Inputs

- `AC-OFFICE-AUTOMATION-RUNS-001.9` and the Automation Runs navigation design.
- Existing `useAutomationSummaries`, `useLiveRefresh`, and automation-sidebar
  test patterns.
- Existing task-switcher animated status treatment as the visual/accessibility
  precedent.

## Results

- Added the visible-section `useLiveRefresh` gate and reused the guarded summary refresh.
- Rendered an automation-scoped animated loader for running rows while preserving localized state text and non-running dots.
- Added component coverage for idle-to-running-to-idle transitions and desktop Chromium coverage for a seeded open run without navigation.
- Passed focused Vitest (22 tests), typecheck, targeted ESLint, Chromium E2E (4 tests), and specification lint.
- Fixup preserved the last successful summary after refresh errors and gated polling at the shared 768px mobile boundary; the added regressions passed with the focused suite at 30 tests, typecheck, targeted ESLint, and Chromium E2E (4 tests).
