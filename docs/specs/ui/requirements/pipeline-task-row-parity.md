---
status: draft
system: ui
created: 2026-09-01
owners:
  - kandev
---

# Pipeline Task Row Parity Requirements

## Overview

Pipeline is the second task-listing view (Display -> view mode "pipeline",
persisted as `kandev.taskListing.view.v1`), rendering one task per row: a title
button, one pill per workflow step, an actions cluster.

The row is a step tracker that has not yet become a task surface. It shows
title, repository name, relative time and session count, omits nearly
everything else the Kanban card shows about a task's state, and offers archive
and delete only.

The reason is measurable. Measured 2026-09-01, "New Feature Dev" workflow, 9
visible steps, 1600px window: each step costs a 130px pill plus a 25px
connector, so nine steps consume 1370px of a 1646px row before the 200px title
and the actions cluster. **There is no room left to put anything in.**
Recovering that width is the enabling change. `SwimlaneGraph2Content` also
accepts and discards two `ViewContentProps` members Kanban honors, `onEditTask`
and `onSelectRange`.

The UI system owns this contract: presentation and interaction over task state
the Tasks system already exposes, with no task, workflow, or persistence
contract changing.

## Terminology

- **Board surface:** The width available to the task list after app chrome and
  any preview panel, per [Adaptive Kanban](adaptive-kanban.md), not viewport
  width.
- **Step run:** The ordered step markers between title and actions cluster.
- **Current step:** The step whose id equals the task's `workflowStepId`. A
  task whose `workflowStepId` is empty has none.
- **Collapsed marker:** Historical term from this contract's initial
  implementation. Superseded 2026-09-03: every step marker now renders its own
  labelled pill (past, current, and future alike); none collapse to an
  unlabelled marker.
- **Actions cluster:** The trailing group holding status affordances and the
  task menu trigger. Card 09404bae pins it to the trailing edge, which this
  contract assumes.
- **Disclosure surface:** The hover/focus popover on the row title.
- **Parity of information, not of layout:** Every fact the Kanban card shows is
  reachable from the row, but the row stays one line and moves what does not fit
  to the disclosure surface.


## Requirements

### REQ-UI-PIPELINE-ROW-001: The step run never grows the row

**Revision (2026-09-03):** This requirement originally collapsed every
non-current step to a 12px unlabelled marker so a nine-step run fit the board
with no scrolling. Design review after shipping found that illegible: a person
scanning the board could not tell which steps existed without hovering each
one. Every step now keeps its own labelled pill; the no-growth guarantee comes
from scrolling the step run instead, anchored so the current pill (not an
earlier one) stays in view by default. AC-UI-PIPELINE-ROW-001.1/.3/.8/.9 below
reflect this revision; as originally written they no longer apply.

**Intent:** A row that grows with the workflow's step count pushes the actions
cluster off the board surface. Constraining growth, not step legibility, is
what this requirement protects.

**User story:** As a person scanning a nine-step workflow, I want to see every
step's name at a glance, with the row never growing wider than the board.

#### Acceptance criteria

- **AC-UI-PIPELINE-ROW-001.1:** Every step marker in the run renders with a
  visible text label, including past and future steps, not only the current
  one.
- **AC-UI-PIPELINE-ROW-001.2:** Past and future step pills remain visually
  distinct from each other and from the current step's pill.
- **AC-UI-PIPELINE-ROW-001.3:** At a 1280px board surface with 9 workflow
  steps, the row element's own width never exceeds the task list's
  `clientWidth` and the actions cluster's right edge falls within the board
  surface, regardless of step count. The step run itself is permitted to
  scroll horizontally (AC-UI-PIPELINE-ROW-001.8) when it does not fit; the row
  sizes to the list, not to its own content, and carries no max-content minimum
  width, which is what puts the cluster off-screen today.
- **AC-UI-PIPELINE-ROW-001.4:** Every step's title is reachable from the row
  whenever the row's menus are available: inline on its own pill for a fine
  pointer, and without a pointer through the menu's move-to-step entries, which
  list every step and mark the current one. Multi-select suppresses both menus
  (AC-UI-PIPELINE-ROW-002.3, 002.8), suspending the pointer-free route until
  exited, matching the Kanban card.
- **AC-UI-PIPELINE-ROW-001.5:** Past and future step pills are not individually
  focusable, so a row's tab stop count does not grow with step count.
- **AC-UI-PIPELINE-ROW-001.6:** The row exposes its position as accessible text
  naming the current step title and its ordinal in the visible run.
- **AC-UI-PIPELINE-ROW-001.7:** The previous-step and next-step move controls
  continue to appear on hover or focus of the current step, each naming its
  destination step in a visible tooltip. At the first displayed step the
  previous-step control is absent, at the last the next-step control is; neither
  wraps, and their absence does not shift the run.
