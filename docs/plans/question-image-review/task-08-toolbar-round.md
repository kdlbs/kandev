---
id: "08-toolbar-round"
title: "Inspector handles, zoom to changes everywhere, simpler desktop toolbar"
status: done
wave: 7
depends_on:
  - "07-usability-round"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-002
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-004
  - REQ-TASKS-QUESTION-IMAGE-REVIEW-005
  - REQ-TASKS-QUESTION-IMAGES-003
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGE-REVIEW-002.5
  - AC-TASKS-QUESTION-IMAGE-REVIEW-004.1
  - AC-TASKS-QUESTION-IMAGE-REVIEW-005.13
  - AC-TASKS-QUESTION-IMAGES-003.6
system_design:
  - ../../specs/tasks/system-design/question-image-review.md
  - ../../specs/tasks/system-design/question-image-previews.md
---

# Task 08: Inspector handles, zoom to changes everywhere, simpler desktop toolbar

## Summary

Third test round, 2026-11-09.

## In scope

1. The thumbnail inspector shows large Previous and Next handles over the image
   edges; the small bottom arrows go; the counter stays.
2. Zoom to changes works in every comparison mode. Outside Difference it computes
   the changed area on demand, keeps it per image pair and tolerance, reports
   "No changes found", and briefly outlines the framed area.
3. Desktop toolbar: one comparison row (mode, an image-pair control with swap and
   candidate stepping, and the mode's action). On the image: a zoom cluster (zoom
   to changes, minus, a zoom level menu with Fit, 100%, 200%, 400%, and 800%,
   plus, and shortcut help), Mark area, and status chips. Double-click switches
   between Fit and 100%. Phones keep their layout.

## Out of scope

Phone layout changes, answer contract changes.

## Acceptance

1. Desktop and phone inspector handles step images; the counter remains.
2. Zoom to changes zooms in Toggle without switching to Difference.
3. The desktop review toolbar is one row; zoom controls are on the image.

## Verification

```bash
(cd apps/web && pnpm exec vitest run lib/question-review components/task/chat)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/question-image-review.spec.ts tests/chat/question-image-previews.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-question-image-review.spec.ts tests/chat/mobile-question-image-previews.spec.ts)
```

## Results

Implemented on 2026-11-09.

1. Inspector: `GalleryHandles` sit over the image edges inside the canvas wrapper
   and hide at the ends; `GalleryCounter` stays in the bottom row.
2. Zoom to changes: `useChangesZoom` computes on demand outside Difference, keeps
   the result per `aId:bId:tolerance`, flashes the outline for 1.5s, and reports
   "No changes found"; `ChangesButton` shows the state.
3. Desktop toolbar: one row (`ToggleGroup`, `ImagePairControl`, `ModeControls`).
   On the image: `ZoomCluster` (zoom to changes, minus, zoom level popover, plus,
   shortcut help), Mark area in `StageOverlayControl`, and `StatusChips`.
   Double-click switches Fit and 100%.

Two defects found while testing this round and fixed:

- Overlay controls inside the stage lost clicks to the stage's pointer capture;
  `StageOverlayControl` and the zoom chip stop pointer and double-click events.
- The zoom level popover kept its closing animation, so for 100ms it stayed the
  top layer and took Escape; the review popovers now close without an exit
  animation (`data-closed:animate-none!`).

Verification:

- Web unit: zoom-to-changes hook (on demand, cached, none found, flash), gallery
  handles, overlay placement, double-click, pickers: pass.
- Desktop E2E (11 tests): pass, including Zoom to changes in Toggle and handle placement.
- Phone E2E (7 tests): pass.
