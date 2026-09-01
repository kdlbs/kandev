---
status: draft
system: ui
created: 2026-08-13
owners:
  - clem
---
# Prompt History Panel Requirements

## Overview

Reviewing what was asked of an agent requires scrolling the transcript; past prompts are easy to lose among long agent replies, and there is no way to see at a glance how long the agent worked on each prompt. A compact per-task history of prompts with send time and agent-work duration fixes both.

## Requirements

### REQ-UI-PROMPT-HISTORY-PANEL-001: Prompt History Panel

**Intent:** Reviewing what was asked of an agent requires scrolling the transcript; past prompts are easy to lose among long agent replies, and there is no way to see at a glance how long the agent worked on each prompt. A compact per-task history of prompts with send time and agent-work duration fixes both.

#### Acceptance criteria

- **AC-UI-PROMPT-HISTORY-PANEL-001.1:** A new optional panel, **Prompt history**, is available from the task workbench "+" menu (the `AddPanelMenuItems` dropdown) on the desktop workbench and the Office workbench, next to the existing Todos entry. On a phone, the same panel is available from the `Panels` bottom-navigation picker.
- **AC-UI-PROMPT-HISTORY-PANEL-001.2:** Passthrough sessions: the "+" menu does not offer the panel (the same `!state.isPassthrough` guard the Plan and Todos rows use), because passthrough sessions render a toolbar instead of a transcript. Because the panel is a reusable, persisted layout panel, a tab can still be present after a layout restore or a session/task switch into a passthrough session; in that case the panel content renders a passthrough empty state with NO navigation arrows (the transcript the arrow would jump to does not exist), instead of a dead control or a hidden-but-broken list.
- **AC-UI-PROMPT-HISTORY-PANEL-001.3:** The panel lists the prompts of the task's active session, newest first. A prompt is a message with `author_type === "user"`. Prompt history owns an independent, user-message-only projection and pagination cursor; it does not require the full transcript to be loaded. The panel always reflects the task's active session: when the active session changes (session tab click, session dropdown, or automatic handoff), it discards the previous projection and loads the new session. A task with multiple agent transcripts therefore shows exactly one session's prompts at a time — it never merges sessions — and switching the active session swaps the whole list. A task with no active session, or an active session whose prompt entries remain empty after loading and pagination are exhausted, shows the empty state; during initial loading it shows the neutral loading state.
- **AC-UI-PROMPT-HISTORY-PANEL-001.4:** Each row shows, on one line:
- **AC-UI-PROMPT-HISTORY-PANEL-001.5:** a clickable arrow button on the left;
- **AC-UI-PROMPT-HISTORY-PANEL-001.6:** a small `#N` number label at the start of the prompt bubble, in front of the prompt text, where `N` is the prompt's 1-based ordinal among ALL user messages of its session ordered by `created_at` ascending with ties broken by message `id` ascending (`#1` is the very first prompt of the whole session, and the newest prompt shows the highest number). The number is small type (`text-[10px]`-scale), distinct from the prompt text, and remains visible when the row is expanded. A prompt whose message carries no ordinal (older payloads, transient WS frames) renders without a number. Prompt numbers appear only in this panel, never in the transcript;
- **AC-UI-PROMPT-HISTORY-PANEL-001.7:** the prompt text truncated to a single line with an ellipsis (CSS truncation, so the visible character count adapts to panel width);
- **AC-UI-PROMPT-HISTORY-PANEL-001.8:** the time the prompt was sent, in compact relative form (`5m ago`, `3h ago`) with the absolute time in a `title` attribute;
- **AC-UI-PROMPT-HISTORY-PANEL-001.9:** Prompt history requests user messages with the session message API's `author_type=user` filter. It uses the returned cursor and `has_more` value for older prompt pages, and it keeps this pagination state separate from transcript pagination.
- **AC-UI-PROMPT-HISTORY-PANEL-001.10:** Session message create, update, and delete events reconcile both the transcript cache and the prompt projection. A stale HTTP response cannot overwrite a newer live event, and removing a session cancels its pending prompt updates.
- **AC-UI-PROMPT-HISTORY-PANEL-001.11:** Selecting a prompt activates the session chat. If the prompt is not in the loaded transcript, chat requests an around window for that message, merges the response, and scrolls to the prompt after it renders. Desktop and phone use the same behavior.

## System design

The migrated technical source is split into [part 1](../system-design/prompt-history-panel.md).
