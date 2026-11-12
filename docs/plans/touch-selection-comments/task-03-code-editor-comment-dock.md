---
id: "03-code-editor-comment-dock"
title: "Dock CodeMirror and Monaco Comment actions"
status: done
wave: 2
depends_on:
  - "01-chat-comment-dock"
plan: "plan.md"
requirements:
  - REQ-UI-SELECTION-COMMENT-ACTIONS-001
acceptance_criteria:
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.1
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.2
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.3
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.4
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.5
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.6
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.7
  - AC-UI-SELECTION-COMMENT-ACTIONS-001.8
system_design:
  - ../../specs/ui/system-design/selection-comment-actions.md
---

# Task 03: Dock CodeMirror and Monaco Comment actions

## Summary

Use the shared touch dock in both file-editor engines. Drive its availability
from eligible engine selections without requiring a mouse release or stored mouse position.

## In scope

- Add a failing component regression for each editor. A nonempty selection
  without mouse events must expose the touch action. Changing the range must
  update the quote while leaving placement independent of that range.
- Use `editorAreaRef` as each editor's owner viewport. Keep the existing
  fine-pointer floating buttons and their coordinates.
- Extend CodeMirror's selection updates to publish valid touch quote and line
  state. Reuse Monaco's existing `onDidChangeCursorSelection` state for touch availability.
- Preserve focus and selection through dock activation, existing keyboard
  shortcuts, and all comment anchors, repository identity, and submission callbacks.
- Keep CodeMirror's `EditorCommentPopover` and Monaco's inline comment view zone.
  An open comment editor suppresses the dock. Identity changes remove stale actions.
- Add browser coverage for each engine in phone and desktop projects. Use the
  existing `code-editor` resolver preference, then restore its baseline.
- Prove keyboard-edge movement, one active action, 44px hit targets, owner
  bounds, final-line reachability, and actual pending comment creation.

## Out of scope

Editor replacement, LSP, gutter-comment redesign, view-zone redesign, diff
viewers, file persistence changes, new dependencies, and public plugin contracts.

## Acceptance

- Both engine regressions fail before production changes, then pass for
  touch-only selection, range updates, correct quote/lines, and single activation.
- Mobile browser tests complete the comment flow in both real engines. The
  dock stays within the visible editor and clears keyboard/navigation/safe areas.
- Desktop comment flows and existing cursor, preview-toggle, and repository
  comment-scoping tests remain green. Package results retain accurate pending/done statuses.

## ASCII UI preview

