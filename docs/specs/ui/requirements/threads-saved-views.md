---
status: draft
system: ui
created: 2026-08-31
owners:
  - kandev
---

# Threads Saved Views Requirements

## Overview

Threads can contain many live task conversations. Users need direct control of
which task columns appear, their order, and the maximum number of columns.

Threads saved views provide this control without changing sidebar task views.
The UI system owns the interaction and query presentation. The task system
continues to own task and workflow state.

## Terminology

- **Threads view:** A named user setting that contains a task scope, filters,
  sort rules, and an optional column limit.
- **Task scope:** Either all eligible Threads tasks or an explicit set of task
  IDs.
- **Eligible task:** A non-archived task in the active workspace that satisfies
  the existing Threads deck eligibility contract and has a primary session.
- **Admitted task:** An eligible task that matches the active view and is inside
  its current column limit.
- **View draft:** Unsaved changes to the active Threads view.

## Requirements

### REQ-UI-THREADS-SAVED-VIEWS-001: Saved view lifecycle

**Intent:** Let each user keep several named Threads arrangements and switch
between them without rebuilding filters.

**User story:** As a user who monitors different groups of tasks, I want saved
Threads views, so that I can change focus with one action.

#### Acceptance criteria

- **AC-UI-THREADS-SAVED-VIEWS-001.1:** On desktop and tablet, the Threads top
  bar shall show the active Threads view selector directly before the
  task-listing view controls.
- **AC-UI-THREADS-SAVED-VIEWS-001.2:** When no Threads view is stored, the
  system shall provide and select one canonical `All threads` view.
- **AC-UI-THREADS-SAVED-VIEWS-001.3:** `All threads` shall use all eligible
  tasks, the existing attention-first order, and a default limit of five
  columns.
- **AC-UI-THREADS-SAVED-VIEWS-001.4:** A user shall be able to create, rename,
  delete, switch, overwrite, duplicate with `Save as`, and discard changes to
  Threads views.
- **AC-UI-THREADS-SAVED-VIEWS-001.5:** A user shall be able to save at most 50
  Threads views. The UI shall explain the limit before another create action.
- **AC-UI-THREADS-SAVED-VIEWS-001.6:** Saved views, the active view, and the
  active draft shall survive reloads and backend restarts through backend user
  settings.
- **AC-UI-THREADS-SAVED-VIEWS-001.7:** A successful settings update shall make
  the same Threads views available to another client for that user.
- **AC-UI-THREADS-SAVED-VIEWS-001.8:** Threads views and sidebar views shall
  have independent names, active selections, drafts, and saved definitions.
- **AC-UI-THREADS-SAVED-VIEWS-001.9:** Switching a Threads view shall not change
  the active sidebar view, Kanban workflow selection, or task-listing view.

### REQ-UI-THREADS-SAVED-VIEWS-002: Task scope and filters

**Intent:** Let the user select exact tasks or define a live task set from
bounded task data.

#### Acceptance criteria

- **AC-UI-THREADS-SAVED-VIEWS-002.1:** A Threads view shall support `all` and
  `selected` task scopes. The `selected` scope shall accept one or more task
  IDs from the active workspace.
- **AC-UI-THREADS-SAVED-VIEWS-002.2:** The task picker shall support individual
  checkboxes, Select all, Clear all, an indeterminate Select all state, and
  task-title search.
- **AC-UI-THREADS-SAVED-VIEWS-002.3:** The `all` scope shall include every
  eligible task that satisfies all active filter clauses.
- **AC-UI-THREADS-SAVED-VIEWS-002.4:** The `selected` scope shall include only
  stored task IDs that are currently eligible and satisfy all active filter
  clauses.
- **AC-UI-THREADS-SAVED-VIEWS-002.5:** Multiple filter clauses shall use AND
  semantics. Multiple selected values in one clause shall use OR semantics.
- **AC-UI-THREADS-SAVED-VIEWS-002.6:** Filters shall support thread status,
  pending action, exact task state, workflow, workflow step, and repository.
- **AC-UI-THREADS-SAVED-VIEWS-002.7:** Filters shall support primary agent,
  executor type, priority, blocked state, queued prompts, and active subagents.
- **AC-UI-THREADS-SAVED-VIEWS-002.8:** Filters shall support Git changes, pull
  request presence, pull request attention, task type, and title text.
- **AC-UI-THREADS-SAVED-VIEWS-002.9:** Thread status shall distinguish needs
  action, running, waiting, and ready for review without loading transcripts.
- **AC-UI-THREADS-SAVED-VIEWS-002.10:** A stored selected task ID shall remain
  in the view when its task becomes ineligible or unavailable. The task shall
  reappear if it becomes eligible again.
- **AC-UI-THREADS-SAVED-VIEWS-002.11:** The active Kanban workflow and
  repository display filters shall not add hidden constraints to a Threads
  view.
- **AC-UI-THREADS-SAVED-VIEWS-002.12:** Clear all can create an empty draft.
  The system shall disable Save until the user selects at least one task or
  changes the task scope to `all`.
- **AC-UI-THREADS-SAVED-VIEWS-002.13:** Filters shall support active errors,
  task labels, task origin, and tasks that have more than one agent session.

