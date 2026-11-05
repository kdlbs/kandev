---
status: active
system: tasks
created: 2026-10-03
owners:
  - kandev
---

# Task unread dot requirements

## Overview

A task keeps working while the user looks at another task. When the user comes
back, the transcript shows a "New" divider before the first message that
arrived after the session's read cursor (see
[REQ-OFFICE-UNREAD-DIVIDER-001](../../office/requirements/unread-divider.md)).
The task list gives no hint that such a task has something new. This
requirement adds a dot to the task's row in the task list when one of the
task's sessions has a message the user has not read. The task system owns it
because the dot is a per-task runtime-state projection delivered with the
task's status summary. The Quick Chat entry already shows similar activity
dots; those are unchanged.

The dot reuses the divider's read cursor but not its first-visit rule. A
session that was never opened has no cursor and shows no divider on its first
visit, but its messages still make the task unread, so a user who launches
several background tasks sees which of them produced output. The user's own
messages never make a task unread.

The dot has its own **Show unread dot on tasks** setting, shown directly under
**Show New divider in transcripts** and defaulting off. The dot is therefore
opt-in and independent of the divider: either setting keeps the shared read
cursor advancing, and only the dot setting shows or hides the dot. Quick Chat
dots are always on; this feature is not. While the dot is on, an **Unread dot
hide delay** setting, default 2 seconds and configurable from 0 to 60, keeps an
opened task's dot visible briefly so the user sees which task they opened. The
delay changes only how long the dot stays on screen; messages are marked read
as soon as the transcript shows them.

## Terminology

- **Read cursor:** The session-global `last_read_message_id` described by the
  unread-divider requirement.
- **Unread message:** A transcript-visible message, not written by the user,
  that comes after a session's read cursor, or any such message in a session
  that has no read cursor. The message kinds named in criterion .10 are
  excluded.
- **Unread dot:** A small marker on a task row meaning that at least one of the
  task's sessions has an unread message.
- **Open task:** The task whose row is selected in the task list because the
  user is viewing it.
- **Displayed session:** The session whose transcript the open task currently
  shows.

## Requirements

### REQ-TASKS-TASK-UNREAD-DOT-001: Task unread dot

**Intent:** Let the user see in the task list which tasks received messages
since they last read them, without opening each task.

**User story:** As a user running several tasks, I want a dot on task rows with
new messages, so that I know which tasks to return to.

#### Acceptance criteria

- **AC-TASKS-TASK-UNREAD-DOT-001.1:** When the unread task dot setting is enabled, a task has a session with an unread message, and the task is not the open task, the system shall show an unread dot on that task's row, whether or not the New divider is enabled.
- **AC-TASKS-TASK-UNREAD-DOT-001.2:** When a session has no read cursor and contains a qualifying message not written by the user, the system shall count the session as unread. The New divider shall keep its first-visit rule and shall not appear on that session's first visit.
- **AC-TASKS-TASK-UNREAD-DOT-001.3:** When the unread task dot setting is disabled, which is its default, the system shall show no unread dot on any task row.
- **AC-TASKS-TASK-UNREAD-DOT-001.4:** When the unread task dot setting is enabled again, the system shall show the dot on tasks that still have unread messages, and while it is enabled the displayed session shall advance its read cursor even when the New divider is disabled.
- **AC-TASKS-TASK-UNREAD-DOT-001.5:** When a message arrives in the open task's displayed session, the system shall not show an unread dot on that task's row. When the user opens a task whose row shows the dot, the system shall keep the dot for the configured hide delay (default 2 seconds, 0 to 60 seconds, 0 hides it immediately) and then hide it unless criterion .17 applies; opening another task ends the delay early.
- **AC-TASKS-TASK-UNREAD-DOT-001.6:** When the displayed transcript has shown every message the client has loaded for a session, the system shall record the session as read through its newest loaded message, including messages the transcript does not render but excluding action-footer notices, which never count as unread and can be deleted, so that reading a session to its end always clears that session's contribution to the dot.
- **AC-TASKS-TASK-UNREAD-DOT-001.7:** When a message arrives for a task that is not the open task, the dot shall appear on its row without a page reload.
- **AC-TASKS-TASK-UNREAD-DOT-001.8:** When the page reloads, the browser reconnects, or another browser window loads the task list, the dot shall reflect the persisted read cursors and the stored messages, including tasks that were unread before a backend upgrade or restart.
- **AC-TASKS-TASK-UNREAD-DOT-001.9:** When a task has several sessions, the dot shall show while any of them has an unread message and shall clear only when all of them have caught up.
- **AC-TASKS-TASK-UNREAD-DOT-001.10:** Setup script output, subagent child messages, the "New session started" and "Session resumed" status notices, recovery action cards, action-footer notices (such as git-failure, stall, and retry notices that carry action buttons), permission requests, clarification requests, and message types the transcript hides shall not by themselves make a task unread.
- **AC-TASKS-TASK-UNREAD-DOT-001.11:** After a session is deleted and an authoritative unread-state refresh succeeds, it shall stop contributing to its task's dot. If the refresh fails at deletion, the system shall preserve the last-known unread state until a later successful refresh, which shall exclude the deleted session.
- **AC-TASKS-TASK-UNREAD-DOT-001.12:** A session whose tab was hidden instead of deleted shall keep contributing to its task's dot until the user reopens and reads it.
- **AC-TASKS-TASK-UNREAD-DOT-001.13:** The dot shall expose its state to assistive technology through a localized accessible name that states the task has unread messages.
- **AC-TASKS-TASK-UNREAD-DOT-001.14:** The desktop task list shall show the dot using its existing row layout.
- **AC-TASKS-TASK-UNREAD-DOT-001.15:** The mobile task switcher shall show the dot using its existing row layout and touch target.
- **AC-TASKS-TASK-UNREAD-DOT-001.16:** Showing or hiding the dot shall not change the row's height.
- **AC-TASKS-TASK-UNREAD-DOT-001.17:** While the open task has an unread message in a session other than its displayed session, the system shall keep the dot on that task's row after the hide delay.
- **AC-TASKS-TASK-UNREAD-DOT-001.18:** When the user writes a message in a session, that message shall not by itself make the session unread, whether or not the session has a read cursor.
- **AC-TASKS-TASK-UNREAD-DOT-001.19:** The hide delay control shall be available only while the unread task dot setting is enabled, and its description shall state that the delay changes only how long the dot stays visible, not when messages are marked read.

## Out of scope

- Unread counts or per-message badges. The dot is boolean.
- Per-user or per-device read state. The cursor stays session-global.
- Dots on kanban cards, session tabs, the command palette, browser tab titles, favicons, sounds, or operating-system notifications.
- Any change to the Quick Chat activity indicators, the divider's own placement, the divider's first-visit rule, or the cursor's monotonic write rule.
- Any change to how permission and clarification requests reach the user. They keep using the existing needs-attention affordance.
- Coupling the dot to the New divider setting. Each has its own setting.
- Suppressing dots for tasks that were never opened before the setting was enabled. Enabling the dot marks every listed task whose sessions hold unread agent output, and opening a task clears it.
