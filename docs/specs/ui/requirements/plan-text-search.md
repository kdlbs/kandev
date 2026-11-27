---
status: active
system: ui
created: 2026-10-13
owners:
  - kandev
---

# Plan Text Search Requirements

## Overview

Plan find locates the text a reader sees, including phrases containing inline
formatting. UI owns this reusable rich-text find, highlight, and navigation
contract independently of task-plan storage. Tasks continues to own content,
history, and persistence. Editor replacement belongs to the adjacent
[task-switch stability contract](plan-editor-task-switch-stability.md).

## Terminology

- **Searchable run:** Contiguous text within one textblock, without a non-text
  inline item between its characters. Inline formatting does not divide a run.
- **Textblock:** A paragraph, heading, code block, or equivalent text-bearing
  block, including one inside a list, quotation, or table cell.
- **Logical match:** One non-overlapping occurrence of the query in a searchable
  run, irrespective of the number of formatting or highlight fragments.

## Requirements

### REQ-UI-PLAN-TEXT-SEARCH-001: Formatting-independent Plan find

**Intent:** Find visible Plan text without requiring the reader to remove its
formatting.

**User story:** As a user reading a Plan, I want phrase search to ignore inline
formatting boundaries, so that formatting cannot conceal text from find.

#### Acceptance criteria

- **AC-UI-PLAN-TEXT-SEARCH-001.1:** For a nonempty query, Plan find shall return
  all non-overlapping literal occurrences in each searchable run in document
  order, ignoring letter case with the existing search semantics. Bold, italic,
  inline code, link, underline, highlight, and comment formatting, including a
  change midway through a word, shall not divide or duplicate an occurrence.
  Query punctuation shall remain literal; spaces and line characters present
  in the text shall not be trimmed, collapsed, or substituted.
- **AC-UI-PLAN-TEXT-SEARCH-001.2:** A match shall not join separate textblocks
  or cross a non-text inline item, including a hard line break. Search shall
  still find occurrences wholly on either side of those boundaries. Text in
  list items, headings, quotations, code blocks, and table cells shall remain
  searchable within its own runs; empty blocks shall not join neighbors.
- **AC-UI-PLAN-TEXT-SEARCH-001.3:** Each match shall highlight exactly its text,
  including when supplementary Unicode characters precede or occur in the
  query. Formatting shall remain intact. The counter shall count logical
  matches, and the current logical match shall be distinguished across all
  of its formatted fragments.
- **AC-UI-PLAN-TEXT-SEARCH-001.4:** Setting a query shall select its first match.
  Next and Previous shall traverse matches in document order, wrapping at both
  ends and retaining existing scrolling behavior. An empty or absent query
  result shall show zero matches and navigation shall do nothing. Clearing
  search shall remove its highlights and match selection.
- **AC-UI-PLAN-TEXT-SEARCH-001.5:** While a query is active, editing Plan text
  or formatting shall recompute matches, highlights, and the counter against
  the edited document. The selected match index shall remain within the new
  result list, or return to zero when there are no matches. Inserting or
  deleting text before a hit shall not leave highlights at former positions.
- **AC-UI-PLAN-TEXT-SEARCH-001.6:** For the same Plan content and query,
  desktop and phone shall produce the same logical matches and selection
  behavior through their existing Plan surface.

## Out of scope

- Plan persistence, autosave, initial loading, revisions, task switching, or
  asynchronous result ownership.
- Layout, responsive composition, controls, copy, or shortcuts.
- Regex search, case toggles, fuzzy matching, whitespace normalization, locale
  collation, or matching across structural boundaries.
- Chat, terminal, workspace, file, or command-palette search.

## Implementation Plans

- [Plan search across formatting](../../../plans/plan-search-across-formatting/plan.md)
