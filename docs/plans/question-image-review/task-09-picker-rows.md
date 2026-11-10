---
id: "09-picker-rows"
title: "Shared image picker rows with thumbnail, nickname, and description"
status: done
wave: 8
depends_on:
  - "08-toolbar-round"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-005
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-009
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-009.1
system_design:
  - ../../specs/tasks/system-design/question-image-review.md
---

# Task 09: Shared image picker rows

## Summary

Fourth test round, 2026-11-09. The desktop A/B popovers and the phone image sheet
use one option row: thumbnail, nickname, description (two lines), choice label,
a check on the current pick, and an "in A" or "in B" marker on the image in the
other slot.

## Acceptance

1. Desktop and phone pickers show the same rows; choosing a row sets the slot.
2. Rows reuse loaded image bytes and add no reads.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/question-review)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/question-image-review.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-question-image-review.spec.ts)
```

## Results

Implemented on 2026-11-09 in `review-image-option.tsx`, used by
`ImageSlotPicker` (desktop popover) and `PickerSection` (phone sheet). The stage
model exposes `urls` from loaded images. Unit test for the row contents and
markers; desktop and phone E2E pass (7 and 4 tests), including descriptions,
thumbnails, the "In B" marker, and 44px phone rows.
