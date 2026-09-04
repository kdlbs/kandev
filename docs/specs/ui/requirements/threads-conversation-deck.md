---
status: draft
system: ui
created: 2026-08-28
owners:
  - kandev
---

# Threads Conversation Deck Requirements

## Overview

Threads gives a user one horizontally scrollable view of current task
conversations. A task can have several agent sessions, and its column can
switch among those existing sessions. Only an explicit pending action marks a
question or permission. The deck must show accurate attention states and stay
usable when a workspace has many active tasks.

The UI system owns the responsive interaction and presentation contract. The
platform owns the bounded status and session-stream delivery contract in
[Viewport-bounded Session Delivery](../../platform/requirements/viewport-bounded-session-delivery.md).

## Terminology

- **Task column:** The stable Threads shell for one task.
- **Selected session:** The existing agent session whose conversation is shown
  in a task column.
- **Attention action:** An explicit pending clarification or permission for a
  session.
- **Detail-active column:** A task column that can mount its selected session's
  full conversation under the platform delivery budget.

## Requirements

### REQ-UI-THREADS-DECK-001: Existing session switching

**Intent:** Let a user follow every existing agent session in a task without
turning Threads into a session-management surface.

**User story:** As a user with several agents on one task, I want to switch the
conversation in its task column, so that I can follow each agent without
leaving Threads.

#### Acceptance criteria

- **AC-UI-THREADS-DECK-001.1:** When a desktop task column has more than one
  session, the system shall show a switch-only session tab list on the right of
  the same header row as the task status, workflow, and step metadata.
- **AC-UI-THREADS-DECK-001.2:** The Threads session control shall not create,
  delete, rename, close, pin, reorder, or change the primary session, and it
  shall not show an add button or a context menu.
- **AC-UI-THREADS-DECK-001.3:** When the user selects a session, only that task
  column shall change its conversation. The selection shall not change another
  task column or the full task page's active session.
- **AC-UI-THREADS-DECK-001.4:** When a valid session stays selected, another
  session status change shall not change the selection.
- **AC-UI-THREADS-DECK-001.5:** When no valid user selection exists, the system
  shall select, in order, a session requested by the URL, a session with an
  explicit pending action, an active session, the primary session, or the
  newest remaining session.
- **AC-UI-THREADS-DECK-001.6:** When the selected session is removed, the system
  shall use the same deterministic fallback without changing the task column's
  position.
- **AC-UI-THREADS-DECK-001.7:** When a task-detail link opens Threads for a
  session that is still a member of the task, the URL shall identify both the
  task and session, the deck shall reveal the task column, and that session
  shall become selected.
- **AC-UI-THREADS-DECK-001.8:** Each selector item shall show the effective agent
  profile name. If profile data is not available, it shall show the custom
  session name or the existing fallback label.
- **AC-UI-THREADS-DECK-001.9:** A settled selector item shall show the agent
  icon. A `STARTING` or `RUNNING` item shall replace that icon with the grid
  spinner.

### REQ-UI-THREADS-DECK-002: Accurate attention state

**Intent:** Show when a person must act without presenting an ordinary
completed turn as an agent question.

#### Acceptance criteria

- **AC-UI-THREADS-DECK-002.1:** When the selected session has a pending
  clarification, the task column shall show a question indicator and a
  localized question label.
- **AC-UI-THREADS-DECK-002.2:** When the selected session has a pending
  permission, the task column shall show a permission indicator and a localized
  permission label.
- **AC-UI-THREADS-DECK-002.3:** When a session is `WAITING_FOR_INPUT` without an
  explicit pending action, the system shall not show a question or permission
  indicator.
- **AC-UI-THREADS-DECK-002.4:** When a task is in its review outcome and no
  session needs an explicit action, the task column shall show a completion
  indicator with the localized label `Ready for review`.
- **AC-UI-THREADS-DECK-002.5:** When a session is `STARTING` or `RUNNING`, its
  selector item shall show the grid spinner without loading its transcript.
- **AC-UI-THREADS-DECK-002.6:** When no valid selection exists and a session
  needs attention, the task column shall select that session without loading
  another transcript first.

### REQ-UI-THREADS-DECK-003: Responsive bounded deck

**Intent:** Keep Threads responsive with many task columns and preserve native
desktop and mobile navigation.

#### Acceptance criteria

- **AC-UI-THREADS-DECK-003.1:** When Threads contains 30 task columns, the deck
  shall preserve stable column order and horizontal scroll geometry while
  detail content is activated only for the current viewport under
  `REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-001`.
- **AC-UI-THREADS-DECK-003.2:** Before a task column becomes detail-active, the
  system shall show a lightweight shell from task summary data and shall not
  show another task's conversation in that shell.
- **AC-UI-THREADS-DECK-003.3:** When a desktop user scrolls a task column into
  view, the system shall activate its selected conversation without a page
  reload. When the column leaves the detail window, the shell and its local
  selection shall remain available.
- **AC-UI-THREADS-DECK-003.4:** On a phone, the system shall keep one snapped
  task column detail-active and shall use a compact session picker on the right
  of the metadata row instead of a nested horizontal tab strip.
- **AC-UI-THREADS-DECK-003.5:** The phone session picker shall open a bottom
  sheet with one row per existing session. Each row shall have a touch target
  of at least 44 by 44 CSS pixels. Each row shall show the same agent identity
  or grid spinner as the desktop tabs.
- **AC-UI-THREADS-DECK-003.6:** Long task metadata or many session names shall
  stay inside the task-column header. Desktop session tabs may scroll inside
  their right-side region, and the document shall not gain horizontal
  overflow.
- **AC-UI-THREADS-DECK-003.7:** Loading, empty-session, and recoverable
  session-list failure states shall remain inside their task column and shall
  not block horizontal navigation to other columns.

## Out of scope

- Creating, deleting, renaming, reordering, or changing the primary session
  from Threads.
- Persisting a Threads-only session selection across browser restarts.
- Copying transcripts or an unbounded session list into a task status summary.
- Replacing the full task workbench or its independent agent-tab behavior.
- Full DOM virtualization of lightweight task-column shells.

Task-column scope, filters, sort, limits, and saved view persistence are in
[Threads Saved Views](threads-saved-views.md).
