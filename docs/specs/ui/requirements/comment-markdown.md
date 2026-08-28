---
status: active
system: ui
created: 2026-04-28
owners:
  - cfl
---
# Comment Markdown Rendering Requirements

## Overview

Task comments render as plain text with `whitespace-pre-wrap`. Agent responses routinely include markdown — code blocks, headers, bold text, lists — which renders as raw punctuation instead of formatted output.

## Requirements

### REQ-UI-COMMENT-MARKDOWN-001: Comment Markdown Rendering

**Intent:** Task comments render as plain text with `whitespace-pre-wrap`. Agent responses routinely include markdown — code blocks, headers, bold text, lists — which renders as raw punctuation instead of formatted output.

#### Acceptance criteria

- **AC-UI-COMMENT-MARKDOWN-001.1:** Comments render as rich GitHub-flavored markdown: headers, bold, italic, inline code, fenced code blocks, blockquotes, ordered and unordered lists, and tables.
- **AC-UI-COMMENT-MARKDOWN-001.2:** `react-markdown` with `remark-gfm` processes comment content. `rehype-sanitize` blocks raw HTML injection (already in dependencies). No `dangerouslySetInnerHTML`.
- **AC-UI-COMMENT-MARKDOWN-001.3:** Fenced code blocks display syntax highlighting and a one-click copy button.
- **AC-UI-COMMENT-MARKDOWN-001.4:** Bare URLs in comment text are auto-linked (handled by `remark-gfm` linkify).
- **AC-UI-COMMENT-MARKDOWN-001.5:** Markdown links to files in the active worktree open the in-app file editor; absolute worktree paths and repo-root paths may include `:line` or `:line:column` suffixes without navigating the browser away from the task.
- **AC-UI-COMMENT-MARKDOWN-001.6:** Patterns matching `[A-Z]+-\d+` (e.g., `KAN-42`, `QA-7`) are rendered as links to `/office/tasks/<identifier>`. The link uses the raw identifier as the URL segment; resolution to an internal task ID (if needed) is handled by the issue page.
- **AC-UI-COMMENT-MARKDOWN-001.7:** The comment list in `TaskChat` renders the available comments in transcript order.
- **AC-UI-COMMENT-MARKDOWN-001.8:** Scroll position is preserved when new comments arrive if the user has scrolled away from the bottom.

## Migrated source detail

## Why

Task comments render as plain text with `whitespace-pre-wrap`. Agent responses routinely include markdown — code blocks, headers, bold text, lists — which renders as raw punctuation instead of formatted output.

## What

- Comments render as rich GitHub-flavored markdown: headers, bold, italic, inline code, fenced code blocks, blockquotes, ordered and unordered lists, and tables.
- `react-markdown` with `remark-gfm` processes comment content. `rehype-sanitize` blocks raw HTML injection (already in dependencies). No `dangerouslySetInnerHTML`.
- Fenced code blocks display syntax highlighting and a one-click copy button.
- Bare URLs in comment text are auto-linked (handled by `remark-gfm` linkify).
- Markdown links to files in the active worktree open the in-app file editor; absolute worktree paths and repo-root paths may include `:line` or `:line:column` suffixes without navigating the browser away from the task.
- Patterns matching `[A-Z]+-\d+` (e.g., `KAN-42`, `QA-7`) are rendered as links to `/office/tasks/<identifier>`. The link uses the raw identifier as the URL segment; resolution to an internal task ID (if needed) is handled by the issue page.
- The comment list in `TaskChat` renders the available comments in transcript order.
- Scroll position is preserved when new comments arrive if the user has scrolled away from the bottom.
- When the user is at (or near) the bottom, new comments auto-scroll into view.

## Scenarios

- **GIVEN** a comment with `**bold** and \`code\``, **WHEN** rendered, **THEN** bold text and inline code appear styled, not as raw characters.
- **GIVEN** a comment containing a fenced code block, **WHEN** rendered, **THEN** the block has syntax highlighting and a copy button that copies the block contents.
- **GIVEN** a comment containing the text `see KAN-42 for context`, **WHEN** rendered, **THEN** `KAN-42` is a clickable link navigating to `/office/tasks/KAN-42`.
- **GIVEN** a comment containing `https://example.com`, **WHEN** rendered, **THEN** the URL is a clickable hyperlink.
- **GIVEN** a comment contains a markdown link to `/root/.kandev/tasks/example/kandev/.github/workflows/build.yml:12`, **WHEN** the user clicks it while the active worktree is `/root/.kandev/tasks/example/kandev`, **THEN** the in-app editor opens `.github/workflows/build.yml` instead of navigating to that absolute URL.
- **GIVEN** the user has scrolled up to read earlier comments and a new comment arrives, **WHEN** the comment is appended, **THEN** the scroll position does not move.
- **GIVEN** the user is at the bottom of the thread and a new comment arrives, **WHEN** the comment is appended, **THEN** the viewport auto-scrolls to show the new comment.

## Out of scope

- Editing or previewing markdown in the comment input box (input remains plain text).
- Server-side markdown storage or transformation — content is stored as raw text and rendered client-side only.
- Resolving task identifier links to internal UUIDs before navigation (the issue page handles that).
- Emoji shortcode rendering (`:+1:` etc.) — `remark-gemoji` is present in dependencies but not required by this spec.
- Notifications or unread indicators for new comments.

## Open questions

- Should the task-identifier regex be configurable (project key prefixes), or is a general `[A-Z]+-\d+` pattern acceptable for all workspaces?
- Which syntax highlighting library to use — `rehype-highlight` (highlight.js) or `rehype-pretty-code` (shiki)? Shiki gives better theming but adds bundle weight.
