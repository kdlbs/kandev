---
status: draft
system: ui
created: 2026-09-03
owners:
  - nova28
---

# Kanban Preview Workflow Step Navigation Requirements

## Overview

The kanban preview panel opens beside the board and shows one task's sessions.
Its header shows the task title and the panel controls, but not the task's
workflow step, and it offers no way to move the task.

Dragging a card to another column is the only in-board way to change a task's
step, and it is not always available: the per-workflow column visibility
filter
([REQ-UI-BOARD-STEP-VISIBILITY-FILTER-001](board-step-visibility-filter.md))
can hide the target step, and the preview narrows the board so columns can sit
off-screen.

The task top bar already solves this in a narrow space
([REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001](compact-workflow-step-navigation.md))
with a compact current-step indicator that discloses every step with a move
control for each eligible target. This requirement puts that indicator in the
preview header, constrains its footprint, and adds a copy-task-link control.

The UI system owns this presentation and its containment. The task system
continues to own workflow order, move eligibility, and task transitions.

## Terminology

- **Preview panel:** The task panel that opens beside the kanban board, in
  either its inline or its floating layout.
- **Preview header:** The single row at the top of the preview panel holding
  the task title and the panel controls.
- **Step indicator:** The compact current-step marker, step name, and position
  count defined by
  [REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001](compact-workflow-step-navigation.md).
- **Step disclosure:** The temporary surface opened from the step indicator
  that lists every step of one workflow.
- **Own workflow:** The workflow named by the previewed task's workflow id,
  which is not always the workflow the board is filtered to.
- **Eligible step:** A step that the existing task-move policy permits as a
  manual target: the step adjacent to the current step, or a step whose
  `allow_manual_move` is set.
- **Panel controls:** The copy-task-link, open-full-page, and close controls:
  the three fixed-width icon buttons in the preview header.
- **Control cluster:** The panel controls plus the task actions menu trigger,
  which renders ahead of them when the task offers actions.
- **Pointer mode:** Fine (mouse, trackpad) or coarse (touch), read live.
- **Minimum panel width:** 320px at a fine pointer, 380px at a coarse pointer
  (AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.7).
- **Indicator floor:** The step indicator's width with its step name truncated
  to nothing: padding, marker, position count, and at a coarse pointer the
  disclosure cue.

## Requirements

### REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001: Step indicator and step navigation in the preview header

**Intent:** A user reading a task in the preview panel needs to see which
workflow step it is on and to move it to any eligible step, including steps
whose board column is hidden or off-screen, without closing the preview.

**User story:** As a board user with the preview open, I want to see and change
the previewed task's workflow step from the preview header, so that I can move
the task even when its target column is not on the board.

#### Acceptance criteria

- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.1:** When the preview panel is
  open on a task and that task's own workflow resolves to at least one step,
  the preview header shall show a step indicator between the task title and the
  panel controls, carrying the current-step marker, the current step name, and
  the position count in the form `<current>/<total>`. The marker is suppressed
  in the one case AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.14 names, and the
  count is omitted in the one case AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.2
  names.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.2:** When the task's own workflow
  has exactly one step, the step indicator shall omit the position count. When
  that single step is the task's current step, the step disclosure shall offer
  no move control, because the current step is never an eligible target. When
  the single step is not the task's current step (the
  AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.14 fallback), the shared eligibility
  policy governs unchanged: a move control shall appear if and only if that
  step's `allow_manual_move` is set. This surface shall not override the shared
  eligibility policy for the single-step case.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.3:** The steps shown shall be the
  steps of the previewed task's own workflow, resolved by that task's workflow
  id, including when the board is filtered to a different workflow and when the
  board shows several workflows at once.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.4:** Steps shall be ordered by
  ascending `position`; when two steps of one workflow share a `position`, the
  system shall order them by ascending step `id` compared as a string, so a
  given step set always renders in one order. This ordering rule shall apply to
  every surface that uses the shared stepper, including the task top bar.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.5:** The step disclosure shall
  list every step of the task's own workflow, including a step the user has
  hidden through the per-workflow column visibility filter and a step whose
  column is scrolled out of view on the board.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.6:** The step disclosure shall
  mark the current step and shall offer a move control for each eligible step
  and for no other step, using the same eligibility policy as the task top bar.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.7:** When the user selects a move
  control, the system shall issue the same task-move request the task top bar
  issues, targeting the previewed task's own workflow id and the selected step
  id, and shall place the task at the first position of the target step. It
  shall not change the task's workflow.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.8:** While a move request is in
  flight, every move control in the step disclosure shall be disabled and the
  disclosure shall stay open, so one user cannot start a second move from this
  surface before the first resolves.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.9:** When a move request succeeds,
  the step disclosure shall close, the preview panel shall stay open on the same
  task with the same selected session, and no route navigation shall occur. When
  the resulting task state arrives, the step indicator shall show the new
  current step and position count.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.10:** When a move request fails,
  the step disclosure shall stay open and usable, and the preview panel shall
  show a move-failure message below the preview header, inside the panel. The
  message shall not appear in the preview header row.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.11:** The move-failure message
  shall clear when the next move request from this surface starts and when the
  preview switches to a different task. Selecting the same target again after a
  failure shall issue the same request again; the system shall not deduplicate,
  queue, or retry move requests on its own. A move request whose failure
  arrives after the preview has stopped showing the task that issued it shall
  be discarded rather than rendered. The message shall appear only when the
  preview has stayed open on that same task continuously since the request was
  issued, so neither switching to another task, nor closing the preview, nor
  closing and reopening it on the same task can surface an error from an
  earlier request.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.12:** When the previewed task's
  step changes from any other source while the step disclosure is open, the
  disclosure shall stay open and shall re-derive its current step, its completed
  steps, and its eligible targets from the newest task state. When a step is
  removed from the workflow while the disclosure is open, its row shall
  disappear without leaving a selection behind.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.13:** When the preview panel has no
  selected task, or the previewed task has no resolvable workflow id, or its own
  workflow resolves to an empty step list, or those steps have not loaded yet,
  the preview header shall show no step indicator and shall otherwise render
  unchanged. The indicator shall appear once the steps resolve, with no
  placeholder or skeleton in between.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.14:** When the previewed task's
  current step id matches no step in the resolved list, the step indicator shall
  show the first step in order without marking it as the current step, and the
  step disclosure shall mark no row as the current step. The disclosure already
  behaves this way. The shared step indicator does not: it marks the step it
  shows as current unconditionally, and shall be corrected to condition that
  marker on a resolved current step. The correction lands in shared code, so it
  applies to every surface using that indicator, including the task top bar.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.15:** When the step disclosure is
  open and the user presses Escape, the system shall dismiss the step disclosure
  only, return focus to the step indicator, and leave the preview panel open. A
  further Escape with the disclosure closed shall close the preview panel, which
  is its behavior today.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.16:** When the preview panel is in
  its floating layout, the step disclosure shall render above the preview panel
  and its backdrop, and pointer interaction inside the disclosure shall not
  close the preview panel.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.17:** On a coarse-pointer device
  that shows the preview panel, the step indicator shall open the same step list
  in the touch surface used by the task top bar, with a minimum 44px hit area
  for the indicator and for each move control, and a visible disclosure cue.
  The indicator's 44px applies to width and height at every panel width
  (AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.4).
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.18:** The step indicator shall
  expose an accessible name carrying the current step name, its step number, and
  the total step count, and the disclosure surface shall expose the same dialog
  semantics and keyboard path as the task top bar's disclosure. In the
  AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.14 fallback, the accessible name
  shall carry, in the same wording, the name and list position of the step the
  indicator shows (the first step) and the total, matching what it renders
  visually. The absence of a current step shall be conveyed by withholding the
  current-step semantics, not by different text, so the accessible name and
  the visible content never disagree about which step is shown or whether the
  task is on it.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.19:** When the preview panel closes
  or switches to a different task while the step disclosure is open, the disclosure
  shall close with it and shall not reopen on the next preview until the user opens
  it again. An in-flight move started from the closed disclosure shall still be
  applied by the backend; the preview shall not cancel it.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.20:** When a fine-pointer user
  hovers or focuses the step indicator, the system shall open the step
  disclosure below it as an interactive dialog surface, and shall close it when
  the pointer leaves both the indicator and the surface and focus is outside
  both. Opening by pointer shall not move focus; opening by keyboard shall move
  focus into the surface.

### REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002: Preview header containment

**Intent:** The preview panel is user-resizable down to a narrow width. The new
step indicator must take its space from the title, not from the panel controls,
and must never turn the header into a second row or a scrolling surface. The
panel's minimum width is set by what the header must fit.

#### Acceptance criteria

- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.1:** At every preview panel width
  from the minimum panel width for the current pointer mode to its maximum, the
  preview header shall stay a single row. No header element shall wrap to a
  second line.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.2:** At the minimum panel width
  for the current pointer mode, every element of the control cluster
  (task actions menu trigger when present, copy-task-link, open-full-page,
  close) shall stay fully inside the panel and shall stay clickable.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.3:** At the minimum panel width
  for the current pointer mode, the task title shall keep at least 88px of
  rendered width and shall truncate with an ellipsis rather than wrap or
  displace any other header element.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.4:** After the control cluster
  and the row's inter-element gaps, the step indicator shall claim at most half
  of the remaining width. The cap is a maximum, not a reservation: when it
  conflicts with the title floor, the title floor wins and the indicator
  shrinks below its cap. The indicator shall never shrink below its indicator
  floor: only the step name truncates, down to nothing, while the marker, the
  count, and the disclosure cue stay fully inside it. At a coarse pointer the
  indicator floor shall be at least 44px. At the minimum panel width, the
  control cluster, the 88px title floor, and the indicator floor shall fit
  together for every workflow of at most 99 steps, while the row's gaps total
  6px or less, as the system design derives.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.5:** The preview header shall not
  introduce horizontal scrolling in the preview panel at any supported width.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.6:** Truncated header text shall
  keep its full value available to assistive technology and shall not be the
  only carrier of the step name, step number, or total.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.7:** The minimum panel width
  shall be 320px at a fine pointer and 380px at a coarse pointer, in both
  layouts. The panel shall render at the larger of the user's chosen width and
  the current minimum, re-evaluated without a reload when the pointer mode
  changes. The chosen width shall never be stored below 320px; a stored value
  below that reads as 320px. Rendering at a larger minimum shall not overwrite
  the stored chosen width, so a 320px choice renders at 380px while the pointer
  is coarse and at 320px again once it is fine. A resize drag shall not choose
  a width below the minimum in effect during the drag. The inline-or-floating
  layout choice shall use the rendered width. The 500px default and the maximum
  width are unchanged.

### REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003: Copy task link control in the preview header

**Intent:** Copying a task's link today means opening the task detail view and
copying the address bar. The preview header should copy it in one click.

**User story:** As a board user with the preview open, I want to copy the
previewed task's link from the preview header without opening the full page.

#### Acceptance criteria

- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.1:** When the preview panel is
  open on a task, the preview header shall show a copy-task-link control among
  the panel controls, before the open-full-page control. With no selected task
  it shall not render, like the other panel controls.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.2:** Selecting the control shall
  copy the task's canonical detail URL (the page origin joined with its
  task-detail path) using the product's shared clipboard-write utility,
  including its non-secure-context fallback.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.3:** The control's icon shall
  differ from the Link submenu's icon, which links an external pull request,
  issue, or tracker resource, so the two actions cannot be mistaken.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.4:** The control shall expose an
  accessible name and tooltip identifying it as copying the task's link,
  worded distinctly from the Link submenu, and shall show a brief visual
  confirmation after a successful copy, like the product's other
  copy-to-clipboard affordances, which clears on its own after a bounded
  duration.

## Decisions

- The preview reuses the task top bar's compact stepper, disclosure surface,
  eligibility policy, and move request rather than a preview-specific variant.
  Two step surfaces with two behaviors is the failure this avoids. "Two surfaces"
  means these two specifically. Other places that move a task between steps,
  including board drag and drop, the swimlane and multi-select movers, and the
  task session sidebar's own optimistic mover, keep their existing
  implementations and are not consolidated by this requirement; the system design
  enumerates them and says why.
- Moving a task from the preview keeps the task top bar's existing plan-mode
  cleanup for the previewed session, including its layout reset. Plan mode is
  session state, not page state, so leaving it from one surface must leave it
  everywhere.
- Concurrent moves from two clients are resolved by the backend. The preview
  shows whatever task state it then receives and does not attempt to reconcile
  or warn.
- The minimum panel width is derived per pointer mode from what the header
  must fit. At 300px the copy-task-link control left no room for a 44px
  indicator at a coarse pointer, or for a two-digit count at a fine one.
  Rejected: exempting this surface from the 44px floor, a title below 88px,
  and a narrow-width overflow menu. Cost: at most 80px of board width.

## Out of scope

- Moving the previewed task to a different workflow. The task context menu
  keeps that path, and this surface deliberately keeps the exclusion stated in
  [REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001](compact-workflow-step-navigation.md).
- Any change to per-workflow column visibility, to board drag and drop, or to
  which columns the board renders.
- Any change to the task top bar's full or compact stepper other than three
  deliberate corrections to shared code, which the top bar inherits by
  construction because the Decisions section forbids a second copy: the
  ordering tiebreak (AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.4), the
  current-step marker fix (AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.14), and
  scoping a late move failure and the in-flight step id to the presentation
  that issued the request (AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.11). On the
  top bar, a failure arriving after the user navigated away from the task, or
  away and back, no longer shows the move-failure message. That is intended: a
  failure outliving its presentation is wrong on both surfaces.
- An archived-task presentation in the preview header. The board excludes
  archived tasks and the preview closes when its task leaves the board, so the
  archived indicator path is not reachable from this surface. A future change
  that makes archived tasks previewable owns that presentation.
- A phone presentation. Phones open the task route instead of the preview
  panel, so this surface adds no phone entry point and the existing phone task
  drawer keeps its **Move to** path.
- Showing wip-limit queue state, `queued_for_step`, or dependency blocking in
  the step indicator.
- Any change to what entering a step triggers. Step `on_enter` actions,
  including agent auto-start, behave exactly as they do for a board drag or a
  task top bar move.
- Workflows of 100 or more steps: their wider indicator floor overflows onto
  the control cluster at the minimum panel width, so
  AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.2 and the fit clause of 002.4 are
  not guaranteed for them.
- The task actions menu trigger's own touch hit area, owned by the task actions
  menu requirements; this surface only counts its row width.
- Resizing the panel by touch. The resize handle stays mouse-only.
- New user-facing copy for the step indicator, the disclosure, and the
  move-failure message, which reuse existing strings. Only the copy-task-link
  control adds translated copy.
