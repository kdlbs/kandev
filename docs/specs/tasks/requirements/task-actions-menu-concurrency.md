---
status: draft
system: tasks
created: 2026-09-03
owners:
  - Kandev
---

# Task Actions Menu In-Flight and Concurrency Requirements

## Overview

The actions menu added to the task preview and task detail surfaces by
[Task Actions Menu on Preview and Detail Surfaces](task-actions-menu.md) creates
two new entry points to task actions that already exist. This file owns what
those entry points do while a request is in flight, when two surfaces act on one
task, and when a request fails. Terminology, entry content, and post-action
navigation are defined in the parent requirement and are not restated here.

## Requirements

### REQ-TASKS-TASK-ACTIONS-MENU-004: In-flight, concurrent, and failing actions

**Intent:** The new entry points cannot produce a duplicate destructive request,
a stuck menu, or a silent failure.

#### Acceptance criteria

- **AC-TASKS-TASK-ACTIONS-MENU-004.1:** While an archive, delete, or detach
  request started from a surface's actions menu is in flight, the system shall
  present that surface's actions-menu entries as disabled and shall not start a
  second request from that menu. Because
  AC-TASKS-TASK-ACTIONS-MENU-004.2 closes the menu on a terminal activation,
  and archive, delete and detach are terminal, this state is observed by
  reopening the menu while the request is still in flight.
- **AC-TASKS-TASK-ACTIONS-MENU-004.1a:** Move to, Send to workflow, and Link are
  deliberately outside AC-TASKS-TASK-ACTIONS-MENU-004.1, matching the card,
  whose menu carries no pending state for them either. Selecting a Link entry
  opens a dialog and starts no task-state request, so the dialog owns its own
  submit state. For Move to and Send to workflow the system shall permit a
  second target to be selected before the first request settles, shall issue
  each as an independent request, and shall let the last one the server applies
  determine the task's step, on
  AC-TASKS-TASK-ACTIONS-MENU-004.3a's terms. The system shall not introduce a
  menu-level in-flight lock for these entries.
- **AC-TASKS-TASK-ACTIONS-MENU-004.1b:** The in-flight disabled state of
  AC-TASKS-TASK-ACTIONS-MENU-004.1 is scoped to the surface that started the
  request. A surface's menu shall disable its entries only for a request that
  surface itself started, and shall not disable them because a Kanban card or
  the other surface has a request in flight for the same task. The system shall
  not introduce shared, task-keyed pending state to close that gap. A card
  mid-archive and a preview panel showing the same task may therefore present
  different enabled states for the duration of that request. This is deliberate,
  it is the named exception in AC-TASKS-TASK-ACTIONS-MENU-002.1, and it matches
  the card, whose own pending flags are board-scoped or hook-local and are
  already not shared with any other surface.
- **AC-TASKS-TASK-ACTIONS-MENU-004.1c:** When the subject task's board row stops
  being resolvable while a Move to or Send to workflow submenu is open, the
  system shall demote those entries per AC-TASKS-TASK-ACTIONS-MENU-002.6, which
  removes the submenu's parent entry and so closes the open submenu while
  leaving the top-level menu open. A move request already in flight shall be
  left to complete or fail on its own terms; the system shall not cancel it, and
  shall keep the surface open on the subject task per
  AC-TASKS-TASK-ACTIONS-MENU-003.6. Losing the board row is not losing the task:
  AC-TASKS-TASK-ACTIONS-MENU-004.5 governs the latter.
- **AC-TASKS-TASK-ACTIONS-MENU-004.2:** When the user activates a *terminal*
  entry, meaning one that opens a dialog, dispatches a request, or invokes a
  plugin action, the system
  shall close the menu, so a single activation produces at most one confirmation
  and at most one request. Opening a submenu is not a terminal activation: when
  the user opens the Move to or Send to workflow submenu required by
  AC-TASKS-TASK-ACTIONS-MENU-002.2, the system shall leave both that submenu and
  the top-level menu open until a target inside it is chosen, and choosing that
  target is itself a terminal activation and closes both.
- **AC-TASKS-TASK-ACTIONS-MENU-004.3:** When the same task is acted on
  concurrently from two surfaces by a *one-shot* action, meaning Archive or
  Delete, the system shall send each confirmed request independently; the first
  request to be applied determines task state, and the later request shall
  surface the existing failure feedback for its action rather than a new error
  surface. Two actions are deliberately outside this criterion because neither
  is one-shot: Move to and Send to workflow are repeatable and are governed by
  AC-TASKS-TASK-ACTIONS-MENU-004.3a, and Detach from parent is idempotent and is
  governed by AC-TASKS-TASK-ACTIONS-MENU-004.3b.
