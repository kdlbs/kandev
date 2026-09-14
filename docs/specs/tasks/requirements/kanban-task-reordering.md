---
status: active
system: tasks
created: 2026-09-04
owners:
  - kandev
---
# Kanban Task Reordering Requirements

## Overview

A workflow step's task order already decides real behavior: the WIP overflow
queue promotes the lowest-`position` task when a slot frees. Today a
user cannot set it. `position` is written only as a side effect of a move, and
the board sorts each column newest first, so what the user sees is unrelated to
what the system picks up next. This capability makes that order
a user-controlled, persisted contract: the user drags a card to a new place in
its column, every view reconciles to it, and the top card is the one the step
takes next.

Prior art, the wire contract, and the mechanism behind these criteria are in the
paired [system design](../system-design/kanban-task-reordering.md).

## Terminology

- **Step list:** The ordered tasks the board renders for one workflow step.
- **Task position:** The `position` column of a task, which this capability
  makes user-controlled. Every unqualified use of "position" in this document
  means this column.
- **Step ordinal:** The `position` column of a workflow *step*, fixing that
  step's place among its workflow's steps. A different column from the task
  position, and never written by a reorder.
- **Queued band:** Tasks physically in the step that are queued for that same
  step and not WIP-admitted (`wip_admitted` false and `queued_for_step_id`
  equal to the step).
- **Admitted band:** Every task of the step that is not in the step's queued
  band. Defined by exclusion, so the two bands partition the step. A task
  sitting here while queued for a *different* step is therefore admitted here
  and reordered like any other member.
- **Band:** The admitted band or the queued band of one step.
- **Hidden task:** A task the board never shows in any step list: archived
  (`archived_at` set), ephemeral (`is_ephemeral` true), or automation-run
  (`origin` equal to `automation_run`).
- **Band membership:** The tasks of a band that are not hidden tasks.
  Membership is independent of any board filter.
- **Step order:** The order AC-TASKS-KANBAN-TASK-REORDERING-001.1 defines over
  the tasks of one band. Where a criterion applies it across a whole step, it
  means those same keys over the step's non-hidden tasks.
- **Reorder:** A request that rewrites the stored order of exactly one band.

## Requirements

### REQ-TASKS-KANBAN-TASK-REORDERING-001: Persisted task order within a workflow step

**Intent:** Give a step's task order a single, persisted, user-controlled
definition so the board shows, and the WIP pull uses, the same priority order.

**User story:** As a user with a full backlog, I want to drag a task to the top
of its step, so that it is visibly and actually the next task that step takes.

#### Acceptance criteria

##### Order contract

- **AC-TASKS-KANBAN-TASK-REORDERING-001.1:** The system shall order the tasks of
  a band by `position` ascending, then by priority rank (`critical`, `high`,
  `medium`, `low`, then any other or absent value), then by `queued_at`
  ascending treating an absent `queued_at` as that task's `created_at`, then by
  `created_at` ascending, then by `id` ascending. Every key is a named column
  and `id` is unique, so the order is total for any two distinct tasks.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.36:** The board, the queue comparator
  and every promotion comparison shall apply
  AC-TASKS-KANBAN-TASK-REORDERING-001.1's `queued_at` key identically, using
  the absent-value rule stated there. No other promotion behavior, including
  which candidate source is preferred, shall change.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.2:** The board shall render every step
  list in step order in both shipped views: the Kanban view, on desktop, tablet
  and mobile, and the Pipeline view. Lower `position` shall render nearer the
  top. The topmost card of a band is the one it yields next, except under a
  board filter, where AC-TASKS-KANBAN-TASK-REORDERING-001.34 governs and a
  hidden member may rank ahead of it. In the Kanban view, which splits a
  step into bands, the admitted band shall render first and the queued band
  below it under the existing queued divider. The Pipeline view, which renders
  one row per task and has no divider, shall order each step's rows by
  AC-TASKS-KANBAN-TASK-REORDERING-001.1 and shall gain no band split.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.3:** The system shall order the two
  bands of a step independently: a change to one band shall not change the
  relative order of the other.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.4:** When a WIP slot frees in a step,
  the system shall choose the candidate source exactly as it does today,
  preferring the step's own queued band over its configured feeder step, and
  within the chosen source shall promote the first candidate in step order. The
  destination-first contract in
  [WIP Limits and Visible Overflow Queues](wip-limit-pull-system.md) is
  unchanged apart from AC-TASKS-KANBAN-TASK-REORDERING-001.36's alignment.

