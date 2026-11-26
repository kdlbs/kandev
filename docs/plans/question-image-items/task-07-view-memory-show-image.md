---
id: "07-view-memory-show-image"
title: "Remembered zoom and pan, Show image on choices, and picker fixes"
status: done
wave: 6
depends_on:
  - "06-wipe-handle-names"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-003
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-004
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.4
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.9
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.10
  - AC-TASKS-QUESTION-IMAGE-ITEMS-004.1
system_design:
  - ../../specs/tasks/system-design/question-image-items.md
---

# Task 07: View memory and Show image

## Summary

Third test round (2026-11-26). Returning to an item or question keeps its zoom
and pan, and a same-size new image starts from the last view. A choice's image
line becomes a Show image button. The A/B picker, which did not open in Safari,
and the desktop zoom popovers render inside the review dialog. Wipe names stack
tighter.

## Scope

- `lib/question-review/review-carry.ts`: one dialog-owned holder for comparison
  settings and per-stage views; `use-remembered-view.ts` recalls and records.
- `useReviewViewport` gains `restore`.
- `review-image-request.ts`, `ChoiceStageImageRef`, and `useRegionReveal`.
- `ImageSlotPicker`, `ZoomLevelMenu`, and `ShortcutHelp` get `portalContainer`.
- `review-wipe-grip.tsx`: 1em letter rows; `wipeNameMaxLetters` uses 12px.

## Exclusions

Keeping views after the review view closes.

## Acceptance conditions

1. Zoom and pan survive item and question changes while the view is open, and
   carry to a same-size image only.
2. Show image moves the stage to the choice's image.
3. The A/B picker list renders inside the dialog.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat lib/question-review)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/chat/question-image-items.spec.ts e2e/tests/chat/question-image-review.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome e2e/tests/chat/mobile-question-image-items.spec.ts e2e/tests/chat/mobile-question-image-review.spec.ts)
```

## Likely files

`review-carry.ts`, `use-remembered-view.ts`, `use-review-viewport.ts`,
`use-compare-stage.ts`, `question-review-stage.tsx`, `question-review-dialog.tsx`,
`review-image-request.ts`, `use-region-reveal.ts`, `question-review-controls.tsx`,
`review-desktop-controls.tsx`, `review-compare-toolbar.tsx`, `review-stage-zoom.tsx`,
`review-wipe-grip.tsx`, `identity.ts`, locales, and the review E2E specs.

## Risks

WebKit cannot run on the development host, so the Safari picker fix is verified
by the cause (portal inside the modal) and the DOM test, not in Safari itself.

## Results

- Views: kept per stage key and carried to same-size images; recall waits for
  the image size and happens once per mount.
- Show image: a link-style button with a 44px touch target on phones.
  Superseded by work order 08, where selecting the choice shows its image.
- Popovers: picker, zoom level, and shortcut help render inside the dialog.
- Letters: 1em rows instead of 1.15em.
