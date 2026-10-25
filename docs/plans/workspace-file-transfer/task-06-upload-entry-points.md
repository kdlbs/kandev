---
id: "06-upload-entry-points"
title: "Files panel upload entry points"
status: done
wave: 5
depends_on:
  - "05-conflict-resolution-dialog"
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-001
acceptance_criteria:
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.1
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.2
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.3
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-001.7
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
---

# Task 06: Files panel upload entry points

## Summary

Make upload reachable: a create menu on the panel toolbar and an upload item on the folder
right-click menu, both wired to the flow from tasks 04 and 05. This is the change a person actually
sees.

## Scope

- `file-browser-toolbar.tsx`: the create control becomes a `DropdownMenu` with **New File**, **Upload
  Files**, and **Upload Folder**, using the sibling `WorkspaceActionsMenu` as the template for the
  trigger, tooltip, `min-h-11 ... sm:min-h-8` touch sizing, and `onCloseAutoFocus` handling.
- Two hidden inputs: one `multiple`, one carrying `webkitdirectory`. Destination is the active folder,
  or the workspace root when there is none.
- `file-context-menu.tsx`: **Upload files here** guarded as the exact inverse of the existing download
  guard, present for a folder and absent for a file or a multi-selection, threaded down the same prop
  chain as `onDownloadFile`.
- Both surfaces absent when there is no active task session.
- Confirmation names the written paths.
- The `task` namespace copy in all five locales.

## Exclusions

- Drag and drop from the operating system. Explicitly out of scope for this capability.
- The conflict dialog itself, which task 05 owns.
- Download work.

## Acceptance

- The create menu offers all three items; **New File** still begins inline creation with unchanged
  semantics, and the two upload items open the correct picker.
- **Upload files here** appears on a folder right-click, is absent for files and multi-selections, and
  targets that folder.
- Uploaded files appear in the tree at their destination without a manual refresh, and a folder
  upload recreates its directory structure.

## Verification

Write the menu-presence and guard assertions first and confirm they fail before the production
change. Then:

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/file-browser-toolbar.test.tsx components/task/file-context-menu.test.tsx
cd apps/web && node ../node_modules/typescript/bin/tsc --noEmit && pnpm run lint && pnpm run i18n:check
```

## Files likely touched

- `apps/web/components/task/file-browser-toolbar.tsx` and its test
- `apps/web/components/task/file-context-menu.tsx` and its test
- `apps/web/components/task/file-browser.tsx`
- `apps/web/components/task/files-panel.tsx`
- `apps/web/components/task/task-files-panel.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`

## Dependencies

Task 05.

## Parallelism

Sequential.

## Inputs

- Requirements: `REQ-UI-WORKSPACE-FILE-TRANSFER-001`.
- System design: `Components and responsibilities > Frontend`.
- Plan: `Frontend > Upload entry points` and `Mobile design contract`.
- Existing patterns: `WorkspaceActionsMenu` in the same toolbar file, and the download guard at
  `file-context-menu.tsx:114` for the inverse condition.

## Risks

- Inline file creation is currently one click on the `+` and becomes two through the menu. Keep
  `onStartCreate` semantics identical and cover the existing flow in the toolbar test, or this is a
  regression dressed as a feature.
- The hidden inputs must be reset between selections, or picking the same file twice in a row fires
  no change event.
- `webkitdirectory` is non-standard but universally supported in the browsers Kandev targets. Confirm
  the desktop Tauri webview honors it rather than assuming.
- Touch targets must come from the existing sizing idiom rather than being restyled per surface.

## Output contract

Report both entry points, the destination resolution rule, files changed, exact commands and results,
then mark this task `done` and update its checkbox in `plan.md`.

## Results

- `file-browser-toolbar.tsx` gains `CreateMenu`: **New file / Upload files / Upload folder**. When no
  upload handler is supplied it falls back to the original one-click `ToolbarButton`, so surfaces
  without upload keep today's behavior exactly.
- `file-context-menu.tsx` gains **Upload files here**, guarded as `node.is_dir && !isBulk`, the exact
  inverse of the download guard beside it.
- `components/task/use-file-upload-entry-points.tsx` owns the two hidden inputs (one `multiple`, one
  `webkitdirectory`), the conflict dialog, and the result toasts. It is mounted in `FileBrowser`,
  which both panels already render, so neither panel needed changing and both behave identically.
- The destination is captured when the picker opens, not when it resolves, because the active folder
  can change while the OS dialog is up. Inputs are reset before `.click()` so picking the same file
  twice in a row still fires a change event.
- Ten `task:` keys across five locales plus pseudo.

**Refactors forced by the code-quality limits**, both mechanical:
- `FileContextMenuItems` moved to a new `file-context-menu-items.tsx`; adding the upload item pushed
  `file-context-menu.tsx` to 605 lines and its complexity to 19.
- The delete-descriptor construction became `useDeleteAction`, since `FileContextMenu` hit 103 lines
  and complexity 16.

### Commands

```
pnpm --filter @kandev/web test -- components/task/file-context-menu.test.tsx components/task/file-browser-toolbar.test.tsx components/task/file-upload-conflict-dialog.test.tsx components/task/file-browser   82 passed / 14 files
node ../node_modules/typescript/bin/tsc --noEmit                                                                                                                                                              clean
pnpm run i18n:check                                                                                                                                                                                           6/6 gates
pnpm run lint                                                                                                                                                                                                 0 problems
```

A test asserts **New file still begins inline creation** through the menu, which is the regression
the risk section called out.
