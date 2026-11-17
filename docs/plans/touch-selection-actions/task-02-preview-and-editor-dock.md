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
  comment. Visual-viewport resize and pan also keep the final Markdown,
  CodeMirror, and Monaco lines above the dock while `window.innerHeight` and
  owner layout bounds stay fixed.

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

## PR fixup follow-up (2026-11-17)

The shared dock now reports viewport-relative clearance as the overlap between
its top edge and the owner's bottom edge, bounded below by the dock height.
The shared reservation and Monaco editor padding use that clearance; CodeMirror
reserves it in the editor scroller. Visual viewport resize and pan updates are
covered while `window.innerHeight` and the owner layout bounds remain fixed.
The Markdown preview uses the shared reservation. Office extracts use the
composer slot recorded in tasks 01 and 04. Mobile file viewer editors now keep
the code read-only and hide the desktop toolbar while retaining comment
selection.

CodeMirror transfers touch-selection ownership back to mouse and pen input in
the same editor. Markdown selection capture cancels and generation-fences
pending callbacks when ownership ends, an unrelated outside action dismisses
the dock, the composer opens, or preview identity changes. Opening the
composer preserves the latest validated quote.

Verification:

- Focused Vitest: 41 tests passed across seven files; a second focused run after
  the lint refactor passed 28 tests across five files.
- Changed-source and E2E lint passed. `pnpm run typecheck` passed.
- Managed `mobile-chrome` E2E passed for `mobile-thread-extract-comments`
  (2 tests), `mobile-file-viewer` (12 tests),
  `mobile-markdown-selection-comments` (2 tests), and
  `mobile-file-selection-comments` (2 tests). The editor cases assert that the
  final line stays above the dock after simulated keyboard clipping with
  unchanged layout bounds.
- The managed E2E build completed for the backend and web assets. Native
  Android Chrome and iOS Safari were not exercised.

### CI follow-up (2026-11-17)

The mobile viewer assertions, including the natural-order file-tree case, now
accept both CodeMirror and Monaco markup.
Read-only mobile viewers do not acquire LSP connections. Monaco lays out a
newly mounted editor before revealing a pending cursor, including requests
without a line flash. CodeMirror receives read-only mode through the editor
component prop, avoiding a duplicate `@codemirror/state` extension instance.
The PR popover geometry case waits for its finite menu animation before
measuring the conflict glyph.

Verification:

- Focused Vitest for CodeMirror workspace sync and Monaco cursor/LSP navigation:
  15 passed across three files.
- Managed `mobile-chrome` E2E for eight changed mobile viewer specs:
  14 passed in 1.6 minutes. This run built the backend and Vite assets.
- Focused Chromium E2E for the conflicted PR row's 8 px glyph geometry:
  1 passed after waiting for finite menu animations.
- After the final Monaco session-identity adjustment, the focused Vitest set
  passed again (15 tests) and the mobile LSP suppression/requested-line E2Es
  passed again (2 tests).
- Current-head CI follow-up: the focused `use-file-editors` and CodeMirror
  selection tests passed (23 tests across two files), and the
  exact mobile file-tree natural-order E2E passed (1 test, retries disabled).
- Native Android Chrome and iOS Safari remain untested.
