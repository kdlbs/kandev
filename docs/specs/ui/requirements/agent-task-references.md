---
status: active
system: ui
created: 2026-11-16
owners:
  - kandev
---

# Agent task reference requirements

## Overview

Agents need a readable way to reference an existing task in a conversation.
UI owns this transcript presentation and navigation contract. Tasks retains
identity, access, and lifecycle ownership.

This capability complements [user task mentions](chat-task-mentions.md).
Agent-authored links have a different source contract from composer-selected
mentions. Existing user-mention behavior remains authoritative in that document.

## Requirements

### REQ-UI-AGENT-TASK-REFERENCE-001: Readable agent task references

**Intent:** Identify and open the exact task an agent references without reading
an identifier or losing the surrounding sentence.

#### Acceptance criteria

- **AC-UI-AGENT-TASK-REFERENCE-001.1:** When an agent supplies a recognized task
  link in a normal chat reply or an agent-plan message, the transcript shall
  display an inline chip with a task icon and the authored title. The chip shall
  use the existing task-mention shape and the referenced task's assigned
  personal color in light and dark themes. With no assigned color, use
  Kandev's primary accent. Changing or clearing the personal color shall
  update visible references without reloading the conversation.
- **AC-UI-AGENT-TASK-REFERENCE-001.2:** Clicking, tapping, or pressing Enter on
  the chip shall open the referenced task directly. The action shall preserve
  existing navigation guards, modified-click behavior, and browser Back history.
  It shall not require a preview confirmation or reload the application.
- **AC-UI-AGENT-TASK-REFERENCE-001.3:** Desktop hover or keyboard focus shall
  disclose the full authored title. When available in loaded application data,
  the disclosure shall also show the current title, status and session-count
  badges, creation date, workflow, and parent. The task description shall not
  appear in the disclosure.
  Unknown session counts shall not appear as zero. Invalid dates shall be
  omitted. Parent titles shall use loaded records. Omit the parent row when its title
  is unavailable. Tasks loaded by the sidebar shall provide these
  details without requiring a board or task page to open first.
  Missing metadata shall show no invented status or workflow. Escape shall
  dismiss the disclosure without activating the link.
- **AC-UI-AGENT-TASK-REFERENCE-001.4:** Phone and coarse-pointer chips shall have
  a compact 24px minimum height that fits inline text. Long titles and adjacent chips shall wrap within
  the message without overlapping targets or document horizontal scrolling.
  A single tap shall navigate directly. The destination provides task details
  without a hover dependency or an intermediate drawer.
- **AC-UI-AGENT-TASK-REFERENCE-001.5:** Renaming a task shall not change a stored
  reference's destination or authored label. Duplicate titles shall remain
  distinguishable by their destinations. Unavailable tasks shall retain the
  existing task-route failure behavior without selecting another task.
- **AC-UI-AGENT-TASK-REFERENCE-001.6:** Bare titles, arbitrary IDs, and unrecognized
  links shall not become task chips. Code examples shall remain code. Rendering
  shall preserve stored content, raw view, copy/export behavior, and unrelated
  Markdown links. Partial streamed syntax shall not create a guessed task link.
- **AC-UI-AGENT-TASK-REFERENCE-001.7:** Task-mode and Office agent instructions
  shall describe one explicit reference format using a known task ID and title.
  The instructions shall not authorize task creation, task mutation, or guessed
  identities. References shall continue working after conversation refresh.

## Out of scope

- Composer changes, automatic title matching, or converting terminal output.
- New task cards, task creation tools, status polling, or task subscriptions.
- Reference decoration in files, task plans outside chat, shared exports,
  configuration chat, or third-party renderers.
- New task lifecycle, permission, persistence, or route contracts.

## Implementation plans

- [Agent task references](../../../plans/agent-task-references/plan.md)
