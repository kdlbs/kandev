---
id: "04-toolbar"
title: "Movable, outlined toolbar with an accented panel toggle"
status: done
wave: 2
depends_on: ["03-floating-comments"]
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-005
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-005.2
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-05.md
---

# Task 04: Movable, outlined toolbar with an accented panel toggle

## Summary

Make the fine-pointer toolbar stand out, let the user drag it anywhere over
the preview, remember its position, and accent the panel toggle.

## In scope

- Move `useDrag` and its drag shield from `selection-comment/surface-parts.tsx`
  to `hooks/use-drag-offset.ts`; the comment surface imports it from there.
- `usePreviewToolbarPosition`: fractions of free space in localStorage
  (`kandev:preview-annotation-toolbar-position`), clamped to 0 to 1, default
  top center, reset on handle double-click, arrow keys move 16 pixels.
- Toolbar: opaque background, `ring-2 ring-primary`, `shadow-lg`, and a grip
  handle with an accessible name, on fine pointers only.
- `PreviewAnnotationHost` renders the toolbar over the frame box, above the
  overlay panel; the layer no longer renders it.
- Panel toggle: outlined primary style, tinted while pressed, with
  `IconLayoutSidebarRight` beside the count.
- Locale keys for the handle's name and reset hint in seven locales and
  pseudo.

## Out of scope

- The coarse-pointer bottom bar, which stays fixed.

## Acceptance conditions

1. Dragging the handle moves the toolbar, the position survives a remount,
   and it stays inside the frame after the frame shrinks.
2. Double-click resets to top center; arrow keys move it.
3. The toolbar stays above the overlay panel and keeps its test IDs.

## ASCII UI preview

Plan UI-01 (outline, handle, accented toggle) and UI-02 (moved toolbar).

## Verification

```bash
cd apps/web
pnpm exec vitest run hooks/use-preview-toolbar-position.test.ts hooks/use-drag-offset.test.ts components/task/inspector/preview-annotation-layer.test.tsx components/shared/selection-comment
pnpm run i18n:check
```

## Likely files

- `apps/web/hooks/use-drag-offset.ts`, `.test.ts`
- `apps/web/hooks/use-preview-toolbar-position.ts`, `.test.ts`
- `apps/web/components/shared/selection-comment/surface-parts.tsx`
- `apps/web/components/task/inspector/preview-annotation-toolbar.tsx`
- `apps/web/components/task/inspector/preview-annotation-layer.tsx`
- `apps/web/components/task/inspector/preview-comment-panel.tsx`
- `apps/web/src/locales/*/task.json`

## Risks

- Moving the toolbar out of the layer can break the layer's Escape handling
  for focus on the toolbar; keep the toolbar inside the element the Escape
  handler treats as the preview.