##### Reorder interaction

- **AC-TASKS-KANBAN-TASK-REORDERING-001.5:** When a user drags a card and drops
  it at a different index within the same band, the system shall commit that
  band's new order.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.6:** A drag shall start from the card
  body under the existing activation constraints (pointer movement of at least
  8 CSS pixels, or a touch hold of at least 250 ms), and no grab handle shall be
  added. A click that does not exceed the activation constraint shall retain its
  current behavior.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.7:** While a card is dragged over its
  own band, the system shall show the insertion point the drop would take.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.8:** When a drop commits, the board
  shall show the new order immediately and then reconcile to the persisted
  order returned by the system.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.9:** When a drag is cancelled by the
  Escape key or dropped outside any band, the system shall leave the order
  unchanged and shall issue no reorder request.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.10:** When a drop would produce the
  order already displayed, the system shall issue no reorder request and shall
  leave the board unchanged.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.11:** When a card is dropped on the
  other band of the same step, the system shall reject the drop, leave both
  bands unchanged, and issue no request.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.12:** A user shall be able to reorder a
  card within its band using the keyboard alone, on the same presentations that
  accept a pointer reorder. With a card focused, Space or Enter shall pick it
  up, Up Arrow and Down Arrow shall move it one place among the band members the
  board is currently showing, Space or
  Enter shall drop it and commit, and Escape shall cancel it under
  AC-TASKS-KANBAN-TASK-REORDERING-001.9. While a card is picked up the board
  shall show the insertion point of AC-TASKS-KANBAN-TASK-REORDERING-001.7 and
  shall announce the card's place in its band to assistive technology on pick-up
  and each move. An arrow press that would carry the card out of its band, or
  past the last member the board is showing, shall be ignored. The committed
  order shall be derived as in AC-TASKS-KANBAN-TASK-REORDERING-001.34 and shall
  persist identically to a pointer drag.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.13:** Dragging a card onto a different
  step shall continue to move the task to that step with its existing
  behavior.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.38:** The Pipeline view shall render
  step order per AC-TASKS-KANBAN-TASK-REORDERING-001.2 but shall accept no
  reorder by any input, neither a pointer drag nor the keyboard commands of
  AC-TASKS-KANBAN-TASK-REORDERING-001.12, and shall issue no reorder request.
  Its existing per-task step-move control is unchanged.

##### Persistence and propagation

- **AC-TASKS-KANBAN-TASK-REORDERING-001.14:** The system shall persist a
  committed order server-side so that it survives a page reload, a workflow or
  workspace switch, and a Kandev restart.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.15:** When a reorder commits, the system
  shall renumber the named step densely from `0`: immediately after a reorder
  the `position` values of the step's non-hidden tasks shall be exactly `0`
  through `N-1`, where `N` is their count, with no gap and no duplicate. The
  reordered band shall hold the submitted sequence, the other band shall keep
  its existing relative order, and each band's values shall be contiguous and
  disjoint from the other band's, with the admitted band taking the lower
  range. A hidden task's `position` shall not be rewritten and shall not
  participate in this guarantee. Band membership shall never be inferred
  from `position`; it is decided by `wip_admitted` and `queued_for_step_id`.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.16:** When a reorder commits, the system
  shall publish it so every connected view of that workflow reconciles to the
  new order without a manual refresh.

##### Reorder request contract

- **AC-TASKS-KANBAN-TASK-REORDERING-001.17:** A reorder shall name one workflow
  step, name which of its two bands it targets as an explicit value rather than
  one inferred from the submitted identifiers, and carry the complete ordered
  list of task identifiers for that band. A reorder
  shall not carry a single task's new index.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.18:** When the request names no valid
  band, or the submitted identifier list is empty, contains a duplicate, names a task that is not in the named step, or
  names a task that is in the named step but not in the named band, the system
  shall reject the request with a validation error and shall change no
  `position`.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.19:** When the submitted identifier set
  does not exactly equal the current membership of the named band, the system
  shall reject the whole request atomically with the typed conflict error
  `step_changed`, shall change no `position`, and the board shall silently
  reconcile to the authoritative order without showing an error.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.20:** When a reorder is rejected for any
  reason other than the conflict in
  AC-TASKS-KANBAN-TASK-REORDERING-001.19, the board shall restore the
  authoritative order and shall show a localized failure message.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.21:** A reorder shall not change any
  task's workflow, workflow step, WIP admission, queued destination, queued
  time, or state; shall not start, stop or interrupt a session; and shall not
  run step entry or exit behavior, record a step transition, or publish a
  task-moved event.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.22:** The system shall accept a reorder
  for a step whose tasks have active sessions, and shall not apply the
  active-session restriction that guards a step change.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.23:** A caller authorized to move a task
  in a workflow shall be authorized to reorder the tasks within that workflow's
  steps. This grants no authority over the step ordinal, which a separate
  shipped capability owns. The request shall carry no caller-supplied identity,
  and submitted identifiers shall be validated against persisted membership
  before any write.

