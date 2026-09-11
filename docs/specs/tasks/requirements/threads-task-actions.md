---
status: active
system: tasks
created: 2026-09-10
owners:
  - kandev
---

# Threads Task Actions Requirements

## Overview

Users can triage the task behind a Threads conversation without leaving the
deck. The task system owns the target identity, eligible operations, persisted
results, and recovery behavior. The [Threads deck](../../ui/requirements/threads-conversation-deck.md)
and [saved views](../../ui/requirements/threads-saved-views.md) continue to own
conversation selection, viewport activation, admission, and stable ordering.

This capability adds an entry point to existing task operations. The compact
phone topbar, inline thread pagination, title picker, and native swiper remain
the parent feature's composition.

## Terminology

- **Menu target:** The task whose header or overflow control opened the action
  flow, including its workspace identity.
- **Action flow:** The menu, nested choices, any subsequent link or confirmation
  surface, and the operation initiated from that surface.
- **Current thread:** The thread the reader is following in the deck. It can
  differ from the menu target and from the task workbench's remembered selection.
- **Admitted order:** The visible task order after applying the existing Threads
  query and stable-order rules.

## Requirements

### REQ-TASKS-THREADS-ACTIONS-001: Existing task operations

**Intent:** Expose the task actions users already know with the same eligibility
and consequences on desktop and phone.

#### Acceptance criteria

- **AC-TASKS-THREADS-ACTIONS-001.1:** For an eligible task, the menu shall present
  Priority, Move to, Send to workflow, Link, Archive, and Delete in that order.
  Delete shall follow a separator and use destructive styling. Actions that are
  unavailable under existing rules shall retain their existing hidden or
  disabled treatment.
- **AC-TASKS-THREADS-ACTIONS-001.2:** Priority shall offer the existing four
  localized choices and current-value marker, following the
  [task priority contract](task-priority-visibility.md).
- **AC-TASKS-THREADS-ACTIONS-001.3:** Move to shall offer the target task's
  workflow steps. Send to workflow shall offer eligible other workflows and
  their steps. Current-step disabling, hidden workflows, no-step explanations,
  auto-start markers, and server transition or queue rules shall match the
  existing task menus.
- **AC-TASKS-THREADS-ACTIONS-001.4:** Link shall expose the currently supported
  choices for the target's workspace and repositories, including available
  plugin link actions. It shall open the existing provider flow with its
  validation, linking result, and failure behavior.
- **AC-TASKS-THREADS-ACTIONS-001.5:** Archive shall follow the existing
  [confirmation preference](archive-confirmation.md), cleanup summary,
  in-flight warning, descendant classification, and cascade consent. Cancelling
  shall perform no archive. Disabling confirmation shall not imply cascade.
- **AC-TASKS-THREADS-ACTIONS-001.6:** Delete shall require the existing task
  deletion confirmation, including applicable cleanup, cascade, and worktree
  discard consent. Cancelling shall leave the task and its sessions unchanged.

### REQ-TASKS-THREADS-ACTIONS-002: Target identity and mutation results

**Intent:** Every operation affects the task the user chose, even as the deck
and task data change during the flow.

#### Acceptance criteria

- **AC-TASKS-THREADS-ACTIONS-002.1:** Opening or dismissing a task menu shall not
  select another task, switch a session, navigate away from Threads, or alter
  another surface's multi-selection.
- **AC-TASKS-THREADS-ACTIONS-002.2:** If the visible thread, selected sibling
  session, task title, or global task selection changes during an action flow,
  every subsequent choice and confirmation shall retain the original task and
  workspace identity. A task action shall never turn into a session action or
  an implicit bulk operation.
- **AC-TASKS-THREADS-ACTIONS-002.3:** Eligibility and displayed current values
  shall follow current data for that same target. A removed or archived target,
  lost access, or a changed workspace shall not enable a stale action or retarget
  it. A target merely filtered out of the deck can finish an already-open flow
  while it remains accessible and eligible.
- **AC-TASKS-THREADS-ACTIONS-002.4:** A successful operation shall converge in
  Threads and existing shared task surfaces and remain correct after reload.
  Workflow changes shall retain their existing agent-start and capacity effects.
- **AC-TASKS-THREADS-ACTIONS-002.5:** A failed operation shall expose existing
  localized error feedback, retain or restore the last confirmed task state,
  and permit the existing retry path. A late result shall not overwrite a newer
  menu target or pull the reader away from a newer selection.
- **AC-TASKS-THREADS-ACTIONS-002.6:** Loading and pending operations shall retain
  existing disabled rules and prevent repeated confirmation from submitting the
  same pending operation twice. Unrelated threads shall remain usable after the
  temporary surface closes.

