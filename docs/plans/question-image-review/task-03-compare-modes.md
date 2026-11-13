---
id: "03-compare-modes"
title: "Add A/B comparison modes to the review stage"
status: done
wave: 2
depends_on:
  - "02-review-surface"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-005
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.2
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.3
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.4
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.5
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.6
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.7
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.8
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.9
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.10
system_design:
  - ../../specs/tasks/system-design/question-image-review.md
---

# Task 03: Add A/B comparison modes to the review stage

## Summary

Let the operator see exactly what changed between a baseline and a candidate:
Toggle, Wipe, Side by side, Onion skin, and Difference, all on bytes already loaded.

## In scope

- `useReviewCompare` state, A and B selectors, Swap, and the compare toolbar
  (`ToggleGroup` on desktop, `Select` on phones).
- Toggle, Wipe, Side by side (stacked on phones), and Onion skin layers that share
  the work order 02 viewport transform.
- Size display and top-left alignment for different dimensions.
- `lib/question-review/pixel-diff.ts` (pure) and `diff.worker.ts`, with the
  tolerance slider, changed count and percentage, generation fencing, worker
  termination, and the unavailable state.
- Stage keys `t`, `s`, `[`, and `]`. Status announcements for mode and shown image.

## Out of scope

Annotations, server-side comparison, and downscaled difference fallbacks.

## Acceptance

1. With two or more loaded images, all five modes render. Switching among Toggle,
   Wipe, Side by side, and Onion skin, flipping, or swapping keeps the zoom and pan.
2. Difference highlights changed pixels at full resolution, reports count and
   percentage, honors tolerance, counts out-of-bounds pixels for different sizes,
   and shows an unavailable state when the worker cannot run. The form stays responsive.
3. No new network request is made for comparison beyond the segment's image reads.

## UI preview

Use [UI-02](plan.md#ui-02-desktop-review-view-side-by-side),
[UI-03](plan.md#ui-03-phone-review-view), and the Difference part of
[UI-04](plan.md#ui-04-difference-and-annotation-states).

```text
| [Toggle|Wipe|Side|Onion|Diff]  Image A [1|2]  Image B [1|2]  [Swap]    |
| [-] 100% [+] [Fit] [Actual size]        A 1440×900  B 1440×912           |
```

## Verification

Write failing tests first. `pixel-diff.test.ts` covers identical images, a single
changed pixel, tolerance edges, an alpha-only change, and different sizes.
Component tests cover view preservation across modes and stage keys staying in scope.

```bash
(cd apps/web && pnpm exec vitest run lib/question-review components/task/chat/question-review)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint --deny-warnings lib/question-review components/task/chat/question-review)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/question-image-review.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-question-image-review.spec.ts)
```

Extend both E2E specs with fixture images that differ by a known number of
pixels. Assert the reported count, mode switching with a preserved zoom, the
wipe slider by keyboard, and stacked Side by side on the phone.

## Likely files

- `apps/web/components/task/chat/question-review/` (compare toolbar, layers, `use-review-compare.ts`)
- New `apps/web/lib/question-review/pixel-diff.ts`, `diff.worker.ts`, and tests
- `apps/web/src/locales/*/task.json`
- E2E specs and PNG fixtures under `apps/web/e2e/`

## Dependencies and risks

Depends on work order 02. Risks: worker bundling under Vite (follow
`diff-worker-pool-provider.tsx`); jsdom lacks `OffscreenCanvas`, so keep the
diff logic pure and test the worker path in E2E; memory on 16-megapixel images.

## Results

Implemented on 2026-11-07. `lib/question-review/compare.ts` (reducer),
`pixel-diff.ts`, `diff.worker.ts`, and `diff-worker.ts`; stage components
`review-compare-*` and hooks `use-review-compare.ts` and `use-review-diff.ts`.
Mode and image pickers use `ToggleGroup` on phones too; the toolbar wraps.
The worker transfers an RGBA mask buffer that the stage draws into a canvas in
the shared layer.

Verification:

- Unit: reducer, full-resolution diff (single pixel, tolerance edge, alpha, size
  union), diff hook with a fake worker (debounce, stale results, unavailable,
  terminate), and overlay comparison tests: pass.
- Desktop E2E: all five modes, view preserved across modes and swap, wipe by
  keyboard, a real worker diff reporting exactly 40 changed pixels (0.25%), no
  new image requests while comparing, and a size mismatch counting 640
  out-of-bounds pixels: pass.
- Phone E2E: stacked Side by side and 44px mode controls: pass.
- Not measured: memory and timing for 16-megapixel images on a real phone.