##### Idempotency and concurrency

- **AC-TASKS-KANBAN-TASK-REORDERING-001.24:** When the same reorder request is
  submitted more than once and the band's membership has not changed, each
  submission shall succeed and shall leave the same persisted order, so a retry
  is safe.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.25:** When two callers submit reorders
  for the same band with the same membership, the system shall apply them in
  arrival order, the last committed order shall be authoritative, and every
  connected view shall reconcile to it.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.26:** When the named band's membership
  changes between the client reading the order and the drop committing — a task
  created in, moved into or out of, deleted from, promoted into or queued into
  it, or becoming a hidden task — the system shall treat the request as the
  conflict in AC-TASKS-KANBAN-TASK-REORDERING-001.19. Any membership change in
  that window conflicts, whatever caused it.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.27:** While a reorder request for a band
  is in flight, the board shall not issue a second reorder request for that
  band, and shall prevent one being started: its cards shall not accept a
  reorder drop and shall not accept the keyboard commands of
  AC-TASKS-KANBAN-TASK-REORDERING-001.12, and the board shall keep showing the
  optimistic order from AC-TASKS-KANBAN-TASK-REORDERING-001.8 until the request
  resolves. A gesture already in progress shall be cancelled under
  AC-TASKS-KANBAN-TASK-REORDERING-001.9 rather than queued. Only reordering is
  suspended: a card in that band shall stay draggable and a drop on another step
  shall move it under AC-TASKS-KANBAN-TASK-REORDERING-001.13, while a drop
  within its own band shall be ignored. The step's other band, and every other
  step, shall stay reorderable. A published order under
  AC-TASKS-KANBAN-TASK-REORDERING-001.16 arriving while the request is in flight
  shall be applied to the other band and to other steps, while this band holds
  its optimistic order until the request resolves.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.37:** When two reorders name the same
  step, the system shall apply them one at a time rather than concurrently, each
  renumbering the step from the state the previous one committed. When they name
  the step's two different bands, neither shall be rejected because of the
  other, and each band shall end in the order its own reorder submitted. When
  they name the same band,
  AC-TASKS-KANBAN-TASK-REORDERING-001.25 governs instead.

##### Placement of arriving tasks

- **AC-TASKS-KANBAN-TASK-REORDERING-001.28:** When a task enters a band by
  creation, manual move, bulk move, drag between steps, WIP promotion, or
  automatic workflow transition, the system shall give it a `position` one
  greater than the highest held by any non-hidden task already in that step, or
  `0` when the step holds none, and shall not disturb the relative order of the
  tasks there. The arriving task therefore sorts last in its band and never
  displaces work a user has already ordered. An arrival shall observe that
  highest value and write its own atomically with respect to any reorder of the
  same step, so that an arrival concurrent with a reorder still sorts last.
  Between reorders a step's values may therefore be non-contiguous and the two
  bands' ranges may overlap:
  AC-TASKS-KANBAN-TASK-REORDERING-001.15's dense guarantee holds immediately
  after a reorder, not permanently. A `position` supplied by the caller on any
  of these paths shall be ignored, including the plugin move API's `position`
  field, whose documented top-of-step meaning this criterion supersedes. A move
  naming the task's current step is not an arrival and shall leave its
  `position` unchanged.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.29:** When several tasks enter the same
  band in one bulk move, the system shall place them last in that band, ordered
  by the step ordinal of their source step ascending and, within one source
  step, by that step's admitted band in step order followed by its queued band
  in step order. Their `position` values shall be consecutive, shall start from
  the value AC-TASKS-KANBAN-TASK-REORDERING-001.28 defines, and shall preserve
  that sequence.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.30:** When a task leaves a band, the
  system shall preserve the relative order of the remaining tasks. `position`
  values need not remain contiguous after a departure.

##### Existing data and empty states