- **AC-TASKS-TASK-ACTIONS-MENU-004.3a:** When the same task is moved
  concurrently, whether from two surfaces or twice from one menu, the system
  shall send each move independently and shall not treat the later move as a
  conflict. A move is repeatable and carries no precondition on the step it
  moves from, so the last move the server applies determines the task's step,
  and the system shall raise no failure feedback for an earlier move merely
  being superseded. Failure feedback for a move is raised only when the server
  rejects that move on its own terms
  (AC-TASKS-TASK-ACTIONS-MENU-004.4).
- **AC-TASKS-TASK-ACTIONS-MENU-004.3b:** Detach from parent is idempotent rather
  than one-shot, and the system shall not treat a concurrent or repeated detach
  of one task as a conflict. A task that is already parentless shall remain
  parentless and shall produce no failure feedback. Failure feedback for a
  detach is raised only when the detach operation fails, per
  AC-TASKS-TASK-ACTIONS-MENU-004.4.
- **AC-TASKS-TASK-ACTIONS-MENU-004.4:** When an archive, delete, move, detach,
  or link request started from either surface fails, the system shall surface
  the existing failure feedback for that action, shall leave the task in its
  last confirmed state, and shall keep the surface open on that task.
- **AC-TASKS-TASK-ACTIONS-MENU-004.4a:** AC-TASKS-TASK-ACTIONS-MENU-004.5 takes
  precedence over AC-TASKS-TASK-ACTIONS-MENU-004.4 whenever both apply. When a
  request fails because another actor archived or deleted the subject task,
  the surface shall apply its existing missing-task behavior instead of staying
  open on a task that is gone. The system shall still surface the existing
  failure feedback for the request. For every other failure, the surface shall
  stay open on the task and show that feedback.
- **AC-TASKS-TASK-ACTIONS-MENU-004.5:** When a task disappears from beneath a
  surface for a reason other than the user's own action, such as another client
  archiving it, the system shall apply that surface's existing missing-task
  behavior and shall not leave an actions menu open over a subject that no
  longer exists.
- **AC-TASKS-TASK-ACTIONS-MENU-004.5a:** An actions menu shall never act on a
  task other than the one it was opened on. There are three ways a menu can stop
  matching its subject, and this criterion owns the third. The board row
  becoming unresolvable is governed by AC-TASKS-TASK-ACTIONS-MENU-002.6 and
  AC-TASKS-TASK-ACTIONS-MENU-004.1c, which demote entries but keep the menu
  open. The subject being removed is governed by
  AC-TASKS-TASK-ACTIONS-MENU-004.5, which closes it. The subject being *replaced
  by a different task* is governed here: when the preview surface's subject task
  changes identity while its actions menu is open, which is reachable because
  the preview panel re-renders with a different task rather than unmounting, the
  system shall close that menu and shall not re-target it at the new subject.
  Reopening the trigger then opens a menu on the new subject, per
  AC-TASKS-TASK-ACTIONS-MENU-002.6. For this criterion the subject's identity is
  the subject task's *identifier*, compared across renders, and not the identity
  of the object the surface holds that task in. The system shall not treat a new
  task object bearing the same identifier as a change of subject: the preview
  rebuilds that object whenever the board's task collections change, so a
  field-only update to the subject, or an unrelated task's update anywhere on
  the board, shall leave an open menu open, which is what
  AC-TASKS-TASK-ACTIONS-MENU-002.6 requires of an in-place entry update. Only a
  change of the identifier itself qualifies. A subject replaced by *no* subject
  is not this case: the preview losing its subject entirely is governed by
  AC-TASKS-TASK-ACTIONS-MENU-004.5 when the task went away and by
  AC-TASKS-TASK-ACTIONS-MENU-001.6 for the trigger itself. The detail surface needs no equivalent rule
  because it remounts on navigation, which destroys any open menu; the criterion
  is stated for both surfaces so that a later change to that behavior does not
  silently create the gap.
- **AC-TASKS-TASK-ACTIONS-MENU-004.6:** Repeating an action that already
  succeeded, for example archiving a task another client has already archived
  (whether by confirming it, or by issuing it directly where
  AC-TASKS-TASK-ACTIONS-MENU-003.1a shows no confirmation), shall not corrupt
  client state: the system shall reconcile to the server's task state and
  surface the failure feedback for the rejected request. Where the server
  accepts the repeat instead of rejecting it, as a repeated move does under
  AC-TASKS-TASK-ACTIONS-MENU-004.3a and a repeated detach does under
  AC-TASKS-TASK-ACTIONS-MENU-004.3b, there is no rejection and the system shall
  surface no failure feedback.
