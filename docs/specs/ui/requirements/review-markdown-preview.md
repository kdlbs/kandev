---
status: active
system: ui
created: 2026-07-29
owners:
  - kandev
---
# Review Markdown Preview Requirements

## Overview

Reviewers currently have to leave the expanded Review dialog or open a Markdown file as source before they can inspect its rendered structure. This interrupts file-by-file review and makes prose-heavy changes harder to validate.

## Requirements

### REQ-UI-REVIEW-MARKDOWN-PREVIEW-001: Review Markdown Preview

**Intent:** Reviewers currently have to leave the expanded Review dialog or open a Markdown file as source before they can inspect its rendered structure. This interrupts file-by-file review and makes prose-heavy changes harder to validate.

#### Acceptance criteria

- **AC-UI-REVIEW-MARKDOWN-PREVIEW-001.1:** A changed `.md` file in the expanded Review dialog exposes the existing `Preview markdown` action in its file header.
- **AC-UI-REVIEW-MARKDOWN-PREVIEW-001.2:** Existing `.mdx` preview support remains available wherever the same Markdown action is used.
- **AC-UI-REVIEW-MARKDOWN-PREVIEW-001.3:** Activating the action keeps the Review dialog open and replaces that file's textual diff with a rendered changed-content preview. Activating `Show diff` restores the textual diff in place.
- **AC-UI-REVIEW-MARKDOWN-PREVIEW-001.4:** The preview is derived only from the unified diff already loaded by Review. It does not fetch the workspace file, open a file-editor tab, or navigate away from Review.
- **AC-UI-REVIEW-MARKDOWN-PREVIEW-001.5:** For a complete added or untracked file diff, the preview renders the new-side lines as one Markdown document.
- **AC-UI-REVIEW-MARKDOWN-PREVIEW-001.6:** For a modified file, each hunk renders as a separate Markdown fragment containing its new-side context and additions. Omitted lines between hunks are not joined or implied to be adjacent.
- **AC-UI-REVIEW-MARKDOWN-PREVIEW-001.7:** Deleted lines, diff metadata, hunk headers, and `No newline at end of file` markers are never included in rendered Markdown.
- **AC-UI-REVIEW-MARKDOWN-PREVIEW-001.8:** A partial or truncated diff labels its preview as partial. Files with no renderable new-side Markdown do not expose the preview action.

## Migrated source detail

## Why

Reviewers currently have to leave the expanded Review dialog or open a Markdown file as source
before they can inspect its rendered structure. This interrupts file-by-file review and makes
prose-heavy changes harder to validate.

## What

- A changed `.md` file in the expanded Review dialog exposes the existing `Preview markdown`
  action in its file header.
- Existing `.mdx` preview support remains available wherever the same Markdown action is used.
- Activating the action keeps the Review dialog open and replaces that file's textual diff with a
  rendered changed-content preview. Activating `Show diff` restores the textual diff in place.
- The preview is derived only from the unified diff already loaded by Review. It does not fetch the
  workspace file, open a file-editor tab, or navigate away from Review.
- For a complete added or untracked file diff, the preview renders the new-side lines as one
  Markdown document.
- For a modified file, each hunk renders as a separate Markdown fragment containing its new-side
  context and additions. Omitted lines between hunks are not joined or implied to be adjacent.
- Deleted lines, diff metadata, hunk headers, and `No newline at end of file` markers are never
  included in rendered Markdown.
- A partial or truncated diff labels its preview as partial. Files with no renderable new-side
  Markdown do not expose the preview action.
- Desktop exposes the action as the existing eye-icon toolbar control.
- Mobile exposes the same in-place behavior through the existing 44 px file-actions menu.
- Non-Markdown files do not expose the action.
- Preview mode is transient. Review status, comments, filtering, file ordering, and file selection
  remain unchanged.

## Scenarios

- **GIVEN** a changed `.md` file in the desktop Review dialog, **WHEN** the reviewer activates
  `Preview markdown`, **THEN** the dialog stays open and that file's diff is replaced by rendered
  changed content without opening a file tab.
- **GIVEN** a changed `.md` file in the mobile Review dialog, **WHEN** the reviewer chooses
  `Preview markdown` from the file-actions menu, **THEN** the dialog remains the focused surface and
  shows the rendered changed content in place.
- **GIVEN** a complete added Markdown file diff, **WHEN** the reviewer previews it, **THEN** all
  new-side lines render together as one document.
- **GIVEN** a modified Markdown file with multiple hunks, **WHEN** the reviewer previews it,
  **THEN** each hunk renders separately and omitted unchanged lines are not synthesized.
- **GIVEN** a truncated Markdown diff with renderable new-side lines, **WHEN** the reviewer previews
  it, **THEN** the rendered fragments remain available and the UI identifies them as partial.
- **GIVEN** a Markdown diff with no renderable new-side lines, **WHEN** its Review header renders,
  **THEN** no Markdown preview action is present.
- **GIVEN** a Markdown changed-content preview, **WHEN** the reviewer activates `Show diff`,
  **THEN** the original textual diff returns in the same Review row.
- **GIVEN** a changed non-Markdown file, **WHEN** its Review header renders, **THEN** no Markdown
  preview action is present.

## Out of scope

- A new Markdown renderer or changes to Markdown sanitization.
- Fetching the full workspace file or a remote pull-request revision for preview.
- Reconstructing omitted unchanged lines for modified files.
- Changing the Review dialog layout, file ordering, or review-state persistence.
- Persisting changed-content preview mode after the dialog closes.
- Adding preview support for other document formats.
