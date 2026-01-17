# Relationship Events LLD

This document defines backend-driven state propagation and how WS notifications mirror those changes to the UI. Clients do **not** emit these events.

## Core Rules

1) **Session state changes drive task state changes**.
2) **Agentctl lifecycle changes drive session state changes**, which then drive task state changes.
3) **Task state changes may move columns**, but only between **In Progress** and **Review** for now.

All state propagation happens **server-side** on internal events. WebSocket events are outbound mirrors for UI synchronization.

---

## 1) Session State -> Task State

**Trigger (backend only)**
- `events.TaskSessionStateChanged` (internal event bus)

**Flow**
1. Resolve `task_id` from `session_id`.
2. Map `TaskSessionState -> TaskState` (table below).
3. If task state changes, call `taskSvc.UpdateTaskState(taskID, newState)`.
4. Emit `events.TaskStateChanged` and WS `task.state_changed`.
5. Emit WS `session.state_changed` for clients.

**Mapping Table**

| TaskSessionState  | TaskState     | Board Column          |
|-------------------|---------------|-----------------------|
| CREATED           | SCHEDULING    | In Progress (if any)  |
| STARTING          | SCHEDULING    | In Progress (if any)  |
| RUNNING           | IN_PROGRESS   | In Progress           |
| WAITING_FOR_INPUT | REVIEW        | Review                |
| COMPLETED         | REVIEW        | Review                |
| ERROR             | ERROR         | (no auto-move)        |
| CANCELLED         | CANCELLED     | (no auto-move)        |

Notes:
- `WAITING_FOR_INPUT -> REVIEW` and `COMPLETED -> REVIEW` are the default paths. If a board has no Review column, keep task in its current column.
- If the mapped task state equals current task state, do nothing.

---

## 2) Agentctl State -> Session State -> Task State

**Triggers (backend only)**
- `events.AgentctlStarting`
- `events.AgentctlReady`
- `events.AgentctlError`

**Agentctl -> Session Mapping**

| Agentctl Event           | Session State |
|--------------------------|---------------|
| AgentctlStarting         | STARTING      |
| AgentctlReady            | RUNNING       |
| AgentctlError            | FAILED        |

**Flow**
1. On Agentctl event, update session state in DB.
2. Emit `events.TaskSessionStateChanged` (which triggers session->task mapping).
3. Emit WS `session.agentctl_*` and `session.state_changed`.

---

## 3) Task State -> Column Move

**Trigger (backend only)**
- `events.TaskStateChanged`

**Policy (for now)**
- Only move tasks **between In Progress and Review**.
- Do not auto-move for other states.

**Flow**
1. When task state changes:
   - If new state is `IN_PROGRESS`, move to column mapped to In Progress (if exists).
   - If new state is `REVIEW`, move to column mapped to Review (if exists).
2. If column lookup fails or column already matches, do nothing.
3. Emit `events.TaskUpdated` and WS `task.updated` with new `column_id`.

---

## WebSocket Notifications (Outbound Only)

- `session.state_changed`
- `session.agentctl_starting`
- `session.agentctl_ready`
- `session.agentctl_error`
- `task.state_changed`
- `task.updated`

Clients should treat these as **read-only state mirrors**.

---

## E2E Flow (Client-Initiated)

| Edge Trigger (Client)                         | Backend Action                                    | Internal Event(s)                    | WS Notification(s)                              |
|----------------------------------------------|---------------------------------------------------|--------------------------------------|-------------------------------------------------|
| `POST /api/v1/tasks` (start_agent: true)     | Create task + create session + start agentctl     | task.created, task_session.created   | task.created, session.agentctl_starting         |
| `POST /api/v1/tasks` (start_agent: false)    | Create task + create session (idle)               | task.created, task_session.created   | task.created                                    |
| `message.add` (session message)              | Persist message + prompt agent via ACP            | message.added                        | session.message.added                           |
| `task.state` (manual update)                 | Update task state                                 | task.state_changed                   | task.state_changed, task.updated (column move)  |
| `task.session.resume`                        | Resume session + start agentctl                   | task_session.state_changed           | session.agentctl_starting, session.state_changed |

Notes:
- Column move only occurs for `IN_PROGRESS` / `REVIEW` transitions.

---

## E2E Flow (Agent / agentctl-Initiated)

| Edge Trigger (Agent/agentctl)                | Backend Action                                    | Internal Event(s)                    | WS Notification(s)                              |
|----------------------------------------------|---------------------------------------------------|--------------------------------------|-------------------------------------------------|
| Agentctl starting                            | Update session state → STARTING                   | task_session.state_changed           | session.agentctl_starting, session.state_changed |
| Agentctl ready                               | Update session state → RUNNING                    | task_session.state_changed           | session.agentctl_ready, session.state_changed   |
| Agentctl error                               | Update session state → ERROR                      | task_session.state_changed           | session.agentctl_error, session.state_changed   |
| ACP content/progress                         | Create/append message chunks                      | message.added / message.updated      | session.message.added / session.message.updated |
| ACP input_required                           | Create message (requests_input=true)              | message.added                        | session.message.added + session.waiting_for_input |
| Git status stream update                     | Update session git status                         | git.status.updated                   | session.git.status                              |
| Workspace file change stream                 | Update session file activity                      | workspace.file.changed               | session.workspace.file.changes                  |
| Shell output stream                          | Append shell output                               | shell.output                         | session.shell.output                            |
