---
status: draft
system: tasks
created: 2026-09-03
owners:
  - Kandev
---

# Task Actions Menu Action Outcomes Requirements

## Overview

The actions menu added to the task preview and task detail surfaces by
[Task Actions Menu on Preview and Detail Surfaces](task-actions-menu.md) reaches
task actions that already exist. This file owns what each confirmed action does
to task state and to navigation, so that an action taken from one of these two
surfaces has the same effect as the same action taken from a Kanban card, and
leaves the user somewhere coherent rather than on a surface whose subject no
longer exists. Terminology and entry content are defined in the parent
requirement and are not restated here; in-flight, concurrent, and failing
actions are owned by
[Task Actions Menu In-Flight and Concurrency](task-actions-menu-concurrency.md).

## Requirements

### REQ-TASKS-TASK-ACTIONS-MENU-003: Action outcomes and post-action navigation

**Intent:** An action taken from one of these surfaces has the same effect on
task state as the same action taken from a card, and leaves the user somewhere
coherent rather than on a surface whose subject no longer exists.

#### Acceptance criteria

- **AC-TASKS-TASK-ACTIONS-MENU-003.1:** When the user selects Archive or Delete
  from either surface and a confirmation is shown for that action, the system
  shall open the same confirmation the card actions menu opens, carrying the
  subject task's title and the same cascade choice, and shall make no state
  change until the user confirms. Whether a confirmation is shown at all is not
  decided by this requirement; see AC-TASKS-TASK-ACTIONS-MENU-003.1a.
- **AC-TASKS-TASK-ACTIONS-MENU-003.1a:** Whether Archive shows a confirmation is
  owned by [Archive confirmation](archive-confirmation.md), not by this
  requirement. When the archive-confirmation preference is disabled,
  `AC-TASKS-ARCHIVE-CONFIRMATION-001.2` archives the requested task alone,
  without a dialog and without cascading, from any task surface. These two
  surfaces are task surfaces, so the system shall take that same path here and
  shall not show a confirmation the card would not show: this is parity under
  AC-TASKS-TASK-ACTIONS-MENU-002.1, not an exception to it. On that path the
  archive request is issued immediately with no cascade, and no confirm gesture
  exists, so every criterion worded around confirming an archive
  (AC-TASKS-TASK-ACTIONS-MENU-003.3, AC-TASKS-TASK-ACTIONS-MENU-003.4,
  AC-TASKS-TASK-ACTIONS-MENU-004.3 and AC-TASKS-TASK-ACTIONS-MENU-004.6) shall
  be read as taking effect when the request is issued. Where the preference has
  no stored value the system shall confirm, which is the existing default and
  not a new one. Delete is unaffected:
  `AC-TASKS-ARCHIVE-CONFIRMATION-001.7` leaves delete confirmations unchanged,
  so Delete always confirms.
- **AC-TASKS-TASK-ACTIONS-MENU-003.2:** When the user cancels or dismisses a
  confirmation opened from either surface, the system shall leave the task
  unchanged, shall leave the surface open on the same subject task, and shall
  make no request.
- **AC-TASKS-TASK-ACTIONS-MENU-003.3:** When the user confirms Archive or
  Delete from the preview surface and the request succeeds, the system shall
  remove the task from the board and close the preview panel, and shall not
  navigate to the task detail route.
- **AC-TASKS-TASK-ACTIONS-MENU-003.4:** When the user confirms Archive from the
  detail surface and the request succeeds, the system shall use the same
  archive-and-switch outcome the existing task-scoped archive entry points use:
  the next eligible task opens, or the task overview opens when no eligible
  task remains.
- **AC-TASKS-TASK-ACTIONS-MENU-003.5:** When the user confirms Delete from the
  detail surface and the request succeeds, the system shall remove the task and
  apply the same switch-to-next-task outcome as
  AC-TASKS-TASK-ACTIONS-MENU-003.4.
- **AC-TASKS-TASK-ACTIONS-MENU-003.6:** When the user selects a Move to or Send
  to workflow target from either surface, the system shall move the subject
  task alone to that step, shall keep the surface open on that task, and shall
  not navigate.
- **AC-TASKS-TASK-ACTIONS-MENU-003.7:** When a move made from the detail
  surface succeeds, the system shall show the new step as current in the top
  bar's workflow stepper.
- **AC-TASKS-TASK-ACTIONS-MENU-003.8:** When the user selects Edit from either
  surface, the system shall open the existing task edit dialog initialized from
  the subject task, and shall not navigate or change the active or previewed
  task; saving shall use the existing task update contract and cancelling shall
  leave the task unchanged.
- **AC-TASKS-TASK-ACTIONS-MENU-003.9:** When the user selects a Link entry from
  either surface, the system shall open the same link dialog the card actions
  menu opens for that provider, scoped to the subject task.
- **AC-TASKS-TASK-ACTIONS-MENU-003.10:** When the user confirms Detach from
  parent from either surface, the system shall detach the subject task from its
  parent and shall leave the surface open on that task.
- **AC-TASKS-TASK-ACTIONS-MENU-003.11:** The system shall introduce no API
  endpoint, request payload, persisted field, permission, or feature flag for
  any action reached from these surfaces; every action shall use the contract
  it already uses from the card actions menu.