- **AC-UI-PIPELINE-ROW-001.8:** When the step run's full-pill width exceeds the
  space available to it, the run scrolls horizontally rather than growing the
  row or shrinking pills below their fixed size. The run is kept scrolled so
  the current step's pill stays within the visible lane by default; only past
  steps are permitted to scroll out of view, never the current or (space
  permitting) next steps.
- **AC-UI-PIPELINE-ROW-001.9:** Past and future step pills are not a click
  target for moving the task; movement stays on the move controls and
  move-to menu.

### REQ-UI-PIPELINE-ROW-002: The row offers the full task menu

**Intent:** The row offers Archive and Delete only, so the view cannot be a
primary board.

**User story:** As a person working in pipeline view, I want the same task
actions the Kanban card offers.

#### Acceptance criteria

- **AC-UI-PIPELINE-ROW-002.1:** The row's menu trigger opens a menu whose
  entries match the Kanban card's dropdown in identity, order, labels,
  enablement, and destructive styling for the same task and workspace.
- **AC-UI-PIPELINE-ROW-002.2:** The menu offers edit, move to step, send to
  workflow, link (pull request, issue, merge request, Jira, Linear, Sentry),
  plugin actions, archive, detach from parent, and delete, each under the Kanban
  card's availability conditions.
- **AC-UI-PIPELINE-ROW-002.3:** Right-clicking anywhere on the row that is not
  an interactive control opens a context menu with the same entries, on desktop
  pointers only. Multi-select suppresses it, matching the Kanban card.
- **AC-UI-PIPELINE-ROW-002.4:** Menu entries for both surfaces are produced by
  a single shared source. A change to an entry's label, order, icon or
  enablement made in one place appears in both the Kanban card and the pipeline
  row without a second edit.
- **AC-UI-PIPELINE-ROW-002.5:** Confirmation and link dialogs opened from the
  row behave as from the Kanban card, including the archive and detach
  confirmations and focus return.
- **AC-UI-PIPELINE-ROW-002.6:** The Kanban card's rendered menu output and its
  rendered inline status output are unchanged by this work, except for the
  disclosure change AC-UI-PIPELINE-ROW-004.4 requires. "Unchanged" is judged
  against the card at this contract's base commit, over the fixture matrix and
  comparison fields the system design's Observability section names.
- **AC-UI-PIPELINE-ROW-002.7:** Selecting a menu entry does not also trigger
  the row's click, preview, navigation, or selection behavior.
- **AC-UI-PIPELINE-ROW-002.8:** In multi-select mode the actions cluster is
  absent, matching the Kanban card; its absence does not shift the step run.

### REQ-UI-PIPELINE-ROW-003: Inline status parity

**Intent:** A row showing a blocked task with no blocked indication reads as
progressing. These are the facts a person scans a board for, so they belong on
the row, not behind a hover.

**User story:** As a person scanning many rows, I want change-request and
blocking state visible without hovering.

#### Acceptance criteria

- **AC-UI-PIPELINE-ROW-003.1:** The row shows the pull request, merge request,
  and registered change-request status indicators, each rendering nothing
  without such an association.
- **AC-UI-PIPELINE-ROW-003.2:** The row shows the blocked indicator when the
  task is blocked, distinguishing a failed predecessor from merely pending
  ones.
- **AC-UI-PIPELINE-ROW-003.3:** The row shows the queued-for-step indicator
  when the task is queued for a step.
- **AC-UI-PIPELINE-ROW-003.4:** The row shows the approval-required and
  changes-requested review states under the Kanban card's conditions.
- **AC-UI-PIPELINE-ROW-003.5:** The row shows the active subagent count when it
  exceeds zero, and the remote-cloud executor indicator when the task runs on
  one.
- **AC-UI-PIPELINE-ROW-003.6:** The row renders the `task-card-indicators` and
  `task-card-tags` plugin slots with the slot props the Kanban card supplies,
  and nothing extra when a slot has no contribution.
- **AC-UI-PIPELINE-ROW-003.7:** The row shows repository chips with full paths
  on hover, using the Kanban card's resolution, ordering, visible count, and
  overflow behavior.
- **AC-UI-PIPELINE-ROW-003.8:** Inline status elements render in one fixed
  left-to-right order for every row: repository chips, title, pull request,
  merge request, registered change request, plugin indicators, blocked, queued
  for step, session count, review state, subagent count, remote executor, the
  step run, then the actions cluster. An element that has nothing to show
  occupies no width. Session count is the Kanban card's own badge, under the
  card's condition of more than one session; it replaces the row's present
  render at more than zero, so the row shows one session count, not two.
  Relative updated time renders between the remote executor and the step run and
  is the one member with no Kanban card counterpart: the card neither renders it
  nor gains it, so the shared strip carries it as an optional member the card
  omits. The row's needs-attention edge treatment is not a member of this order;
  it is preserved by the restructure rather than lost with the button that
  carries it today.
