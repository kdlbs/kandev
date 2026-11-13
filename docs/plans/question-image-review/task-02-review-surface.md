---
id: "02-review-surface"
title: "Add the opt-in question image review view"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-001
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-002
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-003
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-004
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-008
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-REVIEW-001.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-001.2
  - AC-TASKS-QUESTION-IMAGE-REVIEW-001.3
  - AC-TASKS-QUESTION-IMAGE-REVIEW-001.4
  - AC-TASKS-QUESTION-IMAGE-REVIEW-001.5
  - AC-TASKS-QUESTION-IMAGE-REVIEW-002.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-002.2
  - AC-TASKS-QUESTION-IMAGE-REVIEW-002.3
  - AC-TASKS-QUESTION-IMAGE-REVIEW-002.4
  - AC-TASKS-QUESTION-IMAGE-REVIEW-003.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-003.2
  - AC-TASKS-QUESTION-IMAGE-REVIEW-003.3
  - AC-TASKS-QUESTION-IMAGE-REVIEW-003.4
  - AC-TASKS-QUESTION-IMAGE-REVIEW-004.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-004.2
  - AC-TASKS-QUESTION-IMAGE-REVIEW-004.3
  - AC-TASKS-QUESTION-IMAGE-REVIEW-004.4
  - AC-TASKS-QUESTION-IMAGE-REVIEW-008.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-008.4
system_design:
  - ../../specs/tasks/system-design/question-image-review.md
---

# Task 02: Add the opt-in question image review view

## Summary

Deliver the review view end to end with one image at a time: the Review control,
the dialog, the moved answer form, segments, lazy loading, zoom, and pan. With
two or more images, show the first image and an image picker. Work order 03 adds
comparison.

## In scope

- `useQuestionReview` hook and the Review images button in
  `ClarificationOverlayTopBar` (`clarification-overlay-header.tsx`).
- `QuestionReviewDialog` with the desktop resizable split and the phone layout.
- Render `ClarificationCarouselBody` in exactly one place, plus the compact
  status row. Add `stageOwnsImages` to `ClarificationCard`.
- `buildReviewSegments` and the segment strip.
- Stage image loading through `useQuestionImages`. Extend its loaded state with
  `blob` for work order 03.
- `useReviewViewport` with Fit, 1:1, steps from 25% to 800%, focal zoom, pointer,
  touch, and keyboard pan, and pixelated rendering above 100%.
- Stage keyboard scope that stops propagation, plus the closing and focus rules.
- Localized copy in all seven locales.

## Out of scope

Comparison modes, the difference worker, annotations, history changes, and backend changes.

## Acceptance

1. In each host that mounts the shared overlay, image bundles show Review images
   and image-free bundles do not. Opening, closing, and Escape never submit or
   collapse. Focus returns to the control. Drafts, selections, attachments, and
   the active question are shared both ways.
2. The stage sits above a visible form on 1280×720 desktop and on the
   `mobile-chrome` phone viewport, with no horizontal document overflow and 44px
   phone targets. Only the active segment's images are requested.
3. Zoom and pan work by mouse, touch, and keyboard. Stage keys do not select,
   navigate, or submit. Form shortcuts still work with focus in the form.

## UI preview

Use [UI-01](plan.md#ui-01-review-control-in-the-compact-panel),
[UI-02](plan.md#ui-02-desktop-review-view-side-by-side) without the compare
toolbar, and [UI-03](plan.md#ui-03-phone-review-view) without mode selectors.

```text
| Review images  [1 Header ✓] [2 Spacing] [3 Colors] [4 Copy]      [Close] |
| [-] 100% [+] [Fit] [Actual size]  [Annotate]       1440 × 900 px         |
|  stage: one image, pan and zoom                                          |
|============================ drag to resize ==============================|
| question, choices, custom text, [< Prev] [Next >] / [Submit]             |
```

## Verification

Write failing tests first: segment building, viewport math, the Review control's
visibility rules, form placement, shared draft both ways, close on resolve, and
stage keys not reaching the carousel.

```bash
(cd apps/web && pnpm exec vitest run lib/question-review components/task/chat/question-review components/task/chat/clarification-panel-section.test.tsx hooks/domains/session/use-question-images.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint --deny-warnings lib/question-review components/task/chat/question-review components/task/chat/clarification-input-overlay.tsx components/task/chat/clarification-overlay-header.tsx)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/question-image-review.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-question-image-review.spec.ts)
```

The E2E specs cover: control visibility, open and close without submit, Escape,
focus return, draft sharing, segment navigation with request counting per
segment, zoom buttons and keys, phone layout bounds, and overflow. Reuse
`e2e/tests/chat/question-image-helpers.ts`.

## Likely files

- New `apps/web/components/task/chat/question-review/` (dialog, stage, segment
  strip, `use-question-review.ts`, `use-review-viewport.ts`, tests)
- New `apps/web/lib/question-review/segments.ts` and tests
- `apps/web/components/task/chat/clarification-input-overlay.tsx` (wiring only)
- `apps/web/components/task/chat/clarification-overlay-header.tsx`
- `apps/web/hooks/domains/session/use-question-images.ts`
- `apps/web/src/locales/*/task.json`
- New E2E specs under `apps/web/e2e/tests/chat/`

## Dependencies and risks

No code dependencies. Read `apps/web/AGENTS.md`, `/mobile-parity`, and `/e2e`.
Risks: the overlay file size limit; moving the form must not reset
`ClarificationCustomInput` state; nested dialogs (Quick Chat) must not close on
the review view's Escape.

## Results

Implemented on 2026-11-07 under `apps/web/components/task/chat/question-review/`
and `apps/web/lib/question-review/`. Differences from the first draft of this
work order, recorded in the system design:

- The compact panel's status row has no Return button. The modal makes the panel
  inert, so the button could never be used.
- A single-image segment has no image picker; segments with two or more images
  use the A and B pickers from work order 03.
- `useQuestionImageOwner` now owns the image-read owner key for both the compact
  previews and the stage.

Verification:

- `vitest run lib/question-review components/task/chat/question-review` plus the
  overlay, panel, choice-image, viewer, and image-hook suites: pass.
- `pnpm run typecheck`, Oxlint on changed files, `i18n:check`, `i18n:ratchet`: pass.
- `e2e:run --project chromium tests/chat/question-image-review.spec.ts`: pass.
- `e2e:run --project mobile-chrome tests/chat/mobile-question-image-review.spec.ts`: pass.
