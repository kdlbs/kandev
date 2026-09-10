---
status: draft
system: tasks
created: 2026-09-10
owners:
  - kandev
---

# Task removal navigation requirements

## Overview

Archive and delete should produce one visible departure from the selected task.
The user should not see session teardown, resume controls, or unarchive controls
appear while that departure is in progress. Tasks owns this contract because
removal, replacement eligibility, and recovery depend on the task lifecycle.

## Terminology

- **Accepted action:** confirmation was submitted, or archive was selected with
  confirmation disabled. Opening or cancelling a confirmation is not acceptance.
- **Removal set:** explicitly selected tasks plus descendants included by an
  explicit cascade choice.
- **Outgoing task:** a task in the removal set currently shown in detail or preview.

## Requirements

### REQ-TASKS-REMOVAL-NAVIGATION-001: Immediate departure

**Intent:** Keep task cleanup states out of the outgoing view.

#### Acceptance criteria

- **AC-TASKS-REMOVAL-NAVIGATION-001.1:** After an accepted action, the outgoing
  detail or preview shall stop presenting live task content before removal can
  change it. It shall not show newly appearing session, resume, unarchive, or
  unavailable-task controls during the operation. Cancelling confirmation shall
  leave the task visible and issue no removal request.
- **AC-TASKS-REMOVAL-NAVIGATION-001.2:** While a destination is unresolved, the
  view shall show neutral, accessible loading feedback and keep navigation
  usable. It shall not expose the outgoing task behind a translucent overlay or
  wait for removal success before hiding its content.
- **AC-TASKS-REMOVAL-NAVIGATION-001.3:** Removing a task from a detail route shall
  select a surviving task from the current workspace, ordered by recent use
  and then current board order. Archived, unavailable, and removal-set tasks
  shall be ineligible. If no eligible task can be established, the task overview
  shall open. The rendered identity and URL shall agree without a full reload.
- **AC-TASKS-REMOVAL-NAVIGATION-001.4:** Removing the previewed task shall close
  that preview and retain the board context. Removing an unselected task shall
  not change the current task, session, route, preview, or keyboard focus.
- **AC-TASKS-REMOVAL-NAVIGATION-001.5:** The same behavior shall apply to desktop
  and phone entry points, including bulk actions. Phone menus shall close after
  acceptance; the destination shall use the existing full task or overview
  surface. Focus shall move to the destination or its loading status, without
  returning to a removed control.

### REQ-TASKS-REMOVAL-NAVIGATION-002: Recovery and concurrent navigation

**Intent:** Preserve user choices and truthful task state while removal completes.

#### Acceptance criteria

- **AC-TASKS-REMOVAL-NAVIGATION-002.1:** If removal fails and the original task
  remains available, the system shall restore its previous view and valid
  session only when no later user navigation occurred. Otherwise it shall keep
  the user's current view and report the error. A dirty-worktree refusal shall
  retain existing explicit discard confirmation and retry guidance.
- **AC-TASKS-REMOVAL-NAVIGATION-002.2:** Delayed destination checks, session
  loads, removal responses, and live events shall not override later user
  navigation, including navigation away and back to the same destination.
- **AC-TASKS-REMOVAL-NAVIGATION-002.3:** Repeated submission of the same pending
  removal shall not issue duplicate mutations. Bulk partial failure shall retain
  failed tasks for retry and shall not restore successfully removed tasks.
- **AC-TASKS-REMOVAL-NAVIGATION-002.4:** Pending removal shall not claim archive
  or deletion success. Success shall produce one concise notification per
  operation or bulk batch. Uncertain failures shall not recreate a deleted task,
  unarchive a task, or start a session to restore the view.
- **AC-TASKS-REMOVAL-NAVIGATION-002.5:** The outgoing task shall not create or
  restart a session because removal empties its session list. After confirmed
  failure, ordinary task behavior shall resume only for an available task.

## Compatibility and exclusions

Existing archive confirmation preferences and cascade choices remain governed
by [archive confirmation](archive-confirmation.md). Backend deletion admission,
cleanup, and discard consent remain governed by [runtime cleanup](runtime-cleanup.md).
This adds local user-action presentation guarantees; it does not change task APIs,
permissions, session-only deletion, Quick Chat expiration/close, or server cleanup.
Remote/API/MCP removal retains existing lifecycle reconciliation and redirects.
Cold unavailable task routes retain [their current contract](missing-task-route-recovery.md).
Undo, animations, new settings, and a new mobile navigation composition are excluded.

## System design

- [Task removal navigation](../system-design/removal-navigation.md)

## Implementation plans

- [Task removal navigation](../../../plans/task-removal-navigation/plan.md)
