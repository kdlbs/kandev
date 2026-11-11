---
id: "07-usability-round"
title: "Second usability round for question image review"
status: done
wave: 6
depends_on:
  - "06-test-feedback"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-001
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-002
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-003
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-004
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-005
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-007
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-009
  - REQ-TASKS-QUESTION-IMAGES-003
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-REVIEW-001.6
  - AC-TASKS-QUESTION-IMAGE-REVIEW-002.3
  - AC-TASKS-QUESTION-IMAGE-REVIEW-003.5
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.3
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.11
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.12
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.13
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-007.7
  - AC-TASKS-QUESTION-IMAGE-REVIEW-009.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-009.2
  - AC-TASKS-QUESTION-IMAGE-REVIEW-009.3
  - AC-TASKS-QUESTION-IMAGES-003.6
system_design:
  - ../../specs/tasks/system-design/question-image-review.md
  - ../../specs/tasks/system-design/question-image-previews.md
---

# Task 07: Second usability round for question image review

## Summary

The user tested the review view again on 2026-11-09 and asked for ten changes.

## In scope

1. Gallery navigation in the thumbnail inspector: Previous and Next, arrow keys,
   swipe at Fit, a counter, no wrap-around.
2. Candidate stepping in Toggle and Side by side with three or more images:
   Previous and Next step B with A fixed; `[` and `]`; swipe at Fit.
3. Pinned center wipe on phones and touch screens: a fixed center line with the
   images scrubbed beneath it, an extended pan range, and the slider as an
   alternative.
4. Condensed phone toolbar: mode and More at the top, frequent actions in a fixed
   bottom bar, zoom and status chips on the image, sheets for mode and image
   picking, and the bar hidden while a text field has focus.
5. Annotation usability: pen icon, **Mark area**, solid primary when on, a hint
   chip with Done, Escape order (comment, marking, view), clicking a region to
   edit it, and the regions list in a desktop side panel or a phone sheet.
6. Agent-supplied one-word image labels, shown in pickers, references, and counters.
7. The comparison mode and slider values carry over between segments.
8. Picking an answer in the review view does not advance to the next question; a
   **Next question** control moves on.
9. Zoom to changes in Difference, with an outline around the changed area at Fit.
10. Hold to compare: press and hold the image, or hold Space, to show A.

## Out of scope

Answer contract changes beyond image labels, history layout, and server-side comparison.

## Acceptance

1. Backend validation, persistence, retry identity, and schema text cover image labels.
2. Desktop and phone E2E specs cover each listed behavior.
3. The phone toolbar has no horizontal overflow at 390 by 844 and every control
   measures at least 44px.

## Verification

```bash
(cd apps/backend && go test ./internal/clarification/... ./internal/mcp/server -count=1)
(cd apps/web && pnpm exec vitest run lib/question-review components/task/chat/question-review components/task/chat)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/question-image-review.spec.ts tests/chat/question-image-previews.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-question-image-review.spec.ts tests/chat/mobile-question-image-previews.spec.ts)
```

## Results

Implemented on 2026-11-09.

1. Inspector gallery: `QuestionImageGallery` (card and history), Previous and Next,
   arrow keys, swipe at Fit, a counter with labels, focus return to the image that
   was showing.
2. Candidate stepping: `compareReducer` `step`, `CandidateStepper`, `[`/`]`, swipe at Fit.
3. Pinned touch wipe: `clampPinnedView`, `pinnedWipeFraction`, `PinnedWipeLine`;
   entering or leaving pinned Wipe returns to Fit.
4. Phone composition: `ReviewLayoutProvider`, mode and More sheets, regions button,
   `ZoomChip`, `StatusChips`, `ReviewBottomBar`, bar hidden while typing.
5. Marking: **Mark area** with a pen icon and solid primary state, `AnnotateHint`
   with Done, a ring on the stage, Escape order, click-to-edit regions, side panel
   on wider screens and a sheet on phones.
6. Image labels: backend validation, schema, persistence; web short names.
7. Carried-over comparison settings between segments.
8. No advance on answer in the review view; Next is emphasized.
9. Changed-pixel bounds, outline, and Zoom to changes.
10. Hold to compare by long press or Space.

Verification:

- Backend: `go test ./internal/clarification/... ./internal/mcp/server`: pass.
- Web unit: review, gallery, overlay, and helper tests: pass.
- Desktop E2E (11 tests, including the existing preview specs): pass.
- Phone E2E (7 tests): pass, including real touch scrubbing of the pinned wipe
  with the finger at least 60px from the screen edges, bottom-bar actions, the
  bar hiding while typing, the regions drawer and sheet, and inspector swipes.
