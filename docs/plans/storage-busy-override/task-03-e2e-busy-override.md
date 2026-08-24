---
id: "03-e2e-busy-override"
title: "Desktop and mobile override coverage"
status: done
wave: 3
depends_on: ["01-backend-activity-api", "02-frontend-busy-override"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 03: Desktop and Mobile Override Coverage

## Acceptance

- Desktop E2E proves a running task yields labeled busy feedback, then **Run anyway** starts the
  normal cleanup job with `force: true`.
- Pixel 5 E2E proves the same action is visible and tappable in the Storage page’s one-column flow,
  completes the user outcome, and leaves document horizontal overflow at zero.
- The maintenance-running case remains unbypassable when practical to cover at the public route;
  otherwise the backend contract test is named as its coverage.

## Verification

```bash
cd apps/web && pnpm e2e:run --project chromium tests/system/storage-maintenance.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-storage-maintenance.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`
- `apps/web/e2e/helpers/storage-maintenance.ts` only if a deterministic activity fixture is needed

## Dependencies

Tasks 01 and 02.

## Inputs

- Spec: busy-override scenarios
- Plan: E2E Tests and mobile contract
- Existing active-task busy scenario in `tests/system/storage-maintenance.spec.ts`

## Output contract

Report acceptance status, exact E2E command results, screenshots/traces on failure, risks, and
update this task plus `plan.md` to `done` in the same conversation.

## Validation Results

- Desktop Storage E2E: passed (4 tests in the focused suite).
- Pixel 5 Storage E2E: passed (3 tests in the focused suite).
