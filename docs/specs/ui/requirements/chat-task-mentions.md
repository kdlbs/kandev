---
status: active
system: ui
created: 2026-10-21
owners:
  - kandev
---

# Chat Task Mention Requirements

## Overview

Selected task mentions should remain recognizable and navigable after sending
a message. UI owns this transcript reference presentation contract; Tasks
retains task identity, access, and lifecycle ownership.

## Requirements

### REQ-UI-CHAT-TASK-MENTION-001: Recognizable task references in user messages

**Intent:** Let users identify and open a task referenced in a sent chat bubble.

#### Acceptance criteria

- **AC-UI-CHAT-TASK-MENTION-001.1:** When a sent user message contains an
  `@title` mention backed by an unambiguous persisted task reference, the normal
  chat bubble shall display a distinct task chip with a task icon and the
  submitted title, with Markdown punctuation displayed literally. Its appearance shall distinguish it from ordinary text and
  saved-prompt mentions in light and dark themes.
- **AC-UI-CHAT-TASK-MENTION-001.2:** Clicking, keyboard-activating, or tapping
  the chip shall navigate to the referenced task by its recorded identity,
  including after refreshing the conversation or renaming the target task.
  The link shall expose an accessible name containing the submitted title and
  a visible keyboard focus indicator. Unavailable targets shall use the
  application's existing task-route failure behavior.
- **AC-UI-CHAT-TASK-MENTION-001.3:** Ordinary `@text`, malformed reference
  records, and titles associated with multiple different task identities in
  the same message shall remain text rather than guess a destination. Code,
  existing Markdown links, raw view, copied content, and agent input shall
  retain their existing semantics. Prompt and external-reference rendering
  outside recognized task mentions shall remain functional.
- **AC-UI-CHAT-TASK-MENTION-001.4:** On phones and coarse pointers, task chips
  shall provide at least a 44px active height and remain within the bubble for
  long titles and adjacent mentions. Tapping shall directly open the task
  without requiring hover or an intermediate preview. The conversation shall
  retain its existing scroll owner without document horizontal overflow.

## Out of scope

- Changing task search, composer selection, task permissions, or task lifecycle.
- Inferring task identities from live title searches or arbitrary typed text.
- Assistant messages, terminal output, and additional task preview surfaces.

## Implementation Plans

- [Chat task mentions](../../../plans/chat-task-mentions/plan.md)
