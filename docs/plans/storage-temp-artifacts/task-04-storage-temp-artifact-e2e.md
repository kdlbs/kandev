---
id: "04-storage-temp-artifact-e2e"
title: "Temporary-artifact Storage E2E coverage"
status: done
wave: 4
depends_on: ["03-storage-temp-artifact-ui"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 04: Temporary-artifact Storage E2E coverage

Prove the user-facing desktop and mobile behavior through the existing Storage E2E suites. Use the
current API-route fixture pattern for the UI contract; backend filesystem/provider safety is covered
by Task 02's real SQLite/provider tests.

## Acceptance

- Desktop coverage shows a registered stale artifact in analysis, ignores an unregistered `kandev-*`
  folder, confirms the scoped action with `{ resources: ["temporary_artifacts"] }`, and renders the
  resulting quarantine/job feedback without changing the global Run-now contract.
- Mobile coverage opens the row and confirmation from the settings sheet, completes the same scoped
  action through touch interaction, and proves `document.documentElement.scrollWidth <= innerWidth`.
- Existing Storage desktop/mobile scenarios (Go-cache explicit cleanup, busy override, progressive
  loading, settings persistence) remain green.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web e2e:run tests/system/storage-maintenance.spec.ts
pnpm --filter @kandev/web e2e:run tests/system/mobile-storage-maintenance.spec.ts -- --project=mobile-chrome
```

## Files likely touched

- `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`
- `apps/web/e2e/helpers/storage-maintenance.ts` (only if shared response/fixture seeding is needed)

## Dependencies

Task 03.

## Parallelism

`sequential`; E2E depends on the final API shapes and UI test IDs.

## Inputs

- Spec Scenarios: temporary-artifact analysis, explicit cleanup, unregistered-path safety, and
  mobile Storage behavior.
- Existing storage-maintenance and mobile-storage-maintenance specs/helpers.

## Output contract

Report exact desktop/mobile test counts and commands, request payload assertions, quarantine/job
feedback, overflow result, cleanup of any capture/fixture artifacts, and synchronized task/plan
status.

## Results

- Desktop production E2E passed: 7 tests, including the registered stale row, unregistered-folder
  exclusion, exact scoped cleanup payload, quarantine/job feedback, and existing Storage flows.
- Mobile production E2E passed: 5 tests, including touch interaction, the scoped cleanup flow, and
  `document.documentElement.scrollWidth <= innerWidth`.
- The managed fixture avoids backend overview-cache contamination between the seeded temporary
  artifact scenario and the existing Go-cache scenario.
