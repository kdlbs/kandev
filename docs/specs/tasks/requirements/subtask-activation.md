---
status: active
system: tasks
created: 2026-11-03
owners:
  - kandev
---

# Subtask activation

## Purpose and ownership

After successful subtask creation, users need to open the child's conversation
without an unrelated session error. Tasks owns creation and session identity.
The [UI navigation contract](../../ui/requirements/task-navigation-responsiveness.md)
owns reusable navigation and cached-content behavior.

## Requirements

### REQ-TASKS-SUBTASK-ACTIVATION-001: Open the created child's conversation

**Intent:** Keep the visible task, selected conversation, and session operations
consistent when creation opens a child task.

#### Acceptance criteria

- **AC-TASKS-SUBTASK-ACTIVATION-001.1:** When subtask creation returns a session, the browser shall open that child task and its returned conversation. The route and visible task shall agree without a document reload. The creation dialog shall close before navigation.
- **AC-TASKS-SUBTASK-ACTIVATION-001.2:** During that transition, each conversation shown and each status or recovery request shall belong to its rendered task. An unknown or foreign session shall not supply that task's conversation or session operations. Delayed child data shall not produce a parent-task/child-session request or an ownership error.
- **AC-TASKS-SUBTASK-ACTIVATION-001.3:** When creation fails or returns no session, the current task and conversation shall remain selected. A creation failure shall retain existing error feedback. The browser shall not repeat creation or start another session to correct navigation.
- **AC-TASKS-SUBTASK-ACTIVATION-001.4:** When a navigation guard defers or cancels opening the child, the current task and conversation shall remain consistent. Proceeding shall open the already created child without another creation request.
- **AC-TASKS-SUBTASK-ACTIVATION-001.5:** Desktop and phone shall provide the same creation and activation outcome through their existing controls. The phone shall open the child's focused conversation surface with reachable navigation and no document horizontal overflow.

## Exclusions

Creation payloads, workspace inheritance, agent selection, launch policy,
backend authorization, persistence, other task-switch callers, and dialog
geometry changes remain outside this capability.

## Design and delivery

- [System design](../system-design/subtask-activation.md)
- [Fix package](../../../plans/subtask-activation/plan.md)
