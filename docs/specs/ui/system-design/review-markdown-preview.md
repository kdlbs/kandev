---
status: current
system: ui
requirements:
  - REQ-UI-REVIEW-MARKDOWN-PREVIEW-001
---

# Review Markdown preview design

## Context and boundaries

The expanded Review dialog renders the loaded unified diff for each changed
file. Markdown preview uses only that diff and the existing sanitized Markdown
renderer. It does not read the workspace file or change review persistence.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-UI-REVIEW-MARKDOWN-PREVIEW-001 | Diff extraction and preview state |

## Diff extraction

`review-markdown-diff-preview.ts` extracts new-side Markdown from each loaded
diff. A complete added or untracked file renders as one document. Modified
files render each hunk separately, and incomplete content carries a partial
label. The Review file header offers the action only when extraction produces
renderable content.

## Preview state

`ReviewDialogDiffContent` owns the set of previewed file identities while the
dialog is open. It passes the set and toggle callback through `ReviewDiffList`
to each file row. The identity includes the repository scope and file path, so
files with the same path in different repositories do not share preview state.
The state survives a temporary empty diff list because the dialog content
remains mounted. Closing the dialog or changing the review source remounts the
owner and clears the set. A row only renders preview while its current diff
still has renderable Markdown; otherwise it shows the diff.

Desktop exposes the toggle in the file header. Mobile uses the existing file
actions menu. Both replace only the row body and keep Review open, with its
review state, comments, ordering, and scroll ownership intact.

## Verification

Component coverage removes and restores a file row after selecting preview.
Desktop and mobile Playwright scenarios exercise the rendered preview inside
the open Review dialog.
