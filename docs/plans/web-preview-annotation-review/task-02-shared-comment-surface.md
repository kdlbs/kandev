---
id: "02-shared-comment-surface"
title: "Shared comment surface: close, composer, drag shield"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WEB-PREVIEW-FEEDBACK-006
acceptance_criteria:
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.3
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.4
  - AC-TASKS-WEB-PREVIEW-FEEDBACK-006.6
system_design:
  - ../../specs/tasks/system-design/web-preview-feedback-04.md
---

# Task 02: Shared comment surface: close, composer, drag shield

## Summary

Give the shared `SelectionCommentSurface` an opt-in close control, extract its
composer body so the comment panel can render it inline, and keep header drags
working over iframes.

## In scope

- Optional `onCancel` prop: an X button in the desktop header and a Close
  button in the drawer header, with translated labels. Absent prop renders
  exactly as today.
- Extract `SelectionCommentComposer` (evidence, textarea, messages, actions,
  `usePopoverComposer`) and use it in the desktop popover and the drawer.
- `useDrag` adds a transparent full-viewport shield only while dragging.

## Out of scope

- Preview wiring (tasks 04 and 05).

## Acceptance conditions

1. With `onCancel`, the X and Close controls call it; without it, existing
   snapshots and tests are unchanged.
2. The shield exists only during a drag and is removed on mouseup.
3. Chat, plan, diff, editor, and question surfaces keep passing their tests.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/shared/selection-comment components/task/chat components/editors components/diff
pnpm run typecheck && pnpm run lint && pnpm run i18n:check
```

## Likely files

- `apps/web/components/shared/selection-comment/selection-comment-surface.tsx`, `.test.tsx`
- `apps/web/components/shared/selection-comment/surface-parts.tsx`
- `apps/web/components/shared/selection-comment/surface-types.ts`
- `apps/web/src/locales/*/task.json` (close label, seven locales and pseudo)

## Risks

- `selection-comment-surface.tsx` is 367 lines; the extraction should shrink
  it. Keep the composer in its own file if it grows past 200 lines.