UI-01 and UI-02 excerpt from the [full plan](plan.md#ascii-ui-preview):

```text
UI-01: Phone/coarse editor     UI-01: Wider fine-pointer editor
+--------------------------+  +--------------------------+
| Existing file toolbar    |  | Existing file toolbar    |
|--------------------------|  | [selection] [Comment]    |
| Editable code            |  |                          |
| [selected lines]         |  +--------------------------+
| Native selection menu    |
|--------------------------|
| [ Comment ]              |  Dock within this editor pane
+--------------------------+
| Navigation / safe area   |
+--------------------------+

UI-02: CodeMirror -> current comment popover.
UI-02: Monaco -> current inline comment view zone.
Exact selected quote and line anchors pass to either editor.
```

Both engines keep their existing scroll owner. The dock reserves content
clearance and follows keyboard geometry. Structure is required, while spacing
is illustrative. Criteria `.1` through `.8` apply to each engine separately.

## Verification

Use TDD with Task 01's shared dock. Execute after Task 02 by default.
From the repository root, after new tests exist:

```bash
(cd apps/web && pnpm exec vitest run \
  components/editors/codemirror/codemirror-code-editor.comments.test.tsx \
  components/editors/monaco/monaco-code-editor.comments.test.tsx \
  components/editors/codemirror/codemirror-code-editor.cursor.test.tsx \
  components/editors/codemirror/codemirror-code-editor.preview.test.tsx \
  components/editors/scoped-editor-comments.test.ts \
  components/task/file-editor-content.test.tsx \
  components/shared/selection-comment-dock-position.test.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/task/mobile-file-selection-comments.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project chromium \
  tests/task/file-selection-comments.spec.ts -- --retries=0)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings \
  components/editors/codemirror/codemirror-code-editor.tsx \
  components/editors/codemirror/use-codemirror-editor-state.ts \
  components/editors/codemirror/codemirror-code-editor.comments.test.tsx \
  components/editors/monaco/monaco-code-editor.tsx \
  components/editors/monaco/use-monaco-editor-state.ts \
  components/editors/monaco/monaco-code-editor.comments.test.tsx \
  e2e/tests/task/mobile-file-selection-comments.spec.ts \
  e2e/tests/task/file-selection-comments.spec.ts)
(cd apps/web && pnpm run i18n:check)
git diff --check
```

Run the plan's documentation validation and offline coverage preflight before
delivery. A shared-helper change also requires its relevant tests and an affected
earlier consumer scenario. Otherwise, use the results already recorded by Tasks
01 and 02. Do not add a broad verification pass.

Capture each phone editor with its dock during the focused browser tests.
Record physical Android Chrome and iOS Safari results or their unavailability.
Browser emulation is not native-menu evidence.

## Files likely touched

- `apps/web/components/editors/codemirror/codemirror-code-editor.tsx`
- `apps/web/components/editors/codemirror/use-codemirror-editor-state.ts`
- `apps/web/components/editors/codemirror/codemirror-code-editor.comments.test.tsx` (new)
- `apps/web/components/editors/monaco/monaco-code-editor.tsx`
- `apps/web/components/editors/monaco/use-monaco-editor-state.ts`
- `apps/web/components/editors/monaco/monaco-code-editor.comments.test.tsx` (new)
- `apps/web/e2e/tests/task/mobile-file-selection-comments.spec.ts` (new)
- `apps/web/e2e/tests/task/file-selection-comments.spec.ts` (new)
- A sibling shared E2E helper, if both new specs need the same fixture code.
- This work order and plan for status and result updates.

## Dependencies

Task 01 supplies the shared host, dock, and geometry contract. Task 02 is the
preceding scheduled work order, without an additional technical dependency.

## Risks

- CodeMirror currently snapshots its quote after mouse release. Touch tests
  must exercise engine selection updates without synthesizing a mouse event.
- Monaco already stores a quote, but still gates its trigger on mouse release.
  The touch branch must not inherit that gate or use stale line state.
- A button can steal editor focus before the quote reaches the existing handler.
- Editor preference and seeded file changes must not leak into later E2E tests.

## Parallelism

`sequential`

## Inputs

- [Selection comment requirements](../../specs/ui/requirements/selection-comment-actions.md).
- [Shared design](../../specs/ui/system-design/selection-comment-actions.md).
- `CodeMirrorOverlays`, `useCodeMirrorEditorState`, and current cursor/preview tests.
- `MonacoFloatingCommentButton`, `useMonacoEditorState`, and existing selection callbacks.
- `FileEditorContent` and `useEditorProvider("code-editor")` for provider selection.
- `e2e/tests/task/file-editor-save-refresh.spec.ts` for real file-editor setup.
- Task 01's shared dock and UI-01/UI-02 in the plan.

## Results

Done. CodeMirror and Monaco use the shared touch dock while retaining their
existing editor comment flows. Both providers passed mobile and desktop E2E
coverage, including the final-line quote and saved comment assertions. Focused
editor unit tests passed in the 14-file, 71-test suite.

Review remediation: CodeMirror clears touch ownership and its pending capture on
a real mouse/pen pointerdown in the same editor, restoring the desktop Comment
trigger for the new quote. Monaco receives the shared dock's measured clearance
as editor padding. The hybrid-input regression and both focused mobile editor
cases passed; each verified the final code line above the dock after visual
viewport shrink and pan while the editor owner's layout bounds stayed fixed.
