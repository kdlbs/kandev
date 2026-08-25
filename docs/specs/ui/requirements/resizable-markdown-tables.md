---
status: active
system: ui
created: 2026-08-24
owners:
  - kandev
---

# Resizable Markdown Tables Requirements

## Overview

Kandev renders Markdown tables in chat transcripts, file previews, and the
editable Plan view. Automatic wrapping and table-local scrolling remain the
readability baseline. Fine-pointer users can temporarily rebalance inconvenient
column proportions without editing the Markdown source.

## Terminology

- **Rendered Markdown table:** A table produced by Kandev's shared
  GitHub-flavored Markdown renderer, including chat and rendered Markdown file
  previews.
- **Plan table:** A Markdown table rendered inside the editable Plan view by the
  rich-text editor.
- **Eligible layout:** A non-phone viewport with a fine pointer and measurable
  table geometry.
- **Resize boundary:** An internal edge between two columns. A table's outer
  right edge is not an internal boundary.

## Requirements

### REQ-UI-MARKDOWN-TABLE-RESIZE-001: Surface-consistent column resizing

**Intent:** Let users adjust Markdown table columns wherever they perform
task-focused reading or planning, while preserving source text, accessibility,
and responsive readability.

**User story:** As a user reading or editing a Markdown table, I want to drag an
internal column boundary, so that I can rebalance the visible columns without
rewriting the table.

#### Acceptance criteria

- **AC-UI-MARKDOWN-TABLE-RESIZE-001.1:** When a rendered Markdown table appears
  in chat or a rendered Markdown file preview on an eligible layout, the UI
  shall expose a resize separator at each valid internal column boundary.
- **AC-UI-MARKDOWN-TABLE-RESIZE-001.2:** When a user drags a rendered Markdown
  separator, the UI shall resize only its two adjacent columns, preserve their
  combined width and the table's total width, leave non-adjacent columns
  unchanged, and prevent either adjacent column from becoming narrower than 64
  CSS pixels.
- **AC-UI-MARKDOWN-TABLE-RESIZE-001.3:** When a user focuses a rendered Markdown
  separator, the UI shall expose its adjacent one-based column numbers,
  orientation, current width, minimum, and maximum through separator semantics;
  ArrowLeft and ArrowRight shall adjust by 8 CSS pixels, Enter shall reset the
  table, and double-click shall also reset it.
- **AC-UI-MARKDOWN-TABLE-RESIZE-001.4:** When a user drags an internal boundary
  in a Plan table on an eligible layout, the Plan view shall resize that column
  subject to the same 64 CSS-pixel minimum, keep the table readable within its
  local container, and leave the plan's Markdown text unchanged.
- **AC-UI-MARKDOWN-TABLE-RESIZE-001.5:** When a rendered Markdown file preview
  enables source-range comments, resizing shall preserve the table's source-line
  identity, comment selection and badge behavior, links, and other interactive
  content outside the resize hit area.
- **AC-UI-MARKDOWN-TABLE-RESIZE-001.6:** When any resized table unmounts, reloads,
  or is rendered at another location, no column-width preference shall survive
  in Markdown, task-plan data, browser storage, or backend state.
- **AC-UI-MARKDOWN-TABLE-RESIZE-001.7:** When the same table appears on a phone or
  coarse-pointer layout, the UI shall expose no resize affordance and shall keep
  content accessible through automatic wrapping or one table-local horizontal
  scroll region without document-level horizontal overflow.
- **AC-UI-MARKDOWN-TABLE-RESIZE-001.8:** When resize capability disappears during
  an active interaction, the UI shall end the interaction, clear resize cursor
  and text-selection overrides, preserve Markdown text, and return the table to
  a readable responsive layout.

## Out of scope

- Persisting column widths across reloads, sessions, render locations, or plan
  revisions.
- Encoding widths in Markdown, task-plan content, or backend records.
- Touch dragging or a phone column-width editor.
- Resizing read-only plan revision previews or plan comparison views.
- Resizing rows, reordering or hiding columns, or changing non-Markdown data
  grids.
- Providing a numeric width input.
