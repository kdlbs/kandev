---
id: "03-shared-selection-comment-surface"
title: "Shared selection comment surface"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SELECTION-COMMENT-SURFACE-001
acceptance_criteria:
  - AC-UI-SELECTION-COMMENT-SURFACE-001.1
  - AC-UI-SELECTION-COMMENT-SURFACE-001.2
  - AC-UI-SELECTION-COMMENT-SURFACE-001.3
  - AC-UI-SELECTION-COMMENT-SURFACE-001.4
  - AC-UI-SELECTION-COMMENT-SURFACE-001.5
  - AC-UI-SELECTION-COMMENT-SURFACE-001.6
  - AC-UI-SELECTION-COMMENT-SURFACE-001.7
  - AC-UI-SELECTION-COMMENT-SURFACE-001.8
system_design:
  - ../../specs/ui/system-design/selection-comment-surface.md
---

# Task 03: Shared selection comment surface

## Summary

Turn `PlanSelectionPopover` into `components/shared/selection-comment/`
`SelectionCommentSurface`, adding `contextLabel`, `evidence`, and a controlled
draft. Move the Plan panel, chat and Office comments, question-review regions,
the CodeMirror editor, and the Markdown preview onto it. Then delete
`plan-selection-popover.tsx` and `editor-comment-popover.tsx`.

## In scope

- New folder with the split files from the design. Drag and dismiss come
  from `use-draggable-popover.ts`, which gains a `disabled` flag. Remove the
  private copies.
- Migrate the importers: `task-plan-panel.tsx`, `message-comment-controls.tsx`,
  `region-comment-popover.tsx`, `codemirror-code-editor.tsx`, and
  `markdown-preview-content.tsx`.
- `anchorBelowRect` in `floating-selection-position.ts`, used by
  `regionAnchor`.
- Move and adapt the tests. Remove translation keys that are no longer used,
  after searching for each one.

## Out of scope

- Web-preview adoption (task 04), Monaco view-zone forms, and
  `CommentViewPopover`.
- Changes to test IDs or Add/Run semantics for existing surfaces.

## ASCII UI preview

UI-05 from the [plan](plan.md#ui-05-code-editor-and-markdown-preview): desktop
is unchanged. A coarse pointer opens the shared Drawer with the line-range
label and Add/Run.

## Implementation acceptance conditions

1. All six listed surfaces import only `SelectionCommentSurface`, and
   `rg "PlanSelectionPopover|EditorCommentPopover" apps/web` returns nothing.
2. The unit tests cover `contextLabel`, custom `evidence`, the controlled draft
   surviving remount, `false` keeping the surface open with an error, edit
   mode hiding Run, and dismissal blocked while a mutation is pending.
3. The existing desktop and mobile E2E specs for these surfaces pass.
   `mobile-file-viewer` asserts that the Markdown preview dock opens the
   Drawer.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/shared/selection-comment components/task/use-draggable-popover.test.ts components/task/markdown-preview-content components/editors components/task/chat/messages components/task/chat/question-review components/task/task-plan-panel
pnpm run typecheck && pnpm run lint && pnpm run i18n:check
pnpm e2e:run --host --shards 1 --project chromium tests/chat/markdown-preview.spec.ts tests/chat/agent-message-comments.spec.ts tests/session/task-plan-comments.spec.ts tests/chat/question-image-review.spec.ts
pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-file-viewer.spec.ts tests/chat/mobile-agent-message-comments.spec.ts tests/session/mobile-task-plan-comments.spec.ts tests/chat/mobile-question-image-review.spec.ts
```

## Likely files

- New `apps/web/components/shared/selection-comment/*`
- Deleted `apps/web/components/task/plan-selection-popover.tsx` and `editor-comment-popover.tsx` (move their tests)
- `apps/web/components/task/use-draggable-popover.ts`, `floating-selection-position.ts`
- The five importers listed above, `apps/web/src/locales/*/task.json`, and `apps/web/eslint.i18n.options.mjs` (append new paths; never remove)

## Risks

- `i18nGuardFiles` may list deleted paths, and entries must never be removed.
  Check how `check-guard-allowlist.mjs` treats a missing file before deleting
  anything, and add the new paths.
- Desktop width changes from 384 to 340 px for the editor surfaces. Visual
  only.

## Results

Done.

- `components/shared/selection-comment/` replaces `PlanSelectionPopover` and
  `EditorCommentPopover`; all six surfaces import it.
- Placement measures the rendered height (found by the WebGL E2E: a thumbnail
  pushed Add off-screen) and caps the desktop surface at the viewport height.
- Drag/dismiss stay with the surface; `use-draggable-popover.ts` remains for
  `CommentViewPopover` because their Escape semantics differ.
- Four unused shortcut keys were removed from every locale.

Commands: targeted Vitest (116 files, 980 tests) pass; Chromium
`markdown-preview`, `agent-message-comments`, `task-plan-comments`,
`question-image-review` and mobile-chrome `mobile-file-viewer`,
`mobile-agent-message-comments`, `mobile-task-plan-comments`,
`mobile-question-image-review` pass.
