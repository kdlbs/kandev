---
id: "01-late-answer-review"
title: "Review images in the late-answer form"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-001
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-001
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-REVIEW-001.1
  - AC-TASKS-QUESTION-IMAGE-ITEMS-001.1
  - AC-TASKS-QUESTION-IMAGE-ITEMS-001.2
  - AC-TASKS-QUESTION-IMAGE-ITEMS-001.3
  - AC-TASKS-QUESTION-IMAGE-ITEMS-001.4
  - AC-TASKS-QUESTION-IMAGE-ITEMS-001.5
  - AC-TASKS-QUESTION-IMAGE-ITEMS-001.6
system_design:
  - ../../specs/tasks/system-design/question-image-items.md
  - ../../specs/tasks/system-design/question-image-review.md
---

# Task 01: Review images in the late-answer form

## Summary

Issue #5494. Make the existing review view available in the late-answer form, and
send from it through the late-answer path. Design: "Late-answer review".

## Scope

- `isQuestionReviewAvailable` takes `lateStatus` instead of `lateInteraction`;
  false for sent or queued late answers, and for `ok` or `expired` submit states.
- The overlay passes `lateMode ? effectiveLateStatus : group.lateAnswerState`.
- `ClarificationHeaderActions` renders `reviewAction` in its `lateMode` branch.
- `QuestionReviewSubmit.label: "submit" | "late"`; the overlay shows the dialog
  submit in late mode with the late copy and keeps `onSubmit = handleSubmit`.
- Move the late error banner into the shared `form` fragment.
- Locale keys only if new copy is needed (existing `task:lateAnswerSend` and
  `task:lateAnswerSending` should cover it).

## Exclusions

Item navigation and identity (work orders 03 and 04). No backend change.

## UI preview

From [the plan](plan.md#ui-01-late-answer-form-header):

```text
| | Question 1 of 2   [Review images] [clip] [Close] [Send as new message]| |
```

Dialog header in late mode:

```text
| Review images [1 Header ✓] [2 Screens]               [Send as new message] [Close] |
```

## Acceptance

1. A late-answer form for an image bundle shows Review images; an image-free one
   does not. Opening and closing the view never sends.
2. Sending from the view delivers a late answer (the late endpoint, not
   `/clarification/*/respond`), closes the view, and shows the late success row.
3. A failed late send keeps the view open with the late error and Retry, and the
   draft, selections, and regions remain.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/question-review/use-question-review.test.ts components/task/chat/question-review/question-review-overlay.test.tsx components/task/chat/question-review/question-review-late.test.tsx components/task/chat/clarification-input-overlay.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/chat/question-image-review.spec.ts e2e/tests/chat/question-image-review-late.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome e2e/tests/chat/mobile-question-image-review.spec.ts e2e/tests/chat/mobile-question-image-review-late.spec.ts)
```

Add an E2E case to `question-image-review.spec.ts`: skip an image question with a
routed 409 `not_active` respond (as in `clarification.spec.ts` "late answer from
an inactive historical question"), click **Answer as new message**, open Review
images, choose an answer, send from the dialog, and assert the late success row
and that `respond` was not called again. Add a phone case that the control and
late send are reachable with 44px targets.

## Likely files

- `apps/web/components/task/chat/question-review/use-question-review.ts` and test
- `apps/web/components/task/chat/clarification-input-overlay.tsx`
- `apps/web/components/task/chat/clarification-overlay-header.tsx`
- `apps/web/components/task/chat/question-review/question-review-dialog.tsx`
- `apps/web/components/task/chat/question-review/question-review-overlay.test.tsx`
- `apps/web/e2e/tests/chat/question-image-review.spec.ts`,
  `mobile-question-image-review.spec.ts`, `question-image-review-helpers.ts`

## Risks

- The overlay file is over the line limit; keep edits to arguments and the banner
  move.
- Escape inside the dialog must not dismiss the late form in chat history.

## Results

Implemented on 2026-11-24. `isQuestionReviewAvailable` takes the late status;
the late header renders the Review control; the dialog's submit uses
`label: "late"` and the late copy; the late error banner moved into the shared
form. Unit tests: `use-question-review.test.ts`, `question-review-late.test.tsx`
(control offered, shared state without sending, late send closes the view,
failure keeps the view and retries). E2E: `question-image-review-late.spec.ts`
(chromium) and `mobile-question-image-review-late.spec.ts` (mobile-chrome) pass,
including Escape closing only the view inside chat history.