### REQ-TASKS-THREADS-ACTIONS-003: Deterministic deck recovery

**Intent:** Task changes and view changes leave the reader at a valid thread
without losing the deck's position and session behavior.

#### Acceptance criteria

- **AC-TASKS-THREADS-ACTIONS-003.1:** When task data or view filters change, an
  admitted current thread shall remain current. Changing or removing another
  task shall not move that surviving thread or reset its selected session.
- **AC-TASKS-THREADS-ACTIONS-003.2:** When the current thread leaves the admitted
  set, the replacement shall be the first surviving successor in its previous
  admitted order, otherwise the nearest surviving predecessor, otherwise the
  first task in the new admitted order. Simultaneously removed descendants
  shall not become replacement candidates.
- **AC-TASKS-THREADS-ACTIONS-003.3:** With no admitted tasks after loading
  completes, Threads shall show its existing empty state. During initial
  loading, it shall retain the existing loading state. Pagination, focus, and
  conversation activation shall not reference a removed column.
- **AC-TASKS-THREADS-ACTIONS-003.4:** A failure shall not permanently remove a
  task from the deck or advance selection as if the operation succeeded.
  Server events arriving before or after the response shall produce the same
  final result without repeated jumps.
- **AC-TASKS-THREADS-ACTIONS-003.5:** Recovery shall keep the Threads route,
  workspace, view filters, and saved preferences. Obsolete task/session deep-link
  parameters may be removed when their target is no longer admissible, without
  changing valid links or the existing temporary-admission policy.
- **AC-TASKS-THREADS-ACTIONS-003.6:** Recovery and menu use shall preserve native
  horizontal swiping, stable task order, conversation drafts, session switching,
  and the existing viewport-based transcript and subscription budget.

### REQ-TASKS-THREADS-ACTIONS-004: Responsive accessible interaction

**Intent:** Task actions remain discoverable, contained, and usable by pointer,
  keyboard, and touch without taking space from the conversation.

#### Acceptance criteria

- **AC-TASKS-THREADS-ACTIONS-004.1:** Fine-pointer desktop users shall have a
  context menu on the relevant task header and a visible keyboard-operable
  overflow button. Chat text, selected transcript text, editors, and session
  controls shall retain their own context-menu behavior.
- **AC-TASKS-THREADS-ACTIONS-004.2:** Phones shall retain the compact page
  topbar and inline thread swiper. A visible task-actions control shall occupy
  the existing task header without adding a header row. Required actions shall
  not depend on right-click, hover, or long press.
- **AC-TASKS-THREADS-ACTIONS-004.3:** Phone choices shall use one inset bottom
  drawer with nested pages and a visible Back control. Entering a nested choice
  shall replace its parent content. A link or full confirmation surface shall
  replace the menu surface without concurrent interactive overlay stacks.
- **AC-TASKS-THREADS-ACTIONS-004.4:** Phone and coarse-pointer action buttons
  shall have at least 44-by-44 CSS-pixel hit areas, and menu rows shall be at
  least 44 CSS pixels high. Fine-pointer desktop controls shall retain the
  surrounding compact density, including at responsive transitions.
- **AC-TASKS-THREADS-ACTIONS-004.5:** Menus, nested choices, and confirmations
  shall remain within the visible viewport with safe-area clearance. Long lists
  shall have one internal vertical scroll region per active surface; long task,
  workflow, step, and provider labels shall not cause document horizontal
  overflow or obscure actionable rows.
- **AC-TASKS-THREADS-ACTIONS-004.6:** Controls shall expose localized accessible
  names and current/open states. Escape and Back shall dismiss the current
  choice predictably; outside dismissal shall close the menu without mutation.
  Focus shall return to the surviving opener, otherwise the current surviving
  thread's overflow, otherwise the Threads empty-state container. Focus return
  shall not scroll back to an offscreen thread or steal focus from a newer
  user interaction.
- **AC-TASKS-THREADS-ACTIONS-004.7:** Labels, errors, and announcements shall
  reuse existing localized copy. Any new copy shall be supplied for all
  supported locales, including the generated Traditional Chinese and pseudo
  catalogs, without translating persisted identifiers.

## Out of scope

- Choosing Threads as the default Home view or changing saved-view semantics.
- Reimplementing the parent's compact shared header, thread picker, pagination,
  or gesture recognition.
- New task operations, linking providers, bulk actions, or session management.
- Backend APIs, persistence, permissions, workflow policy, feature flags, or
  plugin SDK changes.

## Design and delivery

- [System design](../system-design/threads-task-actions.md)
- [Implementation plan](../../../plans/threads-task-actions/plan.md)