- **AC-TASKS-KANBAN-TASK-REORDERING-001.31:** The system shall require no
  migration of stored `position` values, and shall rewrite a task's `position`
  only when that task is in a step being renumbered under
  AC-TASKS-KANBAN-TASK-REORDERING-001.15 or is entering a band under
  AC-TASKS-KANBAN-TASK-REORDERING-001.28. No task in another step, and no
  hidden task, shall have its `position` rewritten.
  Existing tasks that share a `position` shall order by the remaining keys of
  AC-TASKS-KANBAN-TASK-REORDERING-001.1.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.32:** An empty step shall render as it
  does today and shall accept a card dragged from another step. A band that
  shows fewer than two tasks shall render them, shall offer no reorder within
  itself, and shall issue no reorder request, whether its membership is that
  small or a board filter hides the rest.

##### Hidden and filtered tasks

- **AC-TASKS-KANBAN-TASK-REORDERING-001.33:** A task already hidden when the
  client read the band (Terminology's hidden tasks) shall never be part of band
  membership, shall never be named in a reorder, shall never have its `position`
  rewritten, and shall never cause the conflict in
  AC-TASKS-KANBAN-TASK-REORDERING-001.19. A task that becomes hidden inside the
  window is governed by AC-TASKS-KANBAN-TASK-REORDERING-001.26 instead and does
  conflict. The membership a submitted list is validated against shall exclude
  exactly the classes the board excludes when it renders the step list, so a
  client submitting every task it can see never conflicts on a task it was never
  shown.
- **AC-TASKS-KANBAN-TASK-REORDERING-001.34:** When the board hides part of a band
  because a search query, repository filter, or plugin task filter is active,
  reordering shall remain available, and the request shall carry the band's
  complete membership rather than only the visible tasks. No task but the
  dragged one shall change its relative order, so filtered-out tasks are never
  rearranged by a reorder they were not part of. When the filter leaves the band
  showing fewer than two
  tasks, AC-TASKS-KANBAN-TASK-REORDERING-001.32 applies and no reorder is
  offered.

## Compatibility

- **AC-TASKS-KANBAN-TASK-REORDERING-001.35:** After this capability ships, a
  step list that has never been reordered shall render in the order of
  AC-TASKS-KANBAN-TASK-REORDERING-001.1, where it previously rendered newest
  task first. Tasks created in the step they still occupy hold `position` 0 and
  therefore sort together, highest priority first and oldest first within a
  priority; tasks moved, bulk-moved or promoted in hold whatever their move path
  wrote and may sort after them. No stored `position` changes, and the only
  promotion change is the tiebreak alignment of
  AC-TASKS-KANBAN-TASK-REORDERING-001.36.

## Out of scope

- **Dragging a card between different steps or workflows.** Unchanged by
  AC-TASKS-KANBAN-TASK-REORDERING-001.13.
- **Dragging a card across the admitted and queued divider.** Rejected by
  AC-TASKS-KANBAN-TASK-REORDERING-001.11.
- **Reordering in the Pipeline view, by pointer or keyboard.** Per
  AC-TASKS-KANBAN-TASK-REORDERING-001.38: it lays out one row per task, so no
  insertion point or arrow has a defined meaning there.
- **A band split or queued divider in the Pipeline view.** It has neither today
  and gains neither, per AC-TASKS-KANBAN-TASK-REORDERING-001.2.
- **Selecting several cards and dragging them as a group.** Bulk move keeps its
  existing multi-select path, placed by
  AC-TASKS-KANBAN-TASK-REORDERING-001.29.
- **Move-up and move-down buttons or a menu action.** Covered by
  AC-TASKS-KANBAN-TASK-REORDERING-001.12.
- **Choosing where a new task lands.** Arrivals go last, per
  AC-TASKS-KANBAN-TASK-REORDERING-001.28.
- **Any other sort key, or a sort control.** Step order is exactly
  AC-TASKS-KANBAN-TASK-REORDERING-001.1.
- **Changing `priority`, or deriving it from `position`.** They stay separate.
- **Unifying any promotion behavior beyond the `queued_at` tiebreak.**
  AC-TASKS-KANBAN-TASK-REORDERING-001.36 aligns that one key and nothing else.
- **A WebSocket action for a reorder.** HTTP only here; deferred as Kandev task
  `48fe260c-de9d-434b-8c8f-3aed46280dca`.
- **Reordering from the task list, sidebar, command panel, or Office
  surfaces.** Those have their own orderings and are untouched.
