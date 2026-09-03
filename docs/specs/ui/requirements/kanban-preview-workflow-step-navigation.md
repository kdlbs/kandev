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
Its header shows the task title, an open-full-page control, and a close
control. It does not show which workflow step the task is on, and it offers no
way to move the task.

Today the only in-board way to change a task's step is dragging its card to
another column. That path is not always available. The per-workflow column
visibility filter
([REQ-UI-BOARD-STEP-VISIBILITY-FILTER-001](board-step-visibility-filter.md))
lets a user hide any step, including the one a task should move to, and the
preview panel narrows the board so remaining columns can sit off-screen. In
both cases the drop target does not exist on screen and the task cannot be
moved without leaving the preview.

The task top bar already solves the same problem in a narrow space
([REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001](compact-workflow-step-navigation.md)):
a compact current-step indicator that discloses every step with a move control
for each eligible target. This requirement puts that same indicator in the
preview header and constrains its footprint so the header's existing title and
controls keep their space.

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
- **Panel controls:** The open-full-page control and the close control in the
  preview header.

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
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.18:** The step indicator shall
  expose an accessible name carrying the current step name, its step number, and
  the total step count, and the disclosure surface shall expose the same dialog
  semantics and keyboard path as the task top bar's disclosure. In the
  AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.14 fallback there is no current step,
  so the accessible name shall instead carry the name and list position of the
  step the indicator is showing, which is the first step in order, together with
  the total. It shall use the same wording as the resolved case, because that is
  exactly what the indicator renders visually in that case: the marker is
  suppressed and the visible position count follows the single-step rule, so it
  is omitted. The absence of a current step shall be conveyed by withholding the
  current-step semantics from the indicator, not by different text, so the
  accessible name and the visible content never disagree about which step is
  shown or about whether the task is on it.
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
and must never turn the header into a second row or a scrolling surface.

#### Acceptance criteria

- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.1:** At every preview panel width
  from its 300px minimum to its maximum, the preview header shall stay a single
  row. No header element shall wrap to a second line.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.2:** At the 300px minimum width,
  both panel controls shall stay fully inside the panel and shall stay
  clickable.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.3:** At the 300px minimum width,
  the task title shall keep at least 88px of rendered width and shall truncate
  with an ellipsis rather than wrap or displace any other header element.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.4:** After the panel controls and
  the header row's inter-element gaps take their fixed width, the step indicator
  shall claim no more than half of the width that remains, at every panel width.
  When the step name is too long for the space that leaves it, the step name
  shall truncate while the current-step marker and the position count stay
  visible. This cap is a maximum, not a reservation: when a header row cannot
  satisfy both this cap and the task title floor
  AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.3 sets, the title floor wins and the
  step indicator shall shrink below its cap. The two bounds never conflict while
  the header row's inter-element gaps total 18px or less, which the system design
  derives from the narrower of the two preview layouts and which the header shall
  respect.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.5:** The preview header shall not
  introduce horizontal scrolling in the preview panel at any supported width.
- **AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.6:** Truncated header text shall
  keep its full value available to assistive technology and shall not be the
  only carrier of the step name, step number, or total.

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

## Out of scope

- Moving the previewed task to a different workflow. The task context menu
  keeps that path, and this surface deliberately keeps the exclusion stated in
  [REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001](compact-workflow-step-navigation.md).
- Any change to per-workflow column visibility, to board drag and drop, or to
  which columns the board renders.
- Any change to the task top bar's full or compact stepper, in presentation or
  in behavior, other than the three corrections this requirement makes to shared
  code deliberately. The first two are presentation: the ordering tiebreak in
  AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.4 and the current-step marker fix in
  AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.14. The third is behavior: the shared
  move hook scopes a late move failure, and the in-flight step id, to the
  presentation that issued the request, per
  AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.11 and the system design's failure and
  recovery rules. On the top bar that means a move failure arriving after the user
  has navigated away from the task, or away and back to it, no longer raises the
  move-failure message, where today it can. All three live in code the top bar
  shares, and the requirement's own Decisions section forbids a second copy of
  that code, so the top bar inherits all three by construction rather than by
  choice. The third is deliberate and not incidental: a failure that outlives the
  presentation which issued it is wrong on both surfaces, and shipping the fix on
  the preview alone would leave the identical defect on its sibling. No other top
  bar change is in scope.
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
- New user-facing copy. This surface reuses the existing translated strings for
  the step indicator, the disclosure, and the move-failure message.