- **AC-UI-PIPELINE-ROW-003.9:** The row remains a single line of constant
  height regardless of how many status elements are present.

- **AC-UI-PIPELINE-ROW-003.10:** No status element is nested inside another
  element's activation target, so the row contains no interactive control inside
  another interactive control.
- **AC-UI-PIPELINE-ROW-003.11:** When the AC-UI-PIPELINE-ROW-003.8 members
  other than the step run exceed the available width, the row yields width in a
  fixed order: first the title truncates to a 96px floor, then the step run
  scrolls per AC-UI-PIPELINE-ROW-001.8, then, both exhausted, the row's content
  between title and actions cluster scrolls horizontally while that cluster
  stays pinned and reachable. That scroll is the terminus: the row never wraps,
  clips an indicator, or drops a plugin contribution. A row carries at most one
  horizontally scrolling region at a time: at the second stage it spans the step
  run alone, and at the terminus the same region widens to span the inline
  status strip and the step run together. Two such regions are never nested one
  inside the other, nor placed side by side, on one row, which is what keeps the
  two stages distinguishable. The region reserves no scrollbar height, so row
  height is identical at every stage, including on platforms rendering
  non-overlay scrollbars. Repository chips are not a yield stage; they render at
  the Kanban card's fixed visible count with overflow at every width
  (AC-UI-PIPELINE-ROW-003.7).

### REQ-UI-PIPELINE-ROW-004: Disclosure parity on hover

**Intent:** The row truncates its title at 200px with no tooltip and no `title`
attribute, so a truncated title is unrecoverable.

**User story:** As a person whose task titles are longer than the column, I
want to read the whole title and its context without leaving the board.

#### Acceptance criteria

- **AC-UI-PIPELINE-ROW-004.1:** Hovering or keyboard-focusing the row title
  opens a disclosure surface showing the task's full untruncated title.
- **AC-UI-PIPELINE-ROW-004.2:** The disclosure surface shows the task
  description when it has one, the parent-task relationship when the task is a
  subtask, and its subtasks when it has any.
- **AC-UI-PIPELINE-ROW-004.3:** The disclosure surface opens when the task has a
  description, a parent, or at least one subtask, or when its title is visually
  truncated at its rendered width, measured on that surface's own rendered title
  element, and not otherwise. A bare task with a short title gains no popover on
  either surface.
- **AC-UI-PIPELINE-ROW-004.4:** The Kanban card uses the same disclosure surface
  and gains the same content, so the two views do not diverge.
- **AC-UI-PIPELINE-ROW-004.5:** Opening the disclosure surface does not
  preview, navigate, select, or move the task.

### REQ-UI-PIPELINE-ROW-005: Deterministic and resilient row behavior

**Intent:** Row order, in-flight actions, and out-of-band updates are
unspecified, leaving each to be invented during implementation.

**User story:** As a person watching a live board, I want rows to hold a stable
order and survive concurrent updates.

#### Acceptance criteria

- **AC-UI-PIPELINE-ROW-005.1:** Rows are ordered by the index of the task's step
  within the displayed steps ascending, then by the task's `position` ascending
  treating an absent position as 0, then by the task's `id` ascending. A task
  with no resolvable current step (AC-UI-PIPELINE-ROW-005.6) sorts after every
  task that has one, not before, ordering within that group by the two remaining
  keys. The order is identical for identical inputs regardless of arrival
  order.
- **AC-UI-PIPELINE-ROW-005.2:** While a step move for a task is in flight,
  further move requests from the same row are ignored rather than queued, and
  that row's move controls and move-to entries are disabled. The guard is
  row-local: a move from another client or mounted view is a cross-client
  conflict owned by AC-UI-PIPELINE-ROW-005.3 and AC-UI-PIPELINE-ROW-005.4.
- **AC-UI-PIPELINE-ROW-005.3:** When a move fails, the row returns to the step
  the server reports and the existing move-error surface reports the failure.
  The row always renders the task's step as the store holds it, so an
  out-of-band change arriving mid-flight is not reverted when the move settles.
  The in-flight guard is presentational: it releases on every terminal outcome,
  success, failure, timeout, or unmount, and once released the same move may be
  requested again.
- **AC-UI-PIPELINE-ROW-005.4:** When the task's step changes out of band while
  its menu or a confirmation dialog is open, the row re-renders at the new step
  and that surface stays open.
