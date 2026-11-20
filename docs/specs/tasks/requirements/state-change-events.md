---
status: draft
system: tasks
created: 2026-11-11
owners:
  - kandev
---

# Task State-Change Events Requirements

## Overview

Task-state consumers need the same notification regardless of the surface that
changes a task. Office status controls and tree controls use this contract too.
The task system owns this contract because it owns persisted task state.

This record makes the existing canonical event contract explicit. Dependency,
parent-completion, handoff, and scheduling policies retain their own owners.

## Terminology

- **State transition:** A committed change from one task-state value to another.
- **Canonical event:** The existing `task.state_changed` event.
- **Producer duplicate:** Two publications for one transition from overlapping
  application paths. Transport redelivery is a separate concern.

## Requirements

### REQ-TASKS-STATE-CHANGE-EVENTS-001: Consistent state-change notification

**Intent:** Subscribers must observe committed task-state transitions without a
restart, a page reload, or knowledge of the mutation surface.

#### Acceptance criteria

- **AC-TASKS-STATE-CHANGE-EVENTS-001.1:** When an application surface commits a
  state transition, it shall publish one canonical event for that transition.
  This includes Office user actions, agent actions, automatic completion, tree
  cancellation, and tree restoration.
- **AC-TASKS-STATE-CHANGE-EVENTS-001.2:** The canonical event shall retain its
  existing task payload and include `task_id`, `workspace_id`, `state`,
  `old_state`, and `new_state`. The previous and new states shall describe the
  committed transition, including concurrent-write cases.
- **AC-TASKS-STATE-CHANGE-EVENTS-001.3:** When validation, a completion guard, or
  a storage error prevents a transition, the system shall publish no canonical
  event for that attempt. An unchanged state shall produce no canonical event.
- **AC-TASKS-STATE-CHANGE-EVENTS-001.4:** When an approval gate commits a redirect
  to `REVIEW`, the event shall describe that redirect. The existing
  approvals-pending response shall remain. The attempt shall produce no
  `COMPLETED` event or completion action.
- **AC-TASKS-STATE-CHANGE-EVENTS-001.5:** A tree operation shall publish an event
  for each member whose state changes. Observers shall see the complete
  committed cancellation batch before the first member notification. Already
  cancelled members shall produce no cancellation transition or restoration
  transition. Pause and resume shall produce no task-state event.
- **AC-TASKS-STATE-CHANGE-EVENTS-001.6:** When a later cleanup operation fails
  after a state commit, observers shall still receive that transition. When
  restoration commits only some members, observers shall receive each committed
  member transition before the operation reports its error.
- **AC-TASKS-STATE-CHANGE-EVENTS-001.7:** An Office state transition shall create
  one status activity row with the existing actor and run attribution. That row
  shall exist before either the Office notification or canonical notification.
  Existing Office and task refresh notifications shall remain available.
- **AC-TASKS-STATE-CHANGE-EVENTS-001.8:** When notification paths overlap, each
  downstream launch, Office wake, parent completion, handoff completion, or
  coordinator episode shall retain its existing duplicate-suppression behavior.
  For a dependent with multiple blockers, all blockers must satisfy its existing
  policy before the dependent starts.
- **AC-TASKS-STATE-CHANGE-EVENTS-001.9:** Concurrent completion requests and
  synchronous subscriber reentry shall finish without duplicate producer events
  or deadlock. Existing workflow-step, expected-state, and completion guards
  shall remain effective.
- **AC-TASKS-STATE-CHANGE-EVENTS-001.10:** Caller cancellation after a commit
  shall not suppress the notification attempt. Notification work shall retain
  the existing bounded service-owned publication behavior.

## Related contracts

- [Task dependencies](task-dependencies.md) owns success-only Kanban admission.
- [Subtask completion](subtask-completion-trigger.md) owns the parent trigger.
- [Subtree controls](subtree-controls.md) owns hold membership and restoration.
- [Office tasks](../../office/requirements/tasks.md) owns approval and reactivity
  policy.
- [Runtime publication order](runtime-state-publication-order.md) owns the
  existing task-before-running-session ordering.

## Out of scope

- New task-state values, event subjects, or user controls.
- Changes to blocker satisfaction, recipient selection, or workflow policy.
- A general audit or migration of unrelated task writers.
- Guaranteed delivery after process failure or exactly-once transport delivery.
- New persistence tables, runtime flags, or recovery sweeps.
- Desktop or phone layout, navigation, touch, or scrolling changes.
