---
id: "03-integrated-e2e"
title: "Integrated desktop and mobile coverage"
status: done
wave: 3
depends_on: ["01-backend-cached-overview", "02-frontend-settings-stability"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 03: Integrated desktop and mobile coverage

## Acceptance

- A page refresh and policy save reuse the current overview snapshot while manual Analyze replaces
  it and updates the displayed relative timestamp.
- The persisted external Go-cache path is visible after the Storage page reloads.
- Desktop and Pixel 5 flows remain usable without horizontal document overflow.

## Verification

```bash
cd apps/web && pnpm e2e:run --project chromium tests/system/storage-maintenance.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/system/mobile-storage-maintenance.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`
- existing Storage E2E helpers only if needed for deterministic provider-call observation

## Dependencies

Tasks 01 and 02.

## Inputs

- Spec cache, external Go-cache, policy editing, and mobile scenarios
- Existing Storage desktop/mobile E2E flows

## Output contract

Return a compact handoff capsule with acceptance status, exact E2E commands/results, risk tags,
failure artifacts when applicable, uncertainties, and set this task to `done`.
