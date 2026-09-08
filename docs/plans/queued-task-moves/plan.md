---
spec: docs/specs/tasks/system-design/wip-limit-pull-system.md
created: 2026-08-12
status: completed
---

# Implementation Plan: Queued Task Moves

## Overview

Make a task move express readiness for the selected workflow step even when
that step has reached its WIP limit. The move succeeds, places the task in the
destination column, and marks it queued without consuming WIP. Capacity later
admits destination-resident queued tasks before pulling work from a configured
feeder.

The UI makes this state explicit. Limited Kanban columns show separate active
and queued areas, and the task sidebar shows a distinct WIP queue icon. Hovering
or focusing the icon discloses the task's one-based queue position, total, and
destination step on desktop and touch layouts.

No new persistence fields are required. The implementation reuses
`wip_admitted`, `queued_for_step_id`, and `queued_at`, but changes task moves
from capacity rejection to atomic destination admission.

## Confirmed Current Behavior

- `task/service.MoveTaskWithOptions` validates target WIP before the update and
  returns a typed capacity conflict when the destination is full.
- The repository has atomic capacity updates and queued-task promotion, but no
  atomic move operation that chooses admitted or queued placement.
- Bulk moves prevalidate capacity, so the whole request fails instead of
  admitting available tasks and queueing the remainder.
- The workflow engine has a separate transition store path that also uses the
  capacity-rejecting update.
- `task.moved` immediately runs both source `on_exit` and destination
  `on_enter`. `task.queue_promoted` currently only handles auto-start, so it
  cannot complete deferred destination entry for a queued move.
- Same-step overflow cards are visible, but the Kanban column does not separate
  admitted and queued cards.
- The sidebar's existing queued count represents pending agent prompts. It is
  unrelated to WIP admission and must remain visually and semantically
  distinct.

## Behavioral Contract

- Moving to an unlimited or non-full limited step admits the task immediately.
- Moving to a full limited step succeeds and places the task in that step with
  `wip_admitted = false`, `queued_for_step_id` equal to the step, and a durable
  `queued_at` value.
- A move never redirects through `pull_from_step_id`. Feeders keep their
  existing automatic-intake behavior.
- Destination-resident queued tasks are promoted before feeder-resident work.
  Queue position uses the same deterministic ordering as promotion.
- A queued move runs source exit once at move time. Destination entry,
  terminal-state effects, context reset, plan-mode behavior, and auto-start run
  once after promotion.
- Moving an already queued task cancels its old queue/deferred-launch state and
  applies admission at the new destination.
- Stepper, drag/drop, move menu, bulk, HTTP, WebSocket, MCP, approval, and
  workflow-engine transition paths share the contract.
- An automatic workflow transition into a full step commits the task in that
  destination queue. Destination entry waits for promotion.
- The move response and task events expose committed admission metadata.

## Backend Design

### Atomic move admission

Add one repository operation analogous to creation admission. In a single
write transaction it locks/checks the target step, updates the task as admitted
when capacity exists, or updates it as destination-resident queued when full.
It returns the committed placement so the service does not infer the result
from a stale capacity read.

For unlimited steps, preserve the ordinary update path and normalize the task
to admitted with no queue destination. For limited steps, count only admitted,
active, non-ephemeral occupants. A race for the final slot admits one task and
queues the other; neither request returns a WIP conflict.

`MoveTaskWithOptions` resolves/validates the destination, clears obsolete queue
and deferred-launch state, applies the atomic admission update, synchronizes
only effects valid for the committed placement, publishes the task update and
move event, and reconciles the vacated source. Bulk moves process each task
through the same admission boundary rather than pre-rejecting the batch.

### Event and lifecycle boundary

Extend `task.moved` data with the committed admission state. For a queued move,
the orchestrator performs source exit and deliberately skips destination entry
and target-derived state. For an admitted move, existing exit/entry behavior
continues.

Extend queue promotion handling into the single destination-entry boundary.
Promotion applies target state, runs `on_enter`, performs context reset and
terminal behavior, and handles auto-start exactly once. This must support tasks
with an existing session and tasks without one. Replayed or duplicate events
must not repeat entry effects.

The workflow engine transition store uses the same admission result and event
contract. It must not leave the engine state ahead of a destination that has
not admitted the task.

### Queue selection

Keep the existing deterministic comparator within a queue. Change selection
to exhaust destination-resident queued candidates before considering feeder
candidates. This gives the displayed one-based position the same meaning as
the next promotion order.

## Frontend Design

### Shared queue model

Add a pure queue helper that:

- identifies destination-resident queued tasks;
- sorts them with the backend promotion order;
- returns one-based position, total, and destination title;
- partitions a limited column into admitted and queued cards.

Use it from the Kanban and sidebar so both surfaces report the same order. The
existing queued-prompt count remains a separate field and badge.

### Kanban column

Within the existing column scroll owner, render admitted cards first and a
visually distinct queued area second. Show the queued-area label and count only
when the queue is non-empty. Keep the header WIP metric as `admitted/limit`.
Drag/drop into a full step succeeds and the card appears in the queued area.

The shared `KanbanColumn` preserves desktop and focused mobile-column parity.
Do not add nested vertical scrolling or horizontal page overflow.

### Sidebar

Derive queue positions from workspace workflow snapshots in
`useWorkspaceSidebarTasks` and carry a distinct WIP queue object through the
desktop and mobile item mappers. `TaskItem` renders the same semantic chip in
both task switchers.

