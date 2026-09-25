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
- **AC-UI-COMMENT-MARKDOWN-001.5:** Markdown links to files in the active task workspace open the in-app file viewer; absolute workspace, worktree, and workspace-root-relative paths may include supported source-location suffixes without navigating the browser away from the task.
- **AC-UI-COMMENT-MARKDOWN-001.6:** Patterns matching `[A-Z]+-\d+` (e.g., `KAN-42`, `QA-7`) are rendered as links to `/office/tasks/<identifier>`. The link uses the raw identifier as the URL segment; resolution to an internal task ID (if needed) is handled by the issue page.
- **AC-UI-COMMENT-MARKDOWN-001.7:** The comment list in `TaskChat` renders the available comments in transcript order.
- **AC-UI-COMMENT-MARKDOWN-001.8:** Scroll position is preserved when new comments arrive if the user has scrolled away from the bottom.
- **AC-UI-COMMENT-MARKDOWN-001.9:** When a task message links to an absolute file path beneath a repository checkout registered to that task, but the active session uses a different worktree path for the same repository, selecting the link opens the corresponding file from the active task workspace on desktop and phone. Kandev does not read or edit the registered source checkout.
- **AC-UI-COMMENT-MARKDOWN-001.10:** When an absolute host file path cannot be mapped to the active task workspace or to a registered repository represented by that workspace, selecting it does not navigate the current Kandev tab or issue a workspace file request.

### REQ-UI-COMMENT-MARKDOWN-002: Prose section separators

**Intent:** Shared Markdown surfaces show prose followed by a section separator
as body text and a horizontal rule. UI owns this reusable presentation contract.
This is a bounded exception to the GFM behavior in requirement 001.

**Delivery:** Implemented as a render-time extension of the existing Markdown
normalizer. The [section separator plan](../../../plans/chat-markdown-separators/plan.md)
records the implementation and verification.

#### Eligibility

A separator candidate contains at least three consecutive hyphens, zero to three
leading spaces, and optional trailing spaces or tabs. Internal spaces are excluded.
The immediately preceding source line must be nonblank, ordinary paragraph text.
It must contain whitespace between words and satisfy either condition:

- At least 70 Unicode code points after trimming surrounding whitespace.
- At least 40 code points and a final sentence terminator (`.`, `!`, or `?`).
  Closing backticks, emphasis markers, parentheses, brackets, or quotes after
  the terminator do not prevent eligibility.

These limits describe source text, including inline Markdown. They do not depend
on viewport width or visual wrapping. A short final line does not become eligible
because earlier paragraph lines are long.

#### Acceptance criteria

- **AC-UI-COMMENT-MARKDOWN-002.1:** When an eligible paragraph precedes a candidate,
  the renderer shall show paragraph content followed by a semantic horizontal rule.
  It shall not promote that paragraph to a level-two heading.
- **AC-UI-COMMENT-MARKDOWN-002.2:** When the preceding line fails both length rules,
  the renderer shall retain GFM interpretation. `Skills to change` followed by
  `---` shall remain a level-two heading. Short punctuated titles also remain headings.
- **AC-UI-COMMENT-MARKDOWN-002.3:** The repair shall leave candidates inside backtick
  or tilde fences unchanged, including unclosed fences and longer enclosing fences.
  An ATX heading, list item, table row, blockquote, rule, fence delimiter, or indented
  code line immediately before a candidate shall not trigger repair.
- **AC-UI-COMMENT-MARKDOWN-002.4:** The repair shall preserve already separated rules,
  `===`, `***`, `___`, spaced hyphens, and leading YAML-style front matter boundaries.
  It shall preserve existing rule-delimited bare-code fixtures byte for byte.
- **AC-UI-COMMENT-MARKDOWN-002.5:** The repair shall change only the render input.
  Stored message content shall remain unchanged. Repeated normalization shall produce
  the same output, with original line endings and surrounding blank lines preserved.
- **AC-UI-COMMENT-MARKDOWN-002.6:** Desktop and phone chat shall show the same
  paragraph, heading, and separator semantics before and after message reload.
  Existing heading sizes, transcript navigation, and scroll ownership shall remain unchanged.

#### Compatibility and exclusions

The threshold deliberately favors section separators for long prose. An intentional
setext heading that meets eligibility also becomes a separator. Authors can use an
ATX heading to express an unambiguous long title. Short prose can remain setext.
General author-intent inference and repairs inside nested Markdown containers are
excluded. This contract does not authorize bare-wrapper repair or raw HTML support.

## Migrated source detail (requirement 001)

## Why

Task comments render as plain text with `whitespace-pre-wrap`. Agent responses routinely include markdown — code blocks, headers, bold text, lists — which renders as raw punctuation instead of formatted output.

## What

- Comments render as rich GitHub-flavored markdown: headers, bold, italic, inline code, fenced code blocks, blockquotes, ordered and unordered lists, and tables.
- `react-markdown` with `remark-gfm` processes comment content. `rehype-sanitize` blocks raw HTML injection (already in dependencies). No `dangerouslySetInnerHTML`.
- Fenced code blocks display syntax highlighting and a one-click copy button.
- Bare URLs in comment text are auto-linked (handled by `remark-gfm` linkify).
- Markdown links to files in the active task workspace open the in-app file viewer; absolute workspace, worktree, and workspace-root-relative paths may include supported source-location suffixes without navigating the browser away from the task.
- Absolute paths beneath a task-linked repository's registered source checkout map by repository identity to the corresponding active worktree. The registered checkout itself is never read or edited.
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
- **GIVEN** a task uses a worktree for a repository registered at `/home/user/projects/example`, **WHEN** a message links to `/home/user/projects/example/ui/bundle.js:61`, **THEN** Kandev opens the active worktree's `ui/bundle.js` in the desktop file tab or phone file viewer without navigating to a same-origin `/home/user/projects/...` URL.
- **GIVEN** a message links to an absolute host path that is neither in the active task workspace nor under a registered repository represented by that workspace, **WHEN** the user selects it, **THEN** Kandev leaves the task visible and does not request that path from the workspace file service.
- **GIVEN** the user has scrolled up to read earlier comments and a new comment arrives, **WHEN** the comment is appended, **THEN** the scroll position does not move.
- **GIVEN** the user is at the bottom of the thread and a new comment arrives, **WHEN** the comment is appended, **THEN** the viewport auto-scrolls to show the new comment.

## Out of scope

- Editing or previewing markdown in the comment input box (input remains plain text).
- Server-side markdown storage or transformation — content is stored as raw text and rendered client-side only.
- Reading or editing a registered repository's source checkout; source-path links always resolve to the active task workspace copy.
- Resolving task identifier links to internal UUIDs before navigation (the issue page handles that).
- Emoji shortcode rendering (`:+1:` etc.) — `remark-gemoji` is present in dependencies but not required by this spec.
- Notifications or unread indicators for new comments.

## Open questions

- Should the task-identifier regex be configurable (project key prefixes), or is a general `[A-Z]+-\d+` pattern acceptable for all workspaces?
- Which syntax highlighting library to use — `rehype-highlight` (highlight.js) or `rehype-pretty-code` (shiki)? Shiki gives better theming but adds bundle weight.
