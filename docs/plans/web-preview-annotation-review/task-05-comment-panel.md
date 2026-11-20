---
id: "05-comment-panel"
title: "Comment panel and toolbar toggle"
status: done
wave: 3
depends_on:
  - "02-shared-comment-surface"
  - "04-draft-lifecycle"
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.1
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.3
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-04.md
---

# Task 05: Comment panel and toolbar toggle

## Summary

Add `PreviewCommentPanel`, docked beside the preview on fine pointers (overlay
under 720 pixels) and a bottom sheet on coarse pointers, and turn the toolbar
count into a panel toggle. Comments are written in the panel when it is open.

## In scope

- Extract the collection list body from `preview-feedback-collection.tsx` and
  render it in both the header popover and the panel, after an Unsaved group.
- Inline entries with `SelectionCommentComposer` for new captures and saved
  edits; scroll into view and focus on capture.
- Toolbar toggle with pressed state, count, and Show or Hide comments label.
- `useLocalStorageBoolean("kandev:preview-comment-panel-open", true)` on fine
  pointers; coarse pointers start hidden.
- Row layout in `BrowserPanel` and `HtmlPreviewContent` hosts; an open entry
  moves to the floating box with its text when the panel hides.
- Selecting an entry sends `highlighted_id`.

## Out of scope

- E2E specs and public docs (task 06).

## Acceptance conditions

1. Component tests cover open by default, toggle, remembered state, overlay
   under 720 pixels, and the coarse-pointer sheet starting hidden.
2. A capture with the panel open adds a focused inline entry; with it hidden,
   the floating box opens.
3. The header popover still lists the collection outside the mode.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/task/inspector components/task/browser-panel.test.tsx components/task/html-preview-content.test.tsx
pnpm run typecheck && pnpm run lint && pnpm run i18n:check
```

## Likely files

- New `apps/web/components/task/inspector/preview-comment-panel.tsx`, `.test.tsx`
- `apps/web/components/task/inspector/preview-feedback-collection.tsx`
- `apps/web/components/task/inspector/preview-annotation-toolbar.tsx`,
  `preview-annotation-layer.tsx`, `preview-annotation-surfaces.tsx`,
  `preview-unsaved-drafts.tsx`
- `apps/web/components/task/browser-panel.tsx`, `html-preview-content.tsx`
- `apps/web/src/locales/*/task.json`

## Risks

- Docking reflows the page; document-anchored outlines can shift after a
  toggle, as after any resize.
- Keep each component under 200 lines; the panel, its entry, and the Unsaved
  group are separate components.