- **AC-UI-PIPELINE-ROW-005.5:** When the task leaves the list while a menu or
  dialog is open, the row and its surfaces unmount without an error.
- **AC-UI-PIPELINE-ROW-005.6:** When the displayed step list is non-empty and
  the task has no resolvable current step, its `workflowStepId` being empty and
  so matching no displayed step, the row renders every displayed marker in its
  not-yet-reached form plus exactly one labelled marker before them, carrying
  fixed unassigned-step copy rather than a step title. A connector renders
  between that marker and the first displayed step, in the not-yet-reached form.
  The row never renders zero labelled markers while any step is displayed. No
  step title is resolved here, and that marker carries no move controls, there
  being no current step for either; the AC-UI-PIPELINE-ROW-001.6 accessible
  summary names the unassigned state in place of a step title and ordinal, and
  the move-to-step menu entries remain the route to a step.
- **AC-UI-PIPELINE-ROW-005.7:** When the displayed step list is empty, the row
  renders its title and actions cluster with no step run and does not error.
  AC-UI-PIPELINE-ROW-005.6 does not apply to an empty displayed step list.
- **AC-UI-PIPELINE-ROW-005.8:** A task with no linked repository, description,
  parent, dependencies, or change-request association renders a row with no
  empty chip, badge, separator, or reserved gap.
- **AC-UI-PIPELINE-ROW-005.9:** Values scoped to the workspace rather than to a
  task, namely external-link availability and the workspace repository list,
  produce the same result on every row of one list for one workspace state: no
  two rows rendered together disagree about available link entries or existing
  repositories.
- **AC-UI-PIPELINE-ROW-005.10:** Shift-clicking a row in multi-select mode
  selects the contiguous range between the last selected and the clicked row in
  the AC-UI-PIPELINE-ROW-005.1 order.

## Out of scope

- **Card 09404bae's two defects.** The 3-dots trigger's right edge measured
  x=1982 against a 1600px viewport, so the view's only menu is off-screen, and
  `onPreviewTask` is threaded into `Graph2StepNode` and never read. Both, with
  pinning the cluster and the resulting click decision, belong to **card
  09404bae**, which lands first. This contract specifies what goes in the
  cluster, not where it sits; the design's "Dependency on card 09404bae" absorbs
  each branch.
- **Redesigning the Kanban card.** Kanban's rendered output changes only where
  a criterion says so, at AC-UI-PIPELINE-ROW-004.4.
- **Mobile and touch pipeline.** `getEffectiveTaskListingView` forces pipeline
  back to kanban on mobile, so pipeline is a desktop surface inheriting the
  existing non-desktop no-ops for context menu and disclosure surface.
- **Right-to-left layouts.** The five shipped locales are all left-to-right.
- **Drag and drop between steps.** Movement stays on the chevron controls and
  the move-to menu.
- **The List view.** Pipeline only.
- **Empty or whitespace-only task titles.** Non-empty by the Tasks contract, so
  no untitled placeholder is specified.
- **A user-configurable row density.** Rejected: it doubles the layout surface
  to test and no measured defect needs it.
- **Labelling the steps adjacent to the current step.** Rejected: at roughly
  96px each it costs about 192px of the width this work exists to recover, and
  AC-UI-PIPELINE-ROW-001.7 puts both neighbor names one hover away.
- **Backend, task, workflow, WIP-limit, preview contracts.** Unchanged.
- **Adding members to `ViewContentProps`.** Values the row needs that the view
  contract does not carry, the workspace id, the workspace repository list, and
  external-link availability, are read from the store as the plugin slots do. This bars new members; it does not bar correcting an
  existing member's declared type to match its real call site, a type-level
  change with no runtime effect.
- **Naming a hidden step on the row.** An earlier revision had
  AC-UI-PIPELINE-ROW-005.6 render a hidden current step's own title, needing the
  task's pre-remap `workflowStepId` and the unfiltered step set. No task can
  reach a row on a hidden step (design, "Failure and recovery"), so that clause
  and its data-layer work are cut; letting hidden steps hold tasks would mean
  reinstating both.

## Scenarios

Each criterion is directly testable, so this section carries only flows
spanning more than one.

- **GIVEN** a 1280px board surface and a 9-step workflow, **WHEN** a task at
  step 4 renders, **THEN** all nine steps render as labelled pills, the step
  run scrolls internally to keep step 4's pill in view, and the row itself
  never overflows the list.
- **GIVEN** a title longer than the row's title width, **WHEN** the person
  hovers or focuses it, **THEN** the disclosure surface shows the full title
  plus description, parent, and subtasks where present, and the task is neither
  previewed nor navigated to; **GIVEN** instead a short-titled task with no
  description, parent, or subtasks, **THEN** no surface opens.