The sidebar renders one compact queue icon on all pointer types. Its tooltip
states `Position N of M in STEP queue`, and the focusable trigger also supports
touch and keyboard users. The icon is status information, not a new action.

All new copy is localized in English, pseudo, Portuguese, and Simplified
Chinese catalogs. Queue helper and component tests cover ordering, render/hide
rules, accessibility, the separate queued-prompt badge, and mobile-safe
presentation.

### Workflow step settings

Keep `Pull from` optional. Add an info tooltip beside it that separates three
behaviors:

- Destination-resident queued tasks receive admission first.
- Kandev then pulls eligible tasks from the selected feeder when capacity is
  available.
- Direct moves and automatic workflow transitions queue in the destination and
  do not require a feeder.

The help also states that new tasks targeting a full step uses the selected
feeder. The tooltip is available by hover or keyboard focus on desktop and by
tap or focus on mobile, so the contract does not depend on pointer hover.

## Responsive and Mobile Contract

- **Entry point:** the existing focused mobile Kanban column and task-switcher
  sheet.
- **Nearest pattern:** `MobileColumnTabs`, `SwipeableColumns`, shared
  `KanbanColumn`, and shared `TaskItem`.
- **Presentation:** inline column section and inline status chip; the workflow
  settings explanation is a compact info tooltip, with no modal, drawer, or
  mobile-only route.
- **Scroll ownership:** the existing Kanban column and task-switcher sheet keep
  their single scroll owners.
- **Touch behavior:** queue position is visible without hover; the workflow
  explanation opens from the existing info icon by tap or focus.
- **Parity:** move, queue visibility, position, and promotion are available on
  desktop and mobile.

## Test Strategy

### Backend

- Atomic move admission admits within capacity and queues overflow under
  synchronized concurrency.
- Bulk movement fills available slots and queues the remainder.
- Moving queued work away clears old queue/deferred state and reapplies the new
  target's admission rule.
- Destination-resident tasks promote before feeder tasks in deterministic
  order.
- Queued moves run source exit once and destination entry only on promotion,
  for existing-session and no-session cases.
- Terminal, auto-start, plan-mode, and context-reset effects are deferred.
- Service, approval, HTTP, WebSocket, MCP, and workflow-engine paths return
  queued success instead of WIP conflict.

### Frontend

- The shared helper matches backend ordering and calculates stable one-based
  positions.
- Kanban columns partition admitted and queued cards and preserve admitted WIP
  counts.
- Sidebar queue status is separate from queued prompts, localized, accessible,
  and correct in desktop and touch presentations.

### Browser E2E

- Desktop: use the UI to move a task into a full step, verify queued placement,
  column partition, sidebar tooltip position, then free capacity and verify
  promotion.
- Mobile Chrome: repeat the move in the focused-column flow, verify the queue
  icon tooltip, promotion, touch usability, and no page-level horizontal
  overflow.
- Seed prerequisite workflow/tasks through the API; perform and assert the
  behavior under test through the UI.

## Public Documentation

Update `docs/public/tasks-and-workflows.md` and
`docs/public/workflow-tips.md`. Replace the manual-move conflict rule with
destination queue admission, explain destination queue priority over feeders,
and describe the Kanban/sidebar queue indicators.

## Implementation Waves

All tasks are sequential. The frontend consumes the finalized backend ordering
and lifecycle contract, and E2E/doc work validates the complete behavior.

- [x] [Task 01: Add atomic move admission](task-01-atomic-move-admission.md)
- [x] [Task 02: Defer destination lifecycle until promotion](task-02-deferred-destination-lifecycle.md)
- [x] [Task 03: Add frontend queue guidance](task-03-kanban-queue-sections.md)
- [x] [Task 04: Show sidebar queue position](task-04-sidebar-queue-position.md)
- [x] [Task 05: Complete feature verification](task-05-e2e-and-documentation.md)

## Implementation Progress

Tasks 01 through 05 are implemented. The backend now commits admitted or
destination-queued placement atomically, defers destination lifecycle work
until promotion, and prioritizes destination-resident queues over feeder work.
The web client shows the same queue order in Kanban and desktop/mobile task
switchers, with localized `Pull from` guidance in an info tooltip. The initial
Kanban boot payload now includes WIP limits and task admission metadata, and the
sidebar uses an accessible queue icon with a position tooltip. Review hardening
also keeps replay tokens available after prerequisite failures, preserves state
changes in the promotion event path, and skips failed fallback candidates while
filling capacity.

Focused backend and frontend checks pass. The broad backend regression gate,
managed browser checks, public-doc validation, and final diff review are
recorded in Task 05.

No task is marked `parallel-safe`; this package does not authorize subagents.

## Environment Prerequisite

If `apps/node_modules` is absent, run `pnpm install --frozen-lockfile` from
`apps/` before frontend tests. Do not change the lockfile.

## Risks

- Applying destination state before admission can complete, reset, or start a
  task while it is still queued.
- Running source exit again during promotion can duplicate messages or agent
  actions. Event data must distinguish move placement from promotion entry.
- A non-atomic count-then-update can still over-admit under concurrent moves.
- Mixing destination and feeder candidates before selection makes the sidebar
  position disagree with actual promotion priority.
- Reusing the sidebar's `queuedCount` field would confuse WIP queue state with
  pending agent prompts.
- Bulk move ordering must be deterministic enough to make which tasks are
  admitted versus queued testable.

## Out of Scope

- User-driven queue reordering.
- Dragging cards within the queued section to change priority.
- Recursive feeder routing.
- Changes to profile-wide agent-session concurrency.
