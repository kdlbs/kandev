---
id: "06-wipe-handle-names"
title: "Wipe names on the handle, Onion layer list, and no swipe navigation"
status: done
wave: 5
depends_on:
  - "05-test-round-controls"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-003
  - REQ-TASKS-QUESTION-IMAGE-ITEMS-004
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-005
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.3
  - AC-TASKS-QUESTION-IMAGE-ITEMS-003.7
  - AC-TASKS-QUESTION-IMAGE-ITEMS-004.1
  - AC-TASKS-QUESTION-IMAGE-ITEMS-004.3
  - AC-TASKS-QUESTION-IMAGE-ITEMS-004.4
  - AC-TASKS-QUESTION-IMAGE-ITEMS-004.5
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.11
system_design:
  - ../../specs/tasks/system-design/question-image-items.md
  - ../../specs/tasks/system-design/question-image-review.md
---

# Task 06: Wipe handle names

## Summary

Second test round on a live instance (2026-11-25 and 26), with the handle
designed iteratively in the real components. Wipe names move onto a slim handle
and show only while the divider is in use; the handle also drags up and down and
keeps its height for the question round. Onion skin shows a layer list, Side by
side keeps zoom at the far corner, the slot dot goes, dragging a slider no longer
re-renders the dialog, and drags never navigate.

## Scope

- `review-wipe-grip.tsx`: `WipeGripHalves`, `useWipeGrip`, and
  `WipeLiftContext`; `WipeHandle` and `PinnedWipeLine` render it.
- `use-wipe-names-reveal.ts`: hold, linger, and pulse for the names; the divider
  slider holds them under the pointer.
- `useQuestionReview`: per-round `wipeLift`.
- `IdentityBadges`: `LayerBadge` for Onion skin, no slot dot, no `WipeLabels`.
- `PaneOverlay`: zoom on the second pane in Side by side.
- `QuestionReviewDialog`: carried compare settings in a ref.
- Remove swipe navigation (`swipeAction`, `useSwipe`, the single stage's swipe).

## Exclusions

The thumbnail inspector outside the review view keeps its swipe between images.
Imperative divider updates during a drag.

## Acceptance conditions

1. Wipe names sit on the handle, hidden until the handle is hovered, focused, or
   held, the divider slider is under the pointer, or the split moves; the handle
   drags vertically and keeps its height for the round.
2. Onion skin shows the layer list, Side by side has one zoom cluster in the
   second pane, and the badge has no slot dot.
3. A drag on the image never changes the item or candidate, on desktop or phones.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/question-review lib/question-review)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/chat/question-image-items.spec.ts e2e/tests/chat/question-image-review.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome e2e/tests/chat/mobile-question-image-items.spec.ts e2e/tests/chat/mobile-question-image-review.spec.ts)
(cd apps/web && pnpm run typecheck && pnpm run lint)
```

## Likely files

`review-wipe-grip.tsx`, `use-wipe-names-reveal.ts`, `review-compare-layers.tsx`,
`review-compare-controls.tsx`, `review-compare-stage.tsx`,
`review-compare-toolbar.tsx`, `review-identity-badges.tsx`,
`question-review-dialog.tsx`, `use-question-review.ts`, `use-compare-stage.ts`,
`use-review-viewport.ts`, `question-review-stage.tsx`, `identity.ts`, locales,
and the four review E2E specs.

## Risks

Vertical text in WebKit (avoided with stacked letters), the handle intercepting
a scrub on small phone stages, and names over dark image areas in dark mode
(white outline in every theme).

## Results

- Handle: 6px grip with stacked names in the divider color and a white outline;
  names hide under 32px of side room and cut with a vertical ellipsis to fit the
  stage height. Names are `pointer-events: none`, so only the grip grabs.
- Vertical drag: `useWipeGrip`, clamped in CSS; the height lives in
  `useQuestionReview` per round and clears when the round changes.
- Reveal: `useWipeNamesReveal` and `usePulseOnSplitMove`; the slider holds the
  names under the pointer.
- Onion skin `LayerBadge`, Side by side zoom on pane B, no slot dot.
- Performance: carried settings in a ref. At 4× CPU throttling in a dev build,
  a 60-step handle drag went from 65 long tasks (about 8s) to 11 to 20 (1 to
  1.5s), p95 frame 117ms to 33ms.
- No swipe navigation; item and candidate controls and keys remain.
- Verification: question-review unit tests, desktop E2E 12 passed, phone E2E 9
  passed.
- Independent review (2026-11-26) fixes: item navigation kept the comparison
  mode only through a dialog-render snapshot, so the dialog now owns a
  `CompareSettingsCarry`; leaving Wipe clears held names; stage shortcuts run
  only when the canvas itself has focus (Space on a zoom button no longer holds
  to compare); the handle stays inside the stage at both ends; forced colors
  keep the line, grip, and names visible; toolbar sliders name their thumb and
  have a 44px phone hit area; choice images show their choice in the single
  stage caption; the phone stepper shows B's position; the late-answer header
  wraps at 320px; pending-question listings include `image_groups`; and parent
  questions reject any image field before typed decoding.
- The phone (pinned) Wipe split now carries across items and questions: the
  scrubbed position is copied into the comparison's wipe value, and Fit places
  the line there.
