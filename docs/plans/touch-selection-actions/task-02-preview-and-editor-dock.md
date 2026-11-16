---
id: "02-preview-and-editor-dock"
title: "Docked touch comment action in Markdown preview and code editors"
status: done
wave: 2
depends_on:
  - "01-shared-dock-and-chat-messages"
plan: "plan.md"
requirements:
  - REQ-UI-TOUCH-SELECTION-ACTIONS-001
acceptance_criteria:
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.1
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.2
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.3
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.5
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.6
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.9
system_design:
  - ../../specs/ui/system-design/touch-selection-actions.md
---

# Task 02: Docked Touch Comment Action in Markdown Preview and Code Editors

## Summary

Adopt `SelectionActionDock` from task 01 in the Markdown file preview and the
CodeMirror and Monaco file editors on coarse pointers.

## In scope

- `MarkdownPreviewCommentOverlays`: render the dock instead of
  `markdown-preview-comment-button` on touch; enable
  `useTouchSelectionTracking` in `useMarkdownPreviewComments`.
- `codemirror-code-editor.tsx` and `monaco-code-editor.tsx`: render the dock
  instead of `floating-comment-btn` on touch, docked to the editor wrapper.
- Keep desktop buttons, shortcuts, and composers unchanged.
- Update `e2e/tests/task/mobile-file-viewer.spec.ts` to use the docked button.

## Out of scope

- Composer redesign for the preview or editors.
- Shared primitives (task 01).

## ASCII UI preview

See [plan.md UI-03](plan.md#ascii-ui-preview).

```text
|  Run [pnpm install] from apps/.      |
|======================================|  dock at preview/editor bottom
|                        [ (C) Comment]|
```

## Acceptance

- Failing component tests first for each surface: coarse pointer renders the
  dock and no floating button; fine pointer renders the floating button.
- `mobile-chrome` E2E: the Markdown preview selection shows the docked Comment
  button at the preview bottom; tapping it opens the composer and saves a
  comment.

## Verification

```bash
cd apps/web
pnpm exec vitest run \
  components/task/markdown-preview-content \
  hooks/domains/comments/use-markdown-preview-comments \
  components/editors/codemirror \
  components/editors/monaco
pnpm e2e:run --project mobile-chrome tests/task/mobile-file-viewer.spec.ts
pnpm e2e:run --project chromium tests/chat/markdown-preview.spec.ts
pnpm run typecheck
pnpm run lint
```

## Files likely touched

- `apps/web/components/task/markdown-preview-content.tsx`
- `apps/web/hooks/domains/comments/use-markdown-preview-comments.ts`
- `apps/web/components/editors/codemirror/codemirror-code-editor.tsx`
- `apps/web/components/editors/codemirror/use-codemirror-editor-state.ts`
- `apps/web/components/editors/monaco/monaco-code-editor.tsx`
- Matching `*.test.tsx` / `*.test.ts` files
- `apps/web/e2e/tests/task/mobile-file-viewer.spec.ts`

## Dependencies

Task 01 (`SelectionActionDock`, `useTouchSelectionTracking`,
`useDockedBarPosition`).

## Risks

- The Markdown preview overlay root may sit inside a Drawer on phones; portal
  the dock into that root so it stays interactive.
- Monaco's touch selection support is limited; if it exposes no selection on
  touch, the dock simply never opens, which satisfies `.9`.

## Parallelism

`sequential`

## Results

Added `EditorSelectionCommentTrigger`, shared by CodeMirror, Monaco, and the
Markdown preview: floating button on fine pointers, `SelectionActionDock` on
touch. CodeMirror follows touch selections from its selection update listener
(`codemirror-selection.ts`); Monaco shows the dock for any current selection on
touch. The Markdown preview follows `selectionchange` on touch and keeps the
docked trigger when its scroller scrolls. `handleFloatingButtonClick` accepts
an optional event in both editors.

Verification:

- New component and hook tests for the preview, editor trigger, CodeMirror
  editor, and CodeMirror selection helper pass. They were written before the
  production changes but their initial failing run was not recorded.
- `mobile-chrome` E2E: `mobile-file-viewer` (with the new dock-position
  assertion) and `mobile-agent-message-comments` passed (14 tests).
- `chromium` E2E: `markdown-preview` and `agent-message-comments` passed
  (13 tests), covering unchanged desktop behavior.
- `pnpm run typecheck`, `pnpm run lint`, and Prettier checks passed.
