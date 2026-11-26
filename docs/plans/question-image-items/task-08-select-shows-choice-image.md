---
id: "08-select-shows-choice-image"
title: "Selecting a choice shows its image"
status: done
wave: 7
depends_on:
  - "07-view-memory-show-image"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-003
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.10
system_design:
  - ../../specs/tasks/system-design/question-image-items.md
---

# Task 08: Selecting a choice shows its image

## Summary

Fourth test round (2026-11-26). A choice is not final until the answer is sent,
so selecting it can show its image directly. The separate Show image button
goes; a passive image marker sits inside the choice instead.

```text
+--------------------------------------------+
| [1] Old                          [img] 2   v |
|     Keep the old header                    |
+--------------------------------------------+
| [2] Plain                                  |
+--------------------------------------------+
```

## Scope

- `ClarificationOptions` gains `renderOptionBadge`, drawn inside the choice
  before the selected check.
- `ChoiceStageImageHint` replaces `ChoiceStageImageRef`; `choiceStageImageId`
  picks the choice's first image.
- `ClarificationCarouselBody` wraps option selection, so clicks and number keys
  call `ShowImageContext` while the review view is open.
- Locale key `questionReviewChoiceImageHint` replaces
  `questionReviewChoiceImageRef`.

## Exclusions

Choices outside the review view keep their thumbnails and do not move anything.

## Acceptance conditions

1. Selecting a choice with an image, by click, tap, or number key, moves the
   stage to it, again after navigating away; a choice without one does not.
2. The marker is inside the choice, announced as part of its name, and adds no
   separate control.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/question-review components/task/chat/clarification)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/chat/question-image-items.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome e2e/tests/chat/mobile-question-image-items.spec.ts)
```

## Likely files

`clarification-overlay-parts.tsx`, `clarification-input-overlay.tsx`,
`question-review-controls.tsx`, locales, `question-review-items.test.tsx`,
`question-review-overlay.test.tsx`, and the item E2E specs.

## Risks

None known; selection already records a draft answer, and the stage move reuses
the region-reveal path.

## Results

- Marker: photo icon plus short name, with a screen-reader "Image {name}".
- Selection: one handler for clicks and number keys; re-selection moves again.
- Phone: the marker stays inside the choice row, which keeps a 44px height.
