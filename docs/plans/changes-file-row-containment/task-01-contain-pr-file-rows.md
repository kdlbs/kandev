---
id: "01-contain-pr-file-rows"
title: "Contain PR file rows"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CHANGES-FILE-ROW-CONTAINMENT-001
acceptance_criteria:
  - AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.1
  - AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.2
  - AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.3
  - AC-UI-CHANGES-FILE-ROW-CONTAINMENT-001.4
system_design:
  - ../../specs/ui/system-design/changes-file-row-containment.md
---

# Task 01: Contain PR File Rows

## Summary

Make PR Changes file rows reuse the working-tree row's shrink-safe path and
trailing-metadata geometry. Prove the correction at the desktop Dockview
minimum and in the Pixel 5 Changes surface while preserving PR diff routing.

## In scope

- Add failing desktop and mobile rendered-overlap regressions with one shared
  PR seed helper.
- Apply the minimum `PRFileRow` class correction.
- Record RED/GREEN results in this work order and `plan.md`.

## Out of scope

- A shared file-row abstraction.
- Changes to local file rows, status semantics, statistics, row actions,
  responsive navigation, or scroll ownership.

## Acceptance

- A long PR path truncates before its statistics and status marker at the 180px
  desktop Changes-panel minimum and on Pixel 5.
- Trailing metadata remains contained and hit-testable, and neither surface
  gains horizontal overflow.
- Clicking or tapping the seeded row still opens the PR-scoped diff with the
  expected content.

## TDD sequence

1. Add desktop/mobile Playwright geometry scenarios; run each and record the
   expected overlap failures.
2. Replace the non-shrinking PR basename and shrinkable trailing container with
   the established working-tree row classes.
3. Run the focused unit test, desktop E2E, and mobile E2E until green.
4. Run web lint and typecheck; update work-order and plan results.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test components/task/changes-panel-pr-files.test.tsx
cd apps/web && pnpm e2e:run --project chromium tests/git/pr-file-row-containment.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/git/mobile-pr-file-row-containment.spec.ts
cd apps && pnpm --filter @kandev/web lint && pnpm --filter @kandev/web typecheck
```

## Files likely touched

- `apps/web/components/task/changes-panel-pr-files.tsx`
- `apps/web/e2e/tests/git/pr-file-row-containment-helpers.ts`
- `apps/web/e2e/tests/git/pr-file-row-containment.spec.ts`
- `apps/web/e2e/tests/git/mobile-pr-file-row-containment.spec.ts`
- `docs/plans/changes-file-row-containment/plan.md`
- `docs/plans/changes-file-row-containment/task-01-contain-pr-file-rows.md`

## Dependencies

None.

## Risks

- Dockview width changes must settle before bounding boxes are read; reuse
  `resizeColumnViaSplitview` and poll the rendered Changes width.
- The browser assertion must compare real element boxes and center hit-testing,
  not infer correctness from visibility alone.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-CHANGES-FILE-ROW-CONTAINMENT-001` and its system design.
- User screenshot showing a long PR filename crossing change metadata.
- Working-tree `FileRow` and its truncation regression as the local exemplar.
- `resizeColumnViaSplitview`, `SessionPage`, mobile Changes tests, and mock
  GitHub API helpers as E2E patterns.

## Results

Completed on 2026-08-25.

- Added one shared mock-GitHub seed and separate desktop/mobile rendered
  regressions for a long PR basename with fixed `+123`, `-45`, and modified
  status metadata.
- RED desktop measured the filename right edge at `1743.25px` against the
  additions left edge at `1270.1875px`. RED Pixel 5 measured `569.25px`
  against `285.1875px`.
- Updated only `PRFileRow` layout classes: directory and basename now accept
  truncation, while the statistics/status region cannot shrink.
- GREEN desktop and mobile scenarios prove bounding-box separation, row and
  document containment, status hit-testing, preserved full-path title, and
  successful click/tap diff opening.
- `changes-panel-pr-files.test.tsx`: 3 tests passed.
- Web ESLint and TypeScript typecheck passed.
