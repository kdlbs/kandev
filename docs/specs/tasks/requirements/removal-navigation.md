---
status: active
system: tasks
created: 2026-09-10
updated: 2026-09-17
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

### REQ-TASKS-REMOVAL-NAVIGATION-003: Immediate archive visibility

**Intent:** Show accepted archive targets as busy until they are removed from
active task navigation after the archive operation completes.

#### Acceptance criteria

- **AC-TASKS-REMOVAL-NAVIGATION-003.1:** After archive acceptance, every visible
  active task in the removal set shall remain in the desktop sidebar and phone
  task picker in a dimmed, busy state with a spinner on the next render, before
  the archive request, destination lookup, or live event completes. The row
  shall keep its place and expose no stale interactive state. Opening or
  cancelling confirmation shall leave rows unchanged.
- **AC-TASKS-REMOVAL-NAVIGATION-003.2:** Refreshes and live updates during the
  operation shall preserve the pending presentation. Successful targets shall
  be removed after completion and failed targets that remain active shall return
  to their normal presentation without overwriting newer task data or user
  navigation. Bulk partial failure shall restore only failed targets.
- **AC-TASKS-REMOVAL-NAVIGATION-003.3:** Archiving an unselected task shall leave
  the selected task and route unchanged. Non-cascade archive shall not hide
  surviving subtasks. Existing archived-inclusive saved views shall continue to
  show confirmed archived tasks according to their filters; pending archive
  intent shall not manufacture a confirmed archived task.

### REQ-TASKS-REMOVAL-NAVIGATION-004: Archive progress feedback

**Intent:** Give users persistent, truthful feedback when a user-initiated
archive request takes time to finish.

#### Acceptance criteria

- **AC-TASKS-REMOVAL-NAVIGATION-004.1:** When a user accepts an archive action
  from any desktop or phone task surface, the system shall show one localized
  loading toast for that operation or bulk batch in the existing bottom-right
  toast stack. The English source copy shall read `Archiving in progress`, the
  toast shall include a loading indicator, and the existing polite live region
  shall announce it.
- **AC-TASKS-REMOVAL-NAVIGATION-004.2:** The archive progress toast shall not
  auto-dismiss while any archive request in its operation or batch remains
  pending. After every request settles, the progress toast shall disappear and
  the existing success or failure feedback shall remain the only terminal
  notification.
- **AC-TASKS-REMOVAL-NAVIGATION-004.3:** Cancelling or dismissing archive
  confirmation shall show no progress toast and issue no request. Programmatic,
  API, CLI, MCP, and agent-driven archive operations shall remain unchanged.

## Compatibility and exclusions

Existing archive confirmation preferences and cascade choices remain governed
by [archive confirmation](archive-confirmation.md). Backend deletion admission,
cleanup, and discard consent remain governed by [runtime cleanup](runtime-cleanup.md).
This adds local user-action presentation guarantees; it does not change task APIs,
permissions, session-only deletion, Quick Chat expiration/close, or server cleanup.
Remote/API/MCP removal retains existing lifecycle reconciliation and redirects.
Cold unavailable task routes retain [their current contract](missing-task-route-recovery.md).
Undo, new settings, and a new mobile navigation composition are excluded.

## System design

- [Task removal navigation](../system-design/removal-navigation.md)

## Implementation plans

- [Task removal navigation](../../../plans/task-removal-navigation/plan.md)

- [Immediate sidebar archive](../../../plans/immediate-sidebar-archive/plan.md)
- [Archive progress feedback](../../../plans/archive-progress-feedback/plan.md)
