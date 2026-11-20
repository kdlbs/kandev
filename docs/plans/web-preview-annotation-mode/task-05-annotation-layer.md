---
id: "05-annotation-layer"
title: "Annotation layer, toolbar, and regions"
status: done
wave: 3
depends_on:
  - "01-replace-screenshot-capture"
  - "02-shared-region-annotation"
  - "04-drafts-and-mode-state"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-004
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-004.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.5
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.6
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.7
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-03.md
---

# Task 05: Annotation layer, toolbar, and regions

## Summary

Render annotation mode: the ring, the toolbar on desktop and phone, region
outlines with move and resize, the comment-on-selection button or dock, and
unsaved drafts in the pending panel. Remove the one-shot capture menu.

## In scope

- `PreviewAnnotationLayer` in `BrowserPanel` (desktop and the mobile focused
  viewer) and `HtmlPreviewContent`.
- `PreviewAnnotationToolbar`: tool group, count button, Exit; floating top on
  fine pointers, bottom bar on coarse pointers.
- `RegionOverlay` from task 02, positioned from `region-layout`, with handles
  in the Region tool, click to open, and recapture on release.
- `EditorSelectionCommentTrigger` and `SelectionActionDock` for selections.
- Header Annotate becomes a toggle with the count and an unsaved indicator.
  Capture choices, Cancel, and the Resume/Discard row are removed.
- Pending panel Unsaved group with Resume and Discard.
- Locale keys in seven languages plus pseudo; remove keys that become unused.
- E2E: rewrite the capture steps in the existing preview specs for the
  toolbar, and add `preview-annotation-mode.spec.ts` (desktop) and mobile
  steps in `mobile-html-preview.spec.ts`.

## Out of scope

- Backend, bridge, and shared-primitive changes beyond wiring.

## Acceptance conditions

1. Desktop E2E covers every desktop flow in the design's verification section.
2. Mobile E2E covers the bottom toolbar, selection dock, drawing a region, and
   Exit.
3. Existing preview, HTML preview, and question-review specs pass.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/task/inspector components/shared/region-annotation hooks/use-preview-capture.test.ts hooks/use-preview-annotation-mode.test.ts components/task/browser-panel.test.tsx
pnpm run typecheck && pnpm run lint && pnpm run i18n:check
pnpm e2e:run --host --shards 1 --project chromium tests/preview tests/chat/html-preview.spec.ts tests/chat/question-image-review.spec.ts
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-html-preview.spec.ts
```

## Likely files

- `apps/web/components/task/inspector/preview-feedback-controls.tsx`, `preview-feedback-collection.tsx`, tests
- New `apps/web/components/task/inspector/preview-annotation-layer.tsx`, `preview-annotation-toolbar.tsx`
- `apps/web/components/task/browser-panel.tsx`, `html-preview-content.tsx`
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/preview/*`, `apps/web/e2e/tests/task/mobile-html-preview.spec.ts`

## Risks

- The layer must not block wheel or touch scrolling over the iframe. Keep it
  `pointer-events: none` except for its controls.
- The comment surface's outside-click dismissal must not fire for clicks on
  region handles or the toolbar, or every resize would close the surface.
