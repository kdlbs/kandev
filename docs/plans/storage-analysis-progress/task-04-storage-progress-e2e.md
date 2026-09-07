---
id: "04-storage-progress-e2e"
title: "Prove progressive storage behavior"
status: done
wave: 4
depends_on:
  - 01-bounded-filesystem-scanner
  - 02-progressive-overview-state
  - 03-live-analysis-ui
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.1
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.2
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.3
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.4
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.5
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.6
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.7
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.8
  - AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.9
system_design:
  - ../../specs/system-page/system-design/storage-analysis-progress.md
---

# Task 04: Prove Progressive Storage Behavior

## Summary

Prove the integrated desktop and phone flows. Record bounded-scan timing without changing the live
storage data of the user.

## In scope

- Add desktop Playwright coverage for progressive data and the timing disclosure.
- Add mobile Playwright coverage for tap behavior, touch size, and containment.
- Make sure that cached, stale, failed, and manual Analyze flows remain correct.
- Record scanner benchmark results and a same-dataset live comparison when available.

## Out of scope

- Broad browser or backend verification.
- Creating large files in the live Kandev directory of the user.
- Performance claims without comparable measurements.

## Acceptance

- Desktop and phone flows receive useful data before the first scan finishes.
- Both viewports expose timing details without hover-only controls or horizontal overflow.
- The handoff records exact benchmark and available live timing evidence.

## Verification

```bash
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --project chromium tests/system/storage-maintenance.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-storage-maintenance.spec.ts)
```

## Files likely touched

- `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`
- `apps/web/e2e/helpers/storage-maintenance.ts`
- `docs/plans/storage-analysis-progress/plan.md`
- `docs/plans/storage-analysis-progress/task-04-storage-progress-e2e.md`

## Dependencies

Tasks 01, 02, and 03 must be complete.

## Risks

- A fast test scan can finish before Playwright observes an intermediate state.
- Browser interception can prove UI behavior but cannot prove backend scan concurrency.
- Live storage timing can differ because of filesystem caches and background host activity.

## Parallelism

`sequential`

## Inputs

- All acceptance criteria in the requirement.
- Backend barrier tests from Tasks 01 and 02.
- Existing desktop and mobile Storage E2E flows.

## Results

Added progressive desktop and mobile storage fixtures and coverage for counted-so-far values, source
states, timing disclosure, touch sizing, and horizontal containment. Production builds passed. The
storage-maintenance E2E suite passed with 8 Chromium tests and the mobile suite passed with 6 tests.
