---
status: shipped
created: 2026-07-29
owner: kandev
---

# Review Markdown Preview

## Why

Reviewers currently have to leave the expanded Review dialog or open a Markdown file as source
before they can inspect its rendered structure. This interrupts file-by-file review and makes
prose-heavy changes harder to validate.

## What

- A changed `.md` file in the expanded Review dialog exposes the existing `Preview markdown`
  action in its file header.
- Existing `.mdx` preview support remains available wherever the same Markdown action is used.
- Activating the action opens that exact file in Kandev's rendered Markdown preview, preserving
  its repository identity in multi-repository tasks.
- Desktop exposes the action as the existing eye-icon toolbar control and opens the existing file
  editor panel directly in preview mode.
- Mobile exposes the action in the existing 44 px file-actions menu and opens the existing
  full-height file viewer directly in preview mode.
- Non-Markdown files do not expose the action.
- The Review dialog's diff, review status, comments, filtering, and file selection remain
  unchanged.

## Scenarios

- **GIVEN** a changed `.md` file in the desktop Review dialog, **WHEN** the reviewer activates
  `Preview markdown`, **THEN** the exact file opens with its rendered Markdown visible immediately.
- **GIVEN** a changed `.md` file in the mobile Review dialog, **WHEN** the reviewer chooses
  `Preview markdown` from the file-actions menu, **THEN** the full-height file viewer opens with
  rendered Markdown visible immediately.
- **GIVEN** two repositories containing the same Markdown path, **WHEN** the reviewer previews one
  file, **THEN** the preview loads the file from the selected review row's repository.
- **GIVEN** a changed non-Markdown file, **WHEN** its Review header renders, **THEN** no Markdown
  preview action is present.

## Out of scope

- A new Markdown renderer or changes to Markdown sanitization.
- Rendering Markdown inline inside the diff body.
- Changing the Review dialog layout, file ordering, or review-state persistence.
- Adding preview support for other document formats.
