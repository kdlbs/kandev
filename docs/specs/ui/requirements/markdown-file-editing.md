---
status: draft
system: ui
created: 2026-08-27
owners:
  - kandev
---

# Markdown File Editing Requirements

## Overview

Kandev users read and change repository Markdown while they work with agents.
The current file surface requires a full switch between Monaco source and a
rendered preview. Kandev needs a source-preserving editing mode that keeps
rendered context visible without weakening the existing preview or save
contracts.

The UI system owns this reusable file interaction. Repository access, file
persistence, dirty-state reconciliation, and agent updates remain owned by the
existing workspace and task contracts.

## Terminology

- **Preview mode:** A read-only, sanitized rendering of the current Markdown
  buffer.
- **Edit mode:** A hybrid view that renders inactive Markdown and exposes
  source syntax around the active editing range.
- **Source mode:** The existing plain-text code editor for exact source access.
- **Canonical source:** The plain Markdown string that Kandev loads, edits,
  marks dirty, and saves.
- **Table delimiter row:** The required Markdown source row, such as
  `| --- | --- |`, that separates a table header from its body.
- **Unsupported construct:** Markdown or embedded syntax that Edit mode cannot
  render without changing its source.

## Requirements

### REQ-UI-MARKDOWN-FILE-EDITING-001: Source-preserving Markdown modes

**Intent:** Let users read and edit Markdown in context without losing access
to the exact file source.

**User story:** As a Kandev user, I want rendered Markdown around the text that
I edit, so that I can understand formatting without switching views after each
change.

#### Acceptance criteria

- **AC-UI-MARKDOWN-FILE-EDITING-001.1:** When a user opens a new `.md` file,
  the system shall start in Preview mode and show an accessible Edit control.
- **AC-UI-MARKDOWN-FILE-EDITING-001.2:** When the user enters Edit mode, the
  system shall render inactive Markdown blocks and reveal source syntax for the
  active editing range.
- **AC-UI-MARKDOWN-FILE-EDITING-001.3:** When the user edits content in Edit
  mode, the system shall update the canonical source without rewriting
  unrelated syntax or formatting.
- **AC-UI-MARKDOWN-FILE-EDITING-001.4:** When the user enters Source mode, the
  system shall provide the existing plain-text editor with the current unsaved
  content.
- **AC-UI-MARKDOWN-FILE-EDITING-001.5:** When the user switches modes, the
  system shall preserve current bytes, dirty state, selection intent, and a
  stable reading position where the target mode supports them.
- **AC-UI-MARKDOWN-FILE-EDITING-001.6:** When Edit mode cannot render a
  construct, the system shall expose that construct as source and shall not
  remove or normalize it.
- **AC-UI-MARKDOWN-FILE-EDITING-001.7:** When the user opens an `.mdx` file,
  the system shall offer Preview and Source modes without a hybrid Edit mode.
- **AC-UI-MARKDOWN-FILE-EDITING-001.8:** When Edit mode reveals an active
  block, the system shall keep block-separating line endings in canonical
  source without showing return or newline glyphs after the block.
- **AC-UI-MARKDOWN-FILE-EDITING-001.9:** Edit mode shall apply theme-aware
  syntax highlighting to supported fenced code blocks while keeping code
  fences and code content in canonical Markdown source.

### REQ-UI-MARKDOWN-FILE-EDITING-002: Safe file workflow integration

**Intent:** Make Markdown editing follow the same save, update, review, and
comment behavior as other repository files.

**User story:** As a Kandev user, I want Markdown editing to respect the file
workflow, so that switching presentation does not risk my work.

#### Acceptance criteria

- **AC-UI-MARKDOWN-FILE-EDITING-002.1:** Preview mode shall use Kandev's
  sanitized Markdown renderer and shall not execute embedded scripts or unsafe
  URLs.
- **AC-UI-MARKDOWN-FILE-EDITING-002.2:** Preview mode shall preserve existing
  task links, external file links, comments, Mermaid diagrams, and resizable
  table behavior.
- **AC-UI-MARKDOWN-FILE-EDITING-002.3:** Edit and Source modes shall use the
  existing dirty, save, discard, delete, and keyboard-save contracts.
