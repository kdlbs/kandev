---
id: "05-annotation-interface"
title: "Draw and submit region annotations in the review view"
status: done
wave: 4
depends_on:
  - "03-compare-modes"
  - "04-annotation-contract"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-007
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.3
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.6
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.7
system_design:
  - ../../specs/tasks/system-design/question-image-review.md
---

# Task 05: Draw and submit region annotations in the review view

## Summary

Let the operator mark an area on an image, comment on it, and send it with the
answer. Show submitted regions in history.

## In scope

- `useReviewAnnotations` with per-question regions stored in the draft cache, and
  enrichment in `enrichAnswers`.
- Annotate mode in the single-image and Toggle views: pointer, touch, and keyboard
  drawing, moving, resizing, and deletion, in integer image coordinates.
- The Regions list in the form: number, image description, comment with counter,
  Select, and Delete. The eight-region limit and empty-comment blocking.
- History rendering in `AnswerSummary` as plain text.
- Localized copy in all seven locales.

## Out of scope

Annotations in Wipe, Side by side, Onion skin, and Difference; freehand shapes;
annotations in the compact panel.

## Acceptance

1. Regions can be drawn, edited, and deleted by mouse, touch, and keyboard. They
   survive moving between the review view and the compact panel, and between
   segments. Skip and bundle replacement clear them.
2. Submitting sends regions with the matching answer, and the agent result
   contains them. Empty comments block submission with a field error.
3. Answered history lists each region's number, image description, and comment.

## UI preview

Use the annotation part of [UI-04](plan.md#ui-04-difference-and-annotation-states).

```text
|  stage: B shown   +--[1]-------+          [Annotate*] [Delete region] |
|------------------------------------------------------------------------|
| Regions (1 of 8)                                                       |
|  1  B: After  [Label contrast too low.............] 22/500  [Delete]   |
```

## Verification

Write failing tests first for coordinate conversion and clamping, the
draft-cache round trip, enrichment, the limit, and empty-comment blocking.

```bash
(cd apps/web && pnpm exec vitest run lib/question-review components/task/chat/question-review components/task/chat/messages)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint --deny-warnings lib/question-review components/task/chat/question-review components/task/chat/messages/clarification-request-message.tsx)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/question-image-review.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-question-image-review.spec.ts)
```

The E2E specs draw a region, edit its comment, submit, assert the mock agent
received the region, and assert the history text.

## Likely files

- `apps/web/components/task/chat/question-review/` (annotation layer, region list, `use-review-annotations.ts`)
- `apps/web/components/task/chat/clarification-draft-context.tsx`
- `apps/web/components/task/chat/clarification-input-overlay.tsx` (enrichment wiring)
- `apps/web/components/task/chat/messages/clarification-request-message.tsx`
- `apps/web/src/locales/*/task.json`
- E2E specs under `apps/web/e2e/tests/chat/`

## Dependencies and risks

Depends on work orders 03 and 04. Risks: pointer handling must not conflict with
panning (annotate mode switches drag from pan to draw); touch drawing must keep
two-finger pinch working.

## Results

Implemented on 2026-11-07. Regions live in the existing answer draft
(`useClarificationDraft` returns `annotations` and `setAnnotations`; the retained
draft cache stores them per question), so they survive the view switch, segment
changes, and task navigation like text drafts. `useReviewAnnotations` owns
selection and annotate mode for the open view; `useRegionDrawing` draws with the
primary pointer while two fingers still pinch-zoom; `RegionOverlay` renders in
the shared transformed layer; `ReviewRegionList` lists regions in the form;
`AnswerRegionSummary` lists them in history. Submission is blocked while a region
has no comment. Selecting a region in Toggle shows the image it was drawn on.

Verification:

- Unit: region geometry, draft retention across task navigation, region list and
  keyboard editing in the overlay, submit gating, and the submitted body: pass.
- Desktop E2E: a mouse-drawn region on image B, its comment, delivery in
  `metadata.response.annotations`, and the history line: pass.
- Phone E2E: 44px Annotate and Delete controls, keyboard-added region, delete: pass.
- Not covered by automation: drawing a region by touch drag on a real phone.
