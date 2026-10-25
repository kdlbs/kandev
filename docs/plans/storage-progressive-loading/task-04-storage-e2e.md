---
id: "04-storage-e2e"
title: "Storage desktop and mobile E2E"
status: done
wave: 4
depends_on: ["03-storage-totals"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 04: Storage Desktop and Mobile E2E

## Acceptance

- Desktop E2E delays the scan-backed overview and proves policy, history, and quarantine render
  first, then verifies analysis and quarantine totals after the delayed response settles.
- Mobile E2E proves the same user value through Settings sheet → System → Storage on the existing
  one-column composition.
- The mobile state retains touch-reachable controls and has no horizontal document overflow.

## Verification

From `apps/`, install dependencies once if absent:

```bash
rtk pnpm install --frozen-lockfile
```

From `apps/web`:

```bash
rtk pnpm e2e:run tests/system/storage-maintenance.spec.ts
rtk pnpm e2e:run --project mobile-chrome tests/system/mobile-storage-maintenance.spec.ts
```

Confirm each command discovers the intended test count. The managed runner rebuilds the Go backend
and production Vite assets before executing and tears down its isolated runtime afterward.

## Files likely touched

- `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`
- Existing Storage component files only if stable `data-testid` selectors are missing

## Dependencies

Task 03.

## Parallelism

Sequential; this is integrated proof of Tasks 01–03.

## Inputs

- Spec: first-load, totals, failure isolation, and mobile scenarios
- Plan: E2E Tests and Mobile design contract
- Existing delayed-overview mobile test and Storage settings navigation flow

## Risks

- Route patterns must distinguish `GET /storage` from `GET /storage/settings`.
- E2E assertions must observe fast sections before releasing the overview gate, not merely after all
  requests have settled.

## Output contract

Report desktop/mobile flows, discovered test counts, exact command outcomes, artifact or teardown
evidence, blockers/risks, and update this task plus `plan.md` status in the same conversation.

## Results

- Added a desktop delayed-overview scenario that distinguishes `GET /api/v1/system/storage` from
  the lightweight settings route and verifies policy, history, and quarantine cards before the
  scan-backed overview is released. It then verifies both derived totals.
- Extended the mobile delayed-overview scenario with the same fast-section and totals assertions,
  while retaining the no-horizontal-overflow check.
- GREEN: `rtk pnpm e2e:run tests/system/storage-maintenance.spec.ts` — 6 tests discovered, 6 passed
  in 15.9s.
- GREEN: `rtk pnpm e2e:run --project mobile-chrome tests/system/mobile-storage-maintenance.spec.ts`
  — 4 tests discovered, 4 passed in 10.3s.
- Retained PR capture artifacts: `storage-maintenance--progressive-loading.png` and
  `mobile-storage-maintenance--progressive-loading.png`; the managed runners completed teardown
  after each capture.