### REQ-UI-THREADS-SAVED-VIEWS-003: Sort and column limit

**Intent:** Give the user a predictable order and a hard bound on simultaneous
task columns.

#### Acceptance criteria

- **AC-UI-THREADS-SAVED-VIEWS-003.1:** A Threads view shall support sort by
  attention, last activity, updated time, created time, title, task state,
  workflow, priority, and primary agent.
- **AC-UI-THREADS-SAVED-VIEWS-003.2:** Every sort shall use task ID as its final
  deterministic tie-breaker.
- **AC-UI-THREADS-SAVED-VIEWS-003.3:** A Threads view shall support no user
  column limit or an integer limit from 1 through 30.
- **AC-UI-THREADS-SAVED-VIEWS-003.4:** The system shall apply task scope,
  filters, sort, and then the column limit in that order.
- **AC-UI-THREADS-SAVED-VIEWS-003.5:** When the limit is 3, the board shall
  mount at most three task-column shells for that view.
- **AC-UI-THREADS-SAVED-VIEWS-003.6:** The view control shall show the admitted
  count and the number of additional matching tasks when a limit hides tasks.
- **AC-UI-THREADS-SAVED-VIEWS-003.7:** Switching a view or changing its query
  shall apply its current sort and reset the stable column order.
- **AC-UI-THREADS-SAVED-VIEWS-003.8:** While the query stays unchanged,
  surviving columns shall keep their positions. Removed tasks shall leave, and
  newly admitted tasks shall fill available positions in sorted order.
- **AC-UI-THREADS-SAVED-VIEWS-003.9:** The user shall be able to reapply the
  active sort to the current matching set without changing the saved view.
- **AC-UI-THREADS-SAVED-VIEWS-003.10:** A valid task deep link shall temporarily
  admit and focus its task when the active view or column limit hides it.
- **AC-UI-THREADS-SAVED-VIEWS-003.11:** A temporary deep-link admission shall
  count toward the column limit and shall not modify the saved task scope.
- **AC-UI-THREADS-SAVED-VIEWS-003.12:** Each sort option shall include a
  visible description that explains the order that the option applies.
- **AC-UI-THREADS-SAVED-VIEWS-003.13:** A new Threads view shall start with a
  limit of five columns. The user can select another valid limit or no limit.

### REQ-UI-THREADS-SAVED-VIEWS-004: Responsive editor and recovery

**Intent:** Keep saved Threads views usable with a pointer, keyboard, or phone
touch screen.

#### Acceptance criteria

- **AC-UI-THREADS-SAVED-VIEWS-004.1:** On desktop, the view selector and editor
  shall use compact top-bar controls and a viewport-contained popover.
- **AC-UI-THREADS-SAVED-VIEWS-004.2:** On tablet and phone, a visible Threads
  view control shall open one inset bottom drawer with saved views and editor
  navigation.
- **AC-UI-THREADS-SAVED-VIEWS-004.3:** The mobile drawer shall provide the same
  task scope, filters, sort, limit, save, and view-switch outcomes as desktop.
- **AC-UI-THREADS-SAVED-VIEWS-004.4:** The task picker shall replace the editor
  body inside the same surface. It shall not open a nested drawer.
- **AC-UI-THREADS-SAVED-VIEWS-004.5:** Mobile rows and standalone actions shall
  have touch targets of at least 44 by 44 CSS pixels.
- **AC-UI-THREADS-SAVED-VIEWS-004.6:** The drawer shall own vertical scrolling,
  clear the bottom safe area, and cause no document-level horizontal overflow.
- **AC-UI-THREADS-SAVED-VIEWS-004.7:** Keyboard users shall be able to switch
  views, edit checkboxes and clauses, save changes, dismiss the surface, and
  return focus to its trigger.
- **AC-UI-THREADS-SAVED-VIEWS-004.8:** An empty result shall keep the top-bar
  controls available and show a local empty state with the active view name.
- **AC-UI-THREADS-SAVED-VIEWS-004.9:** If a settings write fails, the system
  shall restore the last backend value and show a recoverable sync error.
- **AC-UI-THREADS-SAVED-VIEWS-004.10:** If stored view data is invalid or
  references removed tasks, the page shall remain usable and shall fall back
  to the canonical view only when no valid active view remains.
- **AC-UI-THREADS-SAVED-VIEWS-004.11:** Each task-picker row shall show the
  current task-state icon and workflow-step label from the live task summary.
- **AC-UI-THREADS-SAVED-VIEWS-004.12:** A task-picker row with a pull request
  shall show the shared status color and pointer or touch disclosure.
- **AC-UI-THREADS-SAVED-VIEWS-004.13:** The desktop editor popover shall use a
  visible border and elevation that separate it from the page background.

## Out of scope

- Sharing one saved-view collection between Threads and the task sidebar.
- Grouping horizontal columns under section headings.
- Showing archived tasks or tasks without a primary agent session.
- Filtering by transcript text, tool output, files, or other session detail.
- Sharing a Threads view with another user or adding role-based view ownership.
- Adding a server-side task-query endpoint for this feature.
- Changing viewport-bounded session-list and transcript subscription limits.
