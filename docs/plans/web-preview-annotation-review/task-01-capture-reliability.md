---
id: "01-capture-reliability"
title: "Capture reliability and busy outline"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.5
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-04.md
---

# Task 01: Capture reliability and busy outline

## Summary

Fix the dev-build hang, keep a region's outline visible with an animated
border while it is captured, capture the content under the rectangle at
release even if the page scrolls, and never leave a capturing state on screen.

## In scope

- `use-preview-screenshots`: set `mountedRef.current = true` in the effect
  body. Add a StrictMode `renderHook` regression test that fails today.
- A 15-second timeout around rasterization; on failure or timeout remove the
  capturing draft and set the screenshot error.
- `rasterizePreviewRegion`: an `onclone` callback that restores the recorded
  container scroll on the cloned scroll container.
- Drafts gain a `capturing` state: `screenshot-region-selected` adds the draft
  at once with no image, opens its comment, and fills the image when ready.
  Capturing drafts count toward the screenshot limit and cannot be saved yet.
- `RegionOverlay` gains `busyIndexes`; busy outlines show a rotating
  conic-gradient border, and a static dashed border under
  `prefers-reduced-motion`. Question review does not pass it.

## Out of scope

- The close reducer and numbering (task 04).
- Panel rendering of a capturing entry (task 05).

## Acceptance conditions

1. Under StrictMode, a region capture adds its image and clears the busy
   state.
2. A rasterization that never settles is removed after the timeout with the
   error shown.
3. A capture whose container scrolled after release renders the recorded
   scroll position.

## Verification

```bash
cd apps/web
pnpm exec vitest run hooks/use-preview-screenshots.test.ts hooks/use-preview-drafts.test.ts lib/preview-screenshot.test.ts components/shared/region-annotation
pnpm exec vitest run components/task/chat/question-review lib/question-review
pnpm run typecheck && pnpm run lint
```

## Likely files

- `apps/web/hooks/use-preview-screenshots.ts`, new `.test.ts`
- `apps/web/hooks/use-preview-drafts.ts`, `.test.ts`
- `apps/web/hooks/use-preview-capture.ts`
- `apps/web/lib/preview-screenshot.ts`, `.test.ts`
- `apps/web/components/shared/region-annotation/region-overlay.tsx`, `.test.tsx`
- `apps/web/app/globals.css` (the border animation)

## Risks

- Every draft consumer must handle a draft without an image: thumbnails,
  save, send gating, and the limit.
- `@property` is unsupported in some older browsers; the border then shows
  without rotation, which is acceptable.
