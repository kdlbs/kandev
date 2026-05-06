---
status: shipped
created: 2026-05-02
owner: cfl
---

# Office Real-Time Updates

## Why

Every office page fetches data once on mount and never updates. When an agent completes a task, changes status, posts a comment, or creates a subtask, the user sees stale data until they manually refresh. This breaks the "always-on agent orchestration" experience — users should see their agents working in real-time without refreshing.

## What

### WebSocket event forwarding

- The backend already publishes internal events on the event bus (task.updated, task.moved, office.comment.created, agent.completed, etc.).
- A new WS handler subscribes to office-relevant events and forwards them to connected clients scoped by workspace.
- The frontend WS client receives these events and updates the store. All pages read from the store, so they re-render automatically.

### Events to forward

| Backend Event | WS Action | Store Update | Pages Affected |
|---|---|---|---|
| `task.updated` | `office.task.updated` | Update task in `office.tasks.items` | Tasks list, dashboard recent tasks |
| `task.moved` | `office.task.moved` | Update task status | Tasks list, dashboard metrics |
| `task.created` | `office.task.created` | Add to `office.tasks.items` | Tasks list, dashboard task count |
| `office.comment.created` | `office.comment.created` | Append to comments | Task detail chat |
| `office.task.status_changed` | `office.task.status_changed` | Update task status | Tasks list, dashboard |
| `agent.completed` | `office.agent.completed` | Update agent status | Agents list, dashboard, task detail |
| `agent.failed` | `office.agent.failed` | Update agent status | Agents list, dashboard |
| `office.approval.created` | `office.approval.created` | Increment inbox count | Inbox, dashboard approvals count |
| `office.approval.resolved` | `office.approval.resolved` | Update inbox | Inbox |
| `office.wakeup.queued` | `office.wakeup.queued` | Update agent run state | Agent detail runs tab |
| `office.activity.created` | `office.activity.created` | Prepend to activity | Dashboard recent activity, Activity page |

### Per-page subscriptions

Each page subscribes to the events it needs when mounting:

- **Dashboard**: task.created, task.moved, agent.completed, activity.created, approval.created
- **Tasks list**: task.created, task.updated, task.moved, task.status_changed
- **Task detail**: comment.created, task.status_changed (+ existing session WS)
- **Agents list**: agent.completed, agent.failed, wakeup.queued
- **Agent detail**: agent.completed, wakeup.queued, activity.created
- **Inbox**: approval.created, approval.resolved
- **Activity**: activity.created

### Workspace scoping

Events are scoped by workspace ID. The WS handler only forwards events for the client's active workspace. Clients send their active workspace ID on connect (already available from user settings).

## Scenarios

- **GIVEN** a user viewing the tasks list, **WHEN** an agent creates a subtask, **THEN** the new task appears in the list without refreshing.

- **GIVEN** a user viewing the dashboard, **WHEN** an agent completes a task and moves it to REVIEW, **THEN** the "Tasks In Progress" count decreases and "Recent Activity" shows the status change.

- **GIVEN** a user viewing the inbox, **WHEN** an agent requests approval, **THEN** the inbox count badge increments and the approval appears in the list.

- **GIVEN** a user viewing an task detail page, **WHEN** the agent posts a comment via `kandev comment add`, **THEN** the comment appears in the Chat tab without refreshing.

## Out of scope

- Live streaming of agent transcripts in the office chat (separate from event forwarding)
- Cross-workspace event subscriptions (only active workspace)
- Offline/reconnection queue (existing WS reconnection handles this)
- Optimistic UI updates (wait for server confirmation via event)
