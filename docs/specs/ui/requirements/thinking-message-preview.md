---
status: draft
system: ui
created: 2026-08-30
owners:
  - kandev
---

# Thinking Message Preview Requirements

## Overview

Thinking messages can contain several concise reasoning headings. Collapsed
rows currently hide those headings behind the generic Thinking label whenever
the source contains a newline. A compact preview helps users scan the agent's
progress without expanding every reasoning block.

The UI system owns this presentation contract. Agent runtimes and the task
system continue to own reasoning production, delivery, and persistence.

## Terminology

- **Thinking message:** An agent transcript message with the `thinking` type.
- **Meaningful line:** The first source line that contains visible text after
  surrounding whitespace and inline Markdown decoration are removed.
- **Preview:** The plain-text meaningful line shown beside the localized
  Thinking label in the collapsed row.

## Requirements

### REQ-UI-THINKING-MESSAGE-PREVIEW-001: Collapsed thinking message preview

**Intent:** Let users identify the subject of an expandable thinking message
without opening the complete reasoning block.

**User story:** As a user reading an agent transcript, I want collapsed
thinking rows to describe their subject, so that I can scan the agent's
progress quickly.

#### Acceptance criteria

- **AC-UI-THINKING-MESSAGE-PREVIEW-001.1:** When an expandable thinking
  message contains a meaningful line, Kandev shall show that line as plain
  text beside the localized Thinking label while the message is collapsed.
- **AC-UI-THINKING-MESSAGE-PREVIEW-001.2:** When a thinking message starts
  with blank lines or Markdown decoration that produces no visible text,
  Kandev shall skip those lines. If no meaningful line exists, the row shall
  show only the localized Thinking label.
- **AC-UI-THINKING-MESSAGE-PREVIEW-001.3:** When later reasoning content is
  appended to a thinking message after its first meaningful line, Kandev shall
  keep the original preview instead of replacing it with a later line.
- **AC-UI-THINKING-MESSAGE-PREVIEW-001.4:** On desktop and mobile, an
  expandable thinking preview shall occupy one visual line, truncate within
  the available row width, and shall not widen the transcript or document.
- **AC-UI-THINKING-MESSAGE-PREVIEW-001.5:** Expanding a thinking row shall
  continue to show the complete original reasoning content with its existing
  Markdown rendering. The preview shall not modify persisted or expanded
  content.
- **AC-UI-THINKING-MESSAGE-PREVIEW-001.6:** Compact single-line thinking
  messages that already render their complete text inline shall keep their
  current inline and non-expandable behavior.
- **AC-UI-THINKING-MESSAGE-PREVIEW-001.7:** Preview derivation shall apply to
  thinking messages from every agent provider without provider-specific
  labels or parsing rules.

## Out of scope

- Generating a new summary or choosing a later line because it appears more
  descriptive.
- Changing ACP reasoning events, task-message persistence, WebSocket payloads,
  or streaming coalescing.
- Changing the expanded Markdown renderer or the compact single-line
  threshold.
- Redesigning the shared expandable-row interaction or its accessibility
  contract.