- **AC-UI-MARKDOWN-FILE-EDITING-002.4:** When a clean file changes outside the
  editor, the system shall refresh the canonical source and preserve the
  nearest valid selection.
- **AC-UI-MARKDOWN-FILE-EDITING-002.5:** When a dirty file changes outside the
  editor, the system shall preserve local content and expose the existing
  reload decision.
- **AC-UI-MARKDOWN-FILE-EDITING-002.6:** When comments are visible, Preview and
  Edit modes shall map selections and comment ranges to the same source lines.
- **AC-UI-MARKDOWN-FILE-EDITING-002.7:** When a comparison baseline exists,
  Edit mode shall show added and changed regions without making the rendered
  view the source of truth. The normal saved file buffer shall not become a
  comparison baseline or duplicate edited blocks.
- **AC-UI-MARKDOWN-FILE-EDITING-002.8:** When Edit mode fails to initialize or
  update, the system shall keep the current content and offer Source mode.
- **AC-UI-MARKDOWN-FILE-EDITING-002.9:** Preview and Edit modes shall show
  visible table cell boundaries.
- **AC-UI-MARKDOWN-FILE-EDITING-002.10:** When a table is active in Edit mode,
  the system shall hide its table delimiter row while preserving the exact
  delimiter bytes in canonical source.
- **AC-UI-MARKDOWN-FILE-EDITING-002.11:** When a table is active in Edit mode,
  the system shall provide an insertion action after every visible row and
  column. Each action shall remain outside editable cells, apply one undoable
  canonical-source edit, and place the selection in the inserted row or
  column.
- **AC-UI-MARKDOWN-FILE-EDITING-002.12:** When a user resizes an Edit-mode
  table column by pointer, touch, or keyboard, the system shall update the
  visible column width without changing canonical source. The width shall
  survive mode switches while the file tab remains open.

### REQ-UI-MARKDOWN-FILE-EDITING-003: Responsive and accessible editing

**Intent:** Provide the same Markdown reading and editing value on desktop,
tablet, and phone layouts.

**User story:** As a mobile Kandev user, I want to edit and preview Markdown in
the Files surface, so that I can complete the same task without a desktop.

#### Acceptance criteria

- **AC-UI-MARKDOWN-FILE-EDITING-003.1:** Desktop and tablet layouts shall show
  one clear mode control for Preview, Edit, and Source.
- **AC-UI-MARKDOWN-FILE-EDITING-003.2:** Phone layouts shall provide Preview,
  Edit, save, and source fallback actions in a focused file surface.
- **AC-UI-MARKDOWN-FILE-EDITING-003.3:** Phone controls shall have 44-pixel
  touch targets and remain usable with the virtual keyboard and safe areas.
- **AC-UI-MARKDOWN-FILE-EDITING-003.4:** Each responsive layout shall have one
  vertical scroll owner. Wide Markdown tables shall scroll locally without
  widening the page.
- **AC-UI-MARKDOWN-FILE-EDITING-003.5:** The mode control shall expose text or
  accessible names, keyboard operation, selected state, and focus feedback.
- **AC-UI-MARKDOWN-FILE-EDITING-003.6:** Desktop and mobile automated tests
  shall prove editing, saving, reloading, previewing, and source fallback.
- **AC-UI-MARKDOWN-FILE-EDITING-003.7:** Table edge actions and resize handles
  shall use the surrounding file-surface density on fine pointers and at least
  44-pixel hit targets on coarse pointers without covering table content or
  adding another scroll owner. A coarse-pointer action shall not require hover.

## Out of scope

- Editing Markdown inside Review diffs. The Review system keeps its existing
  changed-content preview contract.
- Replacing Monaco as Kandev's general code editor.
- Converting arbitrary repository Markdown into a ProseMirror document.
- Providing hybrid editing for MDX, embedded JSX, or executable HTML.
- Persisting table column widths into repository Markdown or backend state.
- Creating tables, merging cells, or adding Confluence-specific header,
  numbered-column, chart, and macro semantics.
- Adding collaborative multi-cursor editing or a new backend file API.
