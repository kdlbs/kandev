---
status: shipped
created: 2026-05-02
owner: cfl
---

# Office Task Chat v2

## Why

The office task Chat tab shows comments but not the agent's work session. Users can't see what tools the agent used, what it was thinking, or how long it worked. The Activity tab shows "No activity yet" because task status changes aren't logged to the activity table. Users have to switch to advanced mode to see the full session transcript.

## What

### Expandable agent session in Chat tab

- Between comments in the Chat tab, agent work sessions appear as collapsible entries.
- Collapsed: shows "Agent worked for Xm Ys" with a chevron. If running, shows a spinner and "Agent working..." and auto-expands.
- Expanded: renders the existing `AdvancedChatPanel` component (reused from advanced mode) showing the full session transcript with tool calls, thinking, and messages.
- Sessions are identified by `task_sessions` linked to the task.

### Activity tab populated with task events

- Task status changes (CREATED → SCHEDULING → IN_PROGRESS → REVIEW → DONE) are logged to `office_activity_log` with `target_type=task` and `target_id=taskID`.
- The Activity tab shows these entries with the status transition and timestamp.

## Scenarios

- **GIVEN** a task where the CEO agent completed a turn, **WHEN** the user views the Chat tab, **THEN** they see a collapsible "Agent worked for 4s" entry alongside the auto-bridged comment. Expanding it shows the full session transcript with tool calls.

- **GIVEN** a task that transitioned from CREATED to REVIEW, **WHEN** the user views the Activity tab, **THEN** they see entries for each status change with timestamps.

- **GIVEN** an agent currently running on a task, **WHEN** the user views the Chat tab, **THEN** they see "Agent working..." with a spinner, auto-expanded, showing the live session transcript.

## Out of scope

- Multiple session support (show only the primary session)
- Custom transcript parsing (reuse existing `MessageRenderer`)
- Run-level cost tracking per session
