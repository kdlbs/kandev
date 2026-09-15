---
id: "02-editor-download-actions"
title: "Download actions on editor and viewer surfaces"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-WORKSPACE-FILE-TRANSFER-002
acceptance_criteria:
  - AC-UI-WORKSPACE-FILE-TRANSFER-002.1
  - AC-UI-WORKSPACE-FILE-TRANSFER-002.2
  - AC-UI-WORKSPACE-FILE-TRANSFER-002.3
  - AC-UI-WORKSPACE-FILE-TRANSFER-002.4
  - AC-UI-WORKSPACE-FILE-TRANSFER-002.5
  - AC-UI-WORKSPACE-FILE-TRANSFER-002.6
system_design:
  - ../../specs/ui/system-design/workspace-file-transfer.md
---

# Task 02: Download actions on editor and viewer surfaces

## Summary

Put download where people are actually looking at the file: both editor headers, the
unpreviewable-file screen, and the image viewer. Entirely frontend, no transport work, because the
viewer already holds the content.

## Scope

- A download control in `monaco-editor-toolbar.tsx`, shaped like its `DeleteButton` neighbour and
  placed between the open-with dropdown and delete, driven by a new optional `onDownload` prop.
- The same control in the parallel CodeMirror toolbar in `codemirror-code-editor.tsx`.
- A download control appended to the `headerActions` fragment in `file-editor-panel.tsx`, which
  `FileBinaryViewer`, `FileImageViewer`, and the mobile file viewer all consume.
- Download from the content already in the dockview store, through the existing
  `triggerFileDownload`.
- The `editors:downloadFile` key in all five locales.

## Exclusions

- Any upload work.
- A streaming download endpoint, folder download, or multi-select download.
- The separate defect where a file above the read cap hangs on the loading state.
- Changing the existing tree right-click download.

## Acceptance

- Both editors show a download control with the same placement, icon, and accessible name, and each
  invokes its handler for the open file.
- The unpreviewable-file screen and the image viewer both render the control from the shared
  `headerActions` contract, and the mobile viewer inherits it.
- A downloaded binary file keeps its original bytes and original file name.
- The existing single-file download in the tree's right-click menu keeps its current behavior and
  placement, proven by a regression assertion rather than by inspection.

## Verification

Write the toolbar and viewer assertions first and confirm they fail before the production change.
Then:

```bash
cd apps && pnpm --filter @kandev/web test -- components/editors/monaco/monaco-editor-toolbar.test.tsx components/task/file-editor-panel.download.test.tsx
cd apps/web && pnpm run typecheck && pnpm run i18n:check
```

## Files likely touched

- `apps/web/components/editors/monaco/monaco-editor-toolbar.tsx` and its test
- `apps/web/components/editors/codemirror/codemirror-code-editor.tsx` and a new toolbar test
- `apps/web/components/task/file-editor-panel.tsx`
- `apps/web/components/task/file-editor-panel.download.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/editors.json`

## Dependencies

None.

## Parallelism

Parallel-safe with task 01. Land this first if a standalone quick win is wanted; it is independently
shippable.

## Inputs

- Requirements: `REQ-UI-WORKSPACE-FILE-TRANSFER-002`.
- System design: `Components and responsibilities > Frontend`, `Control flow` on why no backend is
  needed.
- Existing patterns: `DeleteButton` and `CodeMirrorDeleteButton` for the control shape,
  `FileViewerHeader` for the action row, `triggerFileDownload` for the transfer.

## Risks

- Doing only Monaco leaves CodeMirror inconsistent, which is the bug this task exists to remove.
- Refetching instead of using the loaded content adds a failure mode the requirement excludes.
- `triggerFileDownload` expects base64 for binary content, matching the `workspace.file.get`
  contract. Passing decoded bytes corrupts the download.
- Five locales are required; `check-i18n-keys.mjs` fails on a value left identical to English.

## Output contract

Report each surface that gained the control, files changed, exact commands and results, then mark
this task `done` and update its checkbox in `plan.md`.

## Results

- `DownloadButton` in `monaco-editor-toolbar.tsx` and `CodeMirrorDownloadButton` in
  `codemirror-code-editor.tsx`, both between open-with and delete, driven by a new optional
  `onDownload` threaded through `file-editor-content.tsx` and `monaco-code-editor.tsx`.
- `FileViewerDownloadButton` in `file-viewer-header.tsx`, appended to the `headerActions` fragment in
  `file-editor-panel.tsx`, which lights up the binary viewer, the image viewer, and the mobile viewer
  from one place.
- Download reads the buffer already in the dockview store via `useOpenFileDownload` / a `useMemo` in
  the editor path. No refetch, and binary content stays base64 for `triggerFileDownload`.
- `editors:downloadFile` added in en, pt-pt, zh-cn, zh-hk, zh-tw plus the regenerated pseudo catalog.

**Regression found and fixed:** the new tooltip broke `file-editor-panel.image.test.tsx`, which
rendered `StaticFilePanel` without a `TooltipProvider`. Production is fine (`app-shell.tsx` and
`app/layout.tsx` both provide one); the test had only escaped it by mocking the single component that
used a tooltip. Its renders are now wrapped.

### Commands

```text
pnpm --filter @kandev/web test -- components/editors/ components/task/file-editor-panel   37 passed
pnpm run i18n:check                                                                        6/6 gates pass
pnpm run lint                                                                              0 problems
node ../node_modules/typescript/bin/tsc --noEmit                                           clean
```
