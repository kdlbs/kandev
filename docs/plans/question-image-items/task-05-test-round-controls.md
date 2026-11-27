---
id: "05-test-round-controls"
title: "Pair controls, identity placement, and phone zoom after the test round"
status: done
wave: 4
depends_on:
  - "04-visible-identity"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-003
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-004
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.2
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.4
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.8
  - AC-TASKS-QUESTION-IMAGE-ITEMS-004.1
  - AC-TASKS-QUESTION-IMAGE-ITEMS-004.3
system_design:
  - ../../specs/tasks/system-design/question-image-items.md
---

# Task 05: Test-round controls

## Summary

Feedback from testing the implemented package on a live instance (2026-11-25).
Pairs drop the A and B chips and Swap, Flip uses the primary style on desktop,
the identity moves to the bottom left (Wipe: upright labels on the divider),
phone Compare and Back to gallery get text labels, and phone zoom opens from the
zoom chip instead of More.

## Scope

- `ReviewCompareToolbar` and the phone bottom bar: no `ImagePairControl` or Swap
  for pairs; `useReviewCompare` ignores `s` for pairs; Flip `variant="default"`.
- `IdentityBadges`: bottom-left pill shared with the status chips; per-pane
  captions at each pane's bottom left; Wipe divider labels with hide and
  truncate rules.
- Phone: labeled Compare (top row) and Gallery (bottom-bar row); `ZoomChip`
  popover with `ReviewZoomControls`; More keeps only Mark area, and singles lose
  More.

## Exclusions

Replacing the operator comparison pickers, desktop zoom, and new comparison
modes.

## Acceptance conditions

1. A pair shows no pickers or Swap on desktop or phones; an operator comparison
   still has both.
2. The identity renders bottom left, per pane, or on the divider as designed, and
   never covers the wipe handle, zoom controls, or Mark area at 1440px, 393px,
   and 320px.
3. On phones, the zoom chip opens zoom controls, More has no zoom, and Compare
   and Gallery show text labels without horizontal overflow at 320px.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/question-review lib/question-review)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/chat/question-image-items.spec.ts e2e/tests/chat/question-image-review.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome e2e/tests/chat/mobile-question-image-items.spec.ts e2e/tests/chat/mobile-question-image-review.spec.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint)
```

## Likely files

`review-compare-toolbar.tsx`, `review-compare-stage.tsx`,
`review-identity-badges.tsx`, `review-phone-controls.tsx`,
`review-item-nav.tsx`, `question-review-stage.tsx`, `use-review-compare.ts`,
`use-compare-stage.ts`, locales, and the four review E2E specs.

## Risks

Divider labels near the edges, long labels in other locales at 320px, and the
popover inside the phone dialog's stacking context.

## Results

- Pairs: no pickers or Swap (desktop toolbar and phone bar); `s` ignored for
  pairs. Operator comparisons keep both. Flip is primary on desktop.
- Identity: `StageBottomRow` holds the identity pill and status chips on the
  left and zoom on the right, so neither covers the other. Side by side has a
  caption per pane. Wipe names sit beside the divider (`WipeLabels`, upright
  stacked text) and hide by `wipeLabelSides` when a side is under 52px.
- Phones: Compare (top row) and Gallery (bottom-bar row) show text labels with
  the full accessible names. At 320px the candidate stepper shares the Gallery
  row, so Flip keeps its touch size. `ZoomChip` opens a popover with
  `ReviewZoomControls`; More keeps Mark area, and gallery items have no More.
- Shared `StageZoom` and `ItemModeButton` replace duplicated markup.
- Verification: question-review unit tests, desktop E2E 13 passed, phone E2E
  10 passed, `make typecheck` and `make lint` clean.
