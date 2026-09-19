---
status: draft
system: tasks
created: 2026-08-03
owners:
  - kandev
---
# Missing task route recovery Requirements

## Overview

Preserve the observable behavior documented for Missing task route recovery.

## Requirements

### REQ-TASKS-MISSING-TASK-ROUTE-RECOVERY-001: Missing task route recovery

**Intent:** Preserve the observable behavior documented for Missing task route recovery.

#### Acceptance criteria

- **AC-TASKS-MISSING-TASK-ROUTE-RECOVERY-001.1:** A failed cold load of `/t/:taskId` or the compatibility `/tasks/:taskId` renders the existing generic unavailable state without revealing whether the task was deleted, never existed, or is outside the current user's access.
- **AC-TASKS-MISSING-TASK-ROUTE-RECOVERY-001.2:** Failure to load the requested task does not prevent the app shell from resolving the user's authorized Kanban workspace with the existing explicit URL parameter, active-workspace cookie, user-setting, and default-workspace precedence.
- **AC-TASKS-MISSING-TASK-ROUTE-RECOVERY-001.3:** When the resolved workspace contains other visible tasks, the desktop task sidebar shows them and its rows remain usable navigation targets. The invalid route ID is not synthesized as a sidebar task.
- **AC-TASKS-MISSING-TASK-ROUTE-RECOVERY-001.4:** When a visible sibling has a loaded session, selecting its sidebar row keeps that sibling active after later session and WebSocket updates.
- **AC-TASKS-MISSING-TASK-ROUTE-RECOVERY-001.5:** When a task has multiple sessions, selecting a non-primary session, visiting another task, and returning to the first task restores the selected session instead of reapplying the route's primary session.
- **AC-TASKS-MISSING-TASK-ROUTE-RECOVERY-001.6:** The unavailable state includes a visible action back to the task overview for the resolved workspace. The action is keyboard accessible, touch reachable, and usable when no workspace can be resolved.
- **AC-TASKS-MISSING-TASK-ROUTE-RECOVERY-001.7:** A valid task-detail boot continues to hydrate from that task's workspace and does not additionally load or briefly expose fallback workspace data.
- **AC-TASKS-MISSING-TASK-ROUTE-RECOVERY-001.8:** Workspace and task authorization remain unchanged. Fallback state contains only workspaces, workflows, repositories, and task snapshots already visible to the current request identity.

## Migrated source detail

## Problem

A cold navigation to a Kanban task-detail URL whose task is missing, deleted,
or inaccessible renders the intended **Task unavailable** state, but the same
failed task lookup also prevents the app shell from hydrating its authorized
workspace and workflow context. The desktop sidebar therefore reports that
there are no tasks even when the selected workspace contains other tasks, and
the unavailable route offers no direct recovery action on phone viewports where
the desktop sidebar is hidden.

## Desired behavior

- A failed cold load of `/t/:taskId` or the compatibility `/tasks/:taskId`
  renders the existing generic unavailable state without revealing whether the
  task was deleted, never existed, or is outside the current user's access.
- Failure to load the requested task does not prevent the app shell from
  resolving the user's authorized Kanban workspace with the existing explicit
  URL parameter, active-workspace cookie, user-setting, and default-workspace
  precedence.
- When the resolved workspace contains other visible tasks, the desktop task
  sidebar shows them and its rows remain usable navigation targets. The invalid
  route ID is not synthesized as a sidebar task.
- When a visible sibling has a loaded session, selecting its sidebar row keeps
  that sibling active after later session and WebSocket updates.
- When a task has multiple sessions, selecting a non-primary session, visiting
  another task, and returning to the first task restores the selected session
  instead of reapplying the route's primary session.
- The unavailable state includes a visible action back to the task overview for
  the resolved workspace. The action is keyboard accessible, touch reachable,
  and usable when no workspace can be resolved.
- A valid task-detail boot continues to hydrate from that task's workspace and
  does not additionally load or briefly expose fallback workspace data.
- Workspace and task authorization remain unchanged. Fallback state contains
  only workspaces, workflows, repositories, and task snapshots already visible
  to the current request identity.
- Loading feedback remains visible while the task result is unresolved; the
  unavailable state and recovery controls appear only after failure is known.

## Regression scenarios

- **GIVEN** an authorized workspace containing another visible task, **WHEN**
  the user cold-loads `/t/<missing-id>`, **THEN** the unavailable state appears,
  the desktop sidebar lists the other task instead of **No tasks yet**, and
  selecting that row opens the valid task.
- **GIVEN** a visible sibling with a loaded primary session, **WHEN** the user
  selects it from a missing task route, **THEN** the sibling route and workbench
  stay active after session hydration and WebSocket updates.
- **GIVEN** task A has a selected non-primary session, **WHEN** the user
  switches to task B and returns to task A, **THEN** task A keeps that selected
  session active.
- **GIVEN** the active-workspace cookie and user settings identify different
  authorized workspaces, **WHEN** a missing task route is cold-loaded, **THEN**
  fallback task data comes from the cookie-selected workspace according to the
  existing boot precedence.
- **GIVEN** a valid task-detail URL, **WHEN** it is cold-loaded, **THEN** its
  route-specific boot data remains authoritative and no fallback workspace
  bootstrap is added to the top-level initial state.
- **GIVEN** a user cannot access the requested task but can access another
  workspace, **WHEN** the task route loads, **THEN** the generic unavailable
  state and only that user's authorized fallback workspace tasks are shown.
- **GIVEN** no Kanban workspace is available, **WHEN** a missing task route
  loads, **THEN** the unavailable state remains visible, the sidebar stays
  empty, and the task-overview action remains safe to activate.
- **GIVEN** a phone viewport on a missing task route, **WHEN** the unavailable
  state renders, **THEN** the task-overview action fits the viewport, has a
  touch-sized target, and opens the mobile task overview without relying on the
  desktop sidebar.

## Constraints

- The Go-served SPA boot payload remains the first-paint source of workspace
  context, consistent with ADRs 0021 and 0023.
- The repair reuses the existing task, workspace, workflow, and snapshot
  contracts; it adds no API, persistence, WebSocket, or permission contract.
- Fallback hydration is conditional on unavailable task-detail route data so a
  successful task boot does not duplicate its route-specific resource loading.

## Out of scope

- Changing task lookup status codes or disclosing why a task is unavailable.
- Creating a placeholder task for the invalid route ID.
- Redesigning the desktop task sidebar, task switcher, or Kanban overview.
- Changing Office task-detail not-found behavior under `/office/tasks/:id`.
- Retrying backend failures beyond the existing task-detail client retry.

## Implementation plans

- [Missing task route recovery](../../../plans/missing-task-route-recovery/plan.md)
- [Missing task session navigation repair](../../../plans/missing-task-session-navigation/plan.md)
