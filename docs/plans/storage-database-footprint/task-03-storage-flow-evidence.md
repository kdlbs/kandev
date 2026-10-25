---
id: "03-storage-flow-evidence"
title: "Storage flow evidence"
status: completed
wave: 3
depends_on: ["02-storage-resource-rows"]
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.1
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.2
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.3
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.4
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.5
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.6
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.7
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.8
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002.9
system_design:
  - ../../specs/system-page/system-design/storage-database-footprint.md
---

# Task 03: Storage flow evidence

## Summary

Prove the new rows through desktop and phone flows, and document the measurement boundary for operators.

## In scope

- Desktop and mobile flows expand both rows and verify known backup bytes and total contribution, including long-path containment.
- A real isolated backend Analyze refresh observes an added disposable backup; controlled response fixtures cover unavailable and not-applicable rendering.
- Public operations documentation explains database sidecars, backup scope, caching, and the difference from whole-filesystem capacity.

## Out of scope

New cleanup behavior, retention changes, remote database measurement, and unrelated storage categories.

## Acceptance

- Desktop and mobile flows expand both rows and verify known backup bytes and total contribution, including long-path containment.
- A real isolated backend Analyze refresh observes an added disposable backup; controlled response fixtures cover unavailable and not-applicable rendering.
- Public operations documentation explains database sidecars, backup scope, caching, and the difference from whole-filesystem capacity.

## Verification

Run from `apps/web`:

For a fresh worktree, first run `rtk pnpm install --frozen-lockfile` from `apps/`.

```bash
rtk pnpm e2e:run --host --project chromium -- tests/system/storage-database-footprint.spec.ts
rtk pnpm e2e:run --host --project mobile-chrome -- tests/system/mobile-storage-database-footprint.spec.ts
```

Run the browser projects sequentially. The guarded runner rebuilds backend and frontend.
Use disposable fixture data only. Capture a phone screenshot and inspect both expanded rows.
Use causal HTTP/WS waits from the repository E2E helpers; do not add timed sleeps.

## Files likely touched

- `apps/web/e2e/tests/system/storage-database-footprint.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-database-footprint.spec.ts`
- `apps/web/e2e/helpers/storage-maintenance.ts`
- `docs/public/operations.md`

## Dependencies

Task 02.

## Risks

Database and WAL sizes change during E2E. Compare displayed values to the captured overview snapshot, not a later filesystem measurement.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/system-page/requirements/storage-maintenance.md), active requirement `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-002`.
- [System design](../../specs/system-page/system-design/storage-database-footprint.md).
- Existing storage provider, overview cache, resource rows, and mobile storage tests.

## Results

Added isolated desktop refresh coverage, mobile long-path coverage, and public
operations guidance. The required chromium and mobile-chrome E2E commands each
passed one test.
