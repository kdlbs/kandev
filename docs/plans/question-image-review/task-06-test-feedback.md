---
id: "06-test-feedback"
title: "Fix review view issues found in manual testing"
status: done
wave: 5
depends_on:
  - "05-annotation-interface"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-002
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-004
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-005
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-007
  - REQ-TASKS-QUESTION-IMAGES-003
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-REVIEW-002.3
  - AC-TASKS-QUESTION-IMAGE-REVIEW-004.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.3
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.4
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.7
  - AC-TASKS-QUESTION-IMAGES-003.1
system_design:
  - ../../specs/tasks/system-design/question-image-review.md
  - ../../specs/tasks/system-design/question-image-previews.md
---

# Task 06: Fix review view issues found in manual testing

## Summary

The user tested the review view in a secondary instance on 2026-11-07 and
reported four issues. Fix them without changing the answer contract.

## In scope

1. **Wipe reach:** on narrow screens the divider reached 0% and 100% only with a
   finger at the screen edge. Map the drag over the stage minus a 24px margin on
   each side, keep the handle inside the stage at the extremes, and add a Wipe
   slider to the toolbar.
2. **Fit clipping:** Fit was clamped to the 25% minimum, so stacked Side by side
   panes clipped on phones. Fit always contains the image; zooming out stops at
   the smaller of Fit and 25%. On phones the comparison toolbar uses one
   horizontally scrolling row so the stage keeps its height.
3. **Region comments:** the comment field sat at the bottom of the form, often
   out of view. Reuse the chat's `PlanSelectionPopover` next to the region (a
   bottom drawer on touch). A new region needs a comment to be kept. The Regions
   list becomes a read-only summary with Edit and Delete.
4. **Compact thumbnails:** question thumbnails flow in a wrapping row instead of
   a vertical stack.

## Out of scope

Answer contract changes, new comparison modes, and history layout changes beyond
the shared thumbnail row.

## Acceptance

1. On a 390 by 844 phone, a touch drag that stops 30px from either screen edge
   moves the divider to 0% or 100%, and both Side by side panes contain their
   whole image.
2. Finishing a region opens a comment popover next to it; cancelling discards a
   new region; selecting a region reopens it for editing or deletion.
3. Two compact thumbnails share one row on desktop and phone without horizontal
   document overflow.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/question-review components/task/chat/question-review components/task/chat/question-images.test.tsx components/task/plan-selection-popover.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/question-image-review.spec.ts tests/chat/question-image-previews.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-question-image-review.spec.ts tests/chat/mobile-question-image-previews.spec.ts)
```

## Likely files

- `apps/web/lib/question-review/viewport.ts`, `use-review-viewport.ts`
- `apps/web/components/task/chat/question-review/review-compare-layers.tsx`,
  `review-compare-toolbar.tsx`, `review-region-list.tsx`, region drawing hooks
- `apps/web/components/task/plan-selection-popover.tsx`
- `apps/web/components/task/chat/question-images.tsx`

## Results

Implemented on 2026-11-07.

1. Wipe: `lib/question-review/wipe.ts` (`wipeFromPointer`, 24px margins); the
   handle is positioned with CSS `clamp()`; the toolbar has a Wipe slider.
2. Fit: `fitView` has no lower bound; `minScaleFor` bounds zoom steps, wheel, and
   pinch. Stage toolbars use `reviewToolbarRow`, one scrolling line on phones.
3. Region comments: `RegionCommentPopover` reuses `PlanSelectionPopover` inside the
   dialog; `useReviewAnnotations` gained `commenting`, `editComment`,
   `saveComment`, and `closeComment`; Enter edits a selected region; the dialog
   defers Escape to an open popover; the Regions list is read-only with Edit.
4. Thumbnails: `QuestionImages` renders a wrapping `question-images-row`; captions
   take the thumbnail's width.

Verification:

- Unit: viewport Fit below 25%, wipe mapping, comment controller, and overlay
  popover flow (open, cancel discards, edit, delete, submit): pass.
- Desktop E2E (review and existing preview specs, 8 tests): pass, including the
  popover next to a mouse-drawn region and thumbnails sharing a row.
- Phone E2E (7 tests): pass, including real touch drags reaching 0% and 100% with
  the finger 30px from the screen edges, both stacked Side by side panes
  containing their image, the comment drawer, and thumbnails sharing a row.
