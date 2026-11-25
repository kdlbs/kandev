---
id: "03-floating-comments"
title: "Floating comments only and closing on tool change"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.2
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.4
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-05.md
---

# Task 03: Floating comments only and closing on tool change

## Summary

Write every comment in the shared floating box, turn the panel into a list,
and close an open comment when the user picks another tool.

## In scope

- Remove the panel's inline composers: `renderComposer`, the inline modes of
  `PendingFeedbackItem` and `DraftRow`, `InlineDraftComposer`,
  `InlineItemEditor`, and `inlineComments`. Delete
  `SelectionCommentComposer` and its test if nothing else uses it.
- `PreviewAnnotationLayer` always renders the floating draft and saved-item
  surfaces; `surface_open` is sent whenever a comment is open.
- Panel Edit and entry picks call `selectCapture` and `openCapture`; the open
  entry stays marked through `activeId`.
- `editingAnchor` uses the region outline, then the `highlighted` marker
  rectangle from `region-layout` when task 02 has landed, then the fallback.
  Until then, it keeps today's region-or-fallback behavior.
- `selectTool(tool)`: a different tool closes the open comment through
  `closeComment()` before `setTool`; the same tool does nothing. The toolbar
  calls it.

## Out of scope

- Toolbar styling and movement (task 04).

## Acceptance conditions

1. With the panel open, a capture opens the floating box and the panel has no
   comment field; Edit in the panel opens the floating box.
2. With a text comment open, choosing Region closes it by the part 4 rules
   and the bridge receives `screenshot` mode at once.
3. A press in the page closes an open comment with the panel open or hidden.

## ASCII UI preview

Plan UI-01: the panel lists entries only, and the comment is in the floating
box.

## Verification

```bash
cd apps/web
pnpm exec vitest run hooks/use-preview-capture.test.ts hooks/use-preview-capture-lifecycle.test.ts components/task/inspector/preview-annotation-layer.test.tsx components/task/inspector/preview-feedback-collection.test.tsx components/shared/selection-comment
```

## Likely files

- `apps/web/hooks/use-preview-capture.ts`, `.test.ts`
- `apps/web/hooks/use-preview-annotation-mode.ts`
- `apps/web/components/task/inspector/preview-comment-panel.tsx`
- `apps/web/components/task/inspector/preview-inline-comments.tsx`
- `apps/web/components/task/inspector/preview-feedback-collection.tsx`
- `apps/web/components/task/inspector/preview-unsaved-drafts.tsx`
- `apps/web/components/task/inspector/preview-annotation-layer.tsx`, `.test.tsx`
- `apps/web/components/task/inspector/preview-annotation-surfaces.tsx`
- `apps/web/components/shared/selection-comment/selection-comment-composer.tsx`

## Risks

- Removing the inline mode must not change the header popover list outside
  annotation mode, which renders the same collection body.
