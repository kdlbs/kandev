---
id: "03-item-review"
title: "Item-by-item review with pairs, singles, and gallery"
status: done
wave: 2
depends_on:
  - "02-image-group-contract"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-003
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-005
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-002
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-003
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-REVIEW-003.5
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.11
  - AC-TASKS-QUESTION-IMAGE-ITEMS-002.3
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.1
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.2
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.3
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.4
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.5
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.6
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.7
system_design:
  - ../../specs/tasks/system-design/question-image-items.md
  - ../../specs/tasks/system-design/question-image-review.md
---

# Task 03: Item-by-item review

## Summary

Issue #5495, review half. Segments become lists of items. Pairs get the
comparison tools; singles and gallery items show one image without them.
Ungrouped segments default to a gallery with **Compare images**. Design: "Segment
items" and "Item stage".

## Scope

- `buildSegmentItems` and `itemIndexForImage` in `lib/question-review/segments.ts`,
  with fallback to gallery for malformed groups.
- `useReviewItems(segment)` for `itemIndex` and `operatorCompare`.
- Split `QuestionReviewStage` into `SegmentStage` (owns `useStageImages`) and
  `ItemStage`; `useCompareStage` and `useSingleImageStage` take loaded images.
- `ReviewCompareStage` gains `pair` (hide pickers) and an initial `{ aId, bId }`.
- `ItemNav` on the desktop toolbar and phone bottom bar; `PageUp`/`PageDown`.
- Swipe chooses item navigation or candidate stepping by item kind.
- Compare images and Back to gallery controls.
- Region selection moves to the region's item.
- Localized copy in all locales (`Item {{n}} of {{total}}`, `Pair`, `Single`,
  `Image`, `Compare images`, `Back to gallery`, Previous/Next item names).

## Exclusions

Identity badges (work order 04). Comparison inside grouped segments.

## UI preview

From [the plan](plan.md#ui-02-desktop-review-view-on-a-pair-opened-from-a-late-form):

```text
| [‹] Item 1 of 3 · Pair [›] | [Toggle|Wipe|Side|Onion|Diff]  [⇄ Swap]  [Flip] |
| [‹] Item 3 of 3 · Single [›]                                                  |
| [‹] Item 2 of 3 · Image [›]   [Compare images]                                |
| [Back to gallery] | [Toggle|Wipe|...]  A [2 v] ⇄ B [3 v] ‹ 1/2 ›  [Flip]       |
```

Phone bottom bar ([UI-04](plan.md#ui-04-phone-review-view)):

```text
| [‹] 1/3 Pair [›] | [⇄] [ Flip A/B ]  |
```

## Acceptance

1. A question with one pair and two singles reviews as three items: comparison
   tools on the pair only, with item navigation by buttons, `PageUp`/`PageDown`,
   and swipe on desktop and phone.
2. An ungrouped question reviews as a gallery; Compare images starts an operator
   comparison with A and B pickers and candidate stepping; Back to gallery returns.
3. Moving between items issues no new image requests, and keeps the mode and
   slider values.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/question-review components/task/chat/question-review)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/chat/question-image-review.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome e2e/tests/chat/mobile-question-image-review.spec.ts)
```

Update existing E2E cases that assumed a flat two-image question opens in
comparison: either declare a pair in `reviewScript` or click Compare images first.
Add a pair-plus-singles case with `recordImageRequests` to prove no refetch.

## Likely files

- `apps/web/lib/question-review/segments.ts`, `segments.test.ts`
- `apps/web/components/task/chat/question-review/question-review-stage.tsx`
- `apps/web/components/task/chat/question-review/review-compare-stage.tsx`
- `apps/web/components/task/chat/question-review/use-compare-stage.ts`
- `apps/web/components/task/chat/question-review/review-compare-toolbar.tsx`,
  `review-phone-controls.tsx`, `review-desktop-controls.tsx`
- New `use-review-items.ts` (+ test) and `review-item-nav.tsx`
- `apps/web/components/task/chat/question-review/question-review-dialog.tsx`
- `apps/web/locales/*/task.json` (generate zh-Hant with `pnpm run i18n:zh-hant`;
  revert unrelated catalog changes)
- E2E specs and `question-image-review-helpers.ts`

## Risks

- File and function limits in `review-compare-stage.tsx` and
  `use-compare-stage.ts`; extract components rather than growing them.
- Swipe on touch Wipe must keep scrubbing.

## Results

Implemented on 2026-11-24: `buildSegmentItems`/`itemIndexForImage`,
`use-review-items.ts`, `review-item-nav.tsx`, and a `QuestionReviewStage` that
loads segment images once and mounts one stage per item. Pairs show read-only
slot names; phones put Back to gallery in the top row so the bottom bar keeps
room for Flip. A found issue, keyboard focus lost on item change, is fixed by
`useKeepStageFocus`. Unit tests: `segments.test.ts`, `use-review-items.test.ts`,
`use-compare-stage.test.ts` (`swipeAction`), `question-review-items.test.tsx`;
legacy overlay tests now declare pairs or use Compare images. E2E:
`question-image-items.spec.ts` (pair, singles, PageUp/PageDown, no refetch, gallery
comparison) and `mobile-question-image-items.spec.ts` (swipe and bar navigation),
plus the existing review specs on both projects, pass.

Review follow-up (2026-11-25): phones put item navigation in its own bottom-bar
row and Compare images in the top row, so no control drops below touch size or
leaves the screen at 320px; a region selection is revealed once
(`useRegionReveal`) and no longer overrides Back to gallery or a new comparison.
