---
id: "02-shared-region-annotation"
title: "Shared region annotation primitives"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.6
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-03.md
---

# Task 02: Shared region annotation primitives

## Summary

Move image review's region outlines, mode hint, and region geometry into
shared modules, and add opt-in pending (dashed) styling and move/resize
handles. Question review keeps its behavior.

## In scope

- `components/shared/region-annotation/region-overlay.tsx` from
  `RegionOverlay`, plus `pendingIndexes` and `onHandlePointerDown` props.
  Handles use a 44-pixel hit area on coarse pointers.
- `components/shared/region-annotation/annotation-mode-hint.tsx` from
  `AnnotateHint`.
- `lib/regions/geometry.ts` from `lib/question-review/regions.ts` geometry,
  plus `resizeRegionFromCorner`. Question-review constants stay where they are.
- Question-review imports updated.

## Out of scope

- Enabling handles in question review, and any preview UI.

## Acceptance conditions

1. Question-review unit and E2E tests pass unchanged.
2. The overlay renders dashed pending regions and handles only when asked, and
   corner resize clamps to bounds with a minimum size.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/shared/region-annotation lib/regions components/task/chat/question-review lib/question-review
pnpm run typecheck && pnpm run lint
pnpm e2e:run --host --shards 1 --project chromium tests/chat/question-image-review.spec.ts
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-question-image-review.spec.ts
```

## Likely files

- `apps/web/components/task/chat/question-review/review-regions.tsx`, `use-stage-annotations.tsx`, `use-region-drawing.ts`
- `apps/web/lib/question-review/regions.ts`
- New `apps/web/components/shared/region-annotation/*`, `apps/web/lib/regions/geometry.ts`, tests

## Risks

- Test IDs (`question-review-region`, `question-review-annotate-done`) must
  stay stable. Pass them as props from question review.
