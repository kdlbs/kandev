---
status: shipped
created: 2026-05-02
owner: cfl
---

# Office Task Chat

## Why

When an office agent completes a task, its response is stored in `task_session_messages` but the office task detail page's Chat tab only shows `task_comments`. Users see "No comments yet" even though the agent responded. There is no unified view of what happened on a task — agent responses, user comments, status changes, and system events are scattered across different tables and pages.

## What

### Auto-bridge agent responses to comments

- When an agent completes a turn, the orchestrator auto-posts the agent's final text message as a `task_comment` with `author_type=agent` and `source=session`.
- Only messages of type `message` (not `status`, `script_execution`, etc.) are bridged.
- The comment includes the agent's name as `author_id`.
- Duplicate comments are prevented by checking if a comment with the same `source=session` already exists for the same turn.

### Merged chat thread

- The Chat tab displays a chronological merge of:
  1. **User comments** — posted via the comment input or API
  2. **Agent comments** — auto-bridged from session messages, or posted via `kandev comment add`
  3. **Timeline events** — status changes (SCHEDULING, IN_PROGRESS, REVIEW, DONE), assignee changes
- Each entry shows: author avatar/initial, author name, timestamp, and content.
- Agent comments from auto-bridge show the agent name and a "via session" indicator.
- Timeline events are rendered as compact system entries (e.g. "Status changed to IN_PROGRESS").

### Reply wakes agent

- The comment input at the bottom of the Chat tab posts a `task_comment`.
- Posting a comment on a task that has an `assignee_agent_instance_id` queues a `task_comment` wakeup for the assigned agent.
- The agent receives the comment text in its wakeup context and can respond.

### Sidebar counters

- The sidebar entries for Tasks, Skills, and Routines show count badges.
- Counts are fetched from the dashboard API (which already returns `agent_count`) or dedicated count endpoints.
- Counts update when the user navigates between pages or when data changes.

## Scenarios

- **GIVEN** a CEO agent that just completed a "present yourself" task, **WHEN** the user opens the task detail page, **THEN** the Chat tab shows the agent's response as a comment entry with the agent's name and timestamp.

- **GIVEN** an task with 2 agent comments and 1 user comment, **WHEN** the user views the Chat tab, **THEN** all 3 entries appear in chronological order with distinct styling for agent vs user comments.

- **GIVEN** an task in REVIEW status, **WHEN** the user types a reply and clicks Send, **THEN** a comment is created, the agent is woken with a `task_comment` wakeup, and the comment appears in the chat immediately.

- **GIVEN** a task that transitioned from TODO to IN_PROGRESS to REVIEW, **WHEN** the user views the Chat tab, **THEN** timeline events for each status change appear inline between comments.

- **GIVEN** a workspace with 5 tasks, 3 skills, and 2 routines, **WHEN** the user views the office sidebar, **THEN** count badges appear next to Tasks (5), Skills (3), and Routines (2).

## Out of scope

- Live streaming of agent responses in the office chat (exists in the kanban task detail page)
- Related work tab (inbound/outbound task references)
- Thread interactions (suggest tasks, request confirmation) — future feature
- File attachments on comments
