---
status: shipped
created: 2026-08-18
updated: 2026-08-18
owner: gsimard-nordai
---

# Auto-hide empty workflow steps

## Why

Long workflows spend most of their lifetime with tasks in only a few steps. The board still renders
every empty step, which consumes horizontal space and makes active work harder to scan.

The shipped per-workflow Columns menu intentionally models explicit, persistent hiding. It is not an
adaptive layout mode: a user must maintain the hidden set as work moves, and a manually hidden step
is not a move target. Automatic empty-column hiding must therefore remain a separate concept with a
different drag-and-drop contract.

## Users and outcome

- An individual developer can opt one workflow into a compact board that shows occupied columns.
- A team sharing a Kandev instance retains user-scoped display preferences; one user's compact board
  does not change another user's board.
- Every real workflow step remains reachable as a move destination even while its empty column is
  auto-hidden.

## What

### Per-workflow preference

- Each workflow's existing Columns menu SHALL expose an **Auto-hide empty columns** switch.
- The adaptive display switch SHALL be visually separated from the individual manual column list
  and SHALL not carry secondary descriptive copy.
- The preference SHALL be user-scoped, persisted per workflow, and disabled by default.
- Enabling it SHALL NOT change `kanban_hidden_step_ids` or the checked state of individual manual
  column toggles.
- The same preference SHALL control Kanban and Pipeline surfaces for that workflow on desktop,
  tablet, and phone.

### Occupancy

- A live step is occupied when at least one task that passes the current workflow, repository, and
  plugin filters belongs to that step.
- Free-text search SHALL NOT affect occupancy. Typing in search may filter cards, but SHALL NOT make
  the board structure appear and disappear on every keystroke.
- Tasks in a manually hidden step SHALL still count toward occupancy for the automatic calculation.
  Manual hiding is authoritative and keeps the step hidden regardless of occupancy.
- Orphaned tasks SHALL continue to use the shipped Needs Reassignment behavior and SHALL NOT make an
  unrelated real step occupied.

### Ordinary rendering

- With auto-hide disabled, current board behavior SHALL remain unchanged.
- With auto-hide enabled, each unoccupied, non-manually-hidden live step SHALL be removed from normal
  column rendering.
- A task created in or moved into an auto-hidden step SHALL make that column appear immediately.
- A task leaving a column SHALL make it collapse after the authoritative move update leaves it empty.
- If every live step is auto-hidden, the workflow lane and its Columns menu SHALL remain reachable.
  The lane SHALL show a contextual empty state rather than the generic "No tasks yet" state.

### Drag and move behavior

- When a pointer drag starts, every auto-hidden, non-manually-hidden live step SHALL become an
  accessible drop target for that workflow.
- Auto-hidden destinations MAY render as compact ghost targets rather than full empty columns, but
  they SHALL expose the step title and the same droppable identity as the real step.
- Cancelling a drag SHALL remove ghost targets and restore automatic hiding.
- Dropping into an auto-hidden step SHALL keep that step visible once the task update is authoritative.
- Manually hidden steps SHALL remain hidden and excluded from pointer and bulk-move destinations, as
  required by the existing column-visibility contract.
- Bulk Move and other explicit move selectors SHALL include auto-hidden steps and exclude manually
  hidden steps.
- Pipeline arrows SHALL name an adjacent destination in a tooltip only when that destination is
  auto-hidden; a directly visible neighboring step needs no tooltip.
- Keyboard and screen-reader users SHALL retain access to auto-hidden destinations through the
  existing move controls; this feature does not add keyboard drag-and-drop.

### Mobile and responsive behavior

- The phone Columns control SHALL expose the same preference for the focused workflow.
- Existing phone focused-column navigation, mobile drop targets, safe-area behavior, and 44 CSS px
  touch targets SHALL remain intact.
- Tablet and compact desktop boards SHALL not gain document-level horizontal overflow.
- The preference SHALL not persist responsive state, drag state, or a derived list of empty steps.

## State and persistence

### Persisted preference

The user settings store gains a set of workflow ids whose empty columns are auto-hidden:

```text
workflowIdsWithAutoHideEmptySteps: string[]
```

The REST/WebSocket wire field is:

```text
workflow_ids_with_auto_hide_empty_steps: string[]
```

- Values SHALL be deduplicated and sorted before comparison and persistence.
- Missing or `null` legacy values SHALL hydrate to `[]`.
- Removing a workflow from the set SHALL restore normal rendering without touching manual hidden ids.
- The setting lives in the existing user settings JSON payload; no SQL column or data migration is
  required.

### Derived presentation state

`autoHiddenStepIds` is ephemeral and SHALL be derived from the current filtered task projection and
live workflow steps. It SHALL NOT be stored in the backend, URL, session, or browser storage.

`manuallyHiddenStepIds` and `autoHiddenStepIds` remain separate. Ordinary rendered steps are:

```text
live steps - manually hidden steps - auto-hidden steps
```

During an active drag, move destinations are:

```text
live steps - manually hidden steps
```

## API surface

- Extend the existing user settings GET/update/boot/WS contracts with
  `workflow_ids_with_auto_hide_empty_steps`.
- Do not add a dedicated endpoint.
- Do not change task, workflow-step, move, WIP-limit, or drag payloads.

## Permissions

- The preference uses the current authenticated user's display settings.
- It does not modify a workflow or require workflow-administration permission.
- It never changes which tasks the user is authorized to read or move.

## Failure modes

- A settings persistence failure MAY leave the optimistic local toggle visible until authoritative
  settings reconcile, matching existing display-setting behavior. It SHALL NOT mutate manual hidden
  ids.
- Stale workflow ids in the preference are inert. They SHALL not hide a newly created workflow that
  reuses a title.
- A stale step id is irrelevant because automatic state is derived only from live steps.
- A cancelled or failed move SHALL clear temporary drop-target presentation.
- If a task update arrives after drag state clears, the authoritative task projection determines
  whether the destination column remains visible.
- If every column is empty, the lane header and Columns control remain reachable so the preference
  can be disabled from the same surface.

## Scenarios

- **GIVEN** auto-hide is disabled, **WHEN** a workflow has empty steps, **THEN** every non-manually-
  hidden column renders exactly as it does today.
- **GIVEN** auto-hide is enabled, **WHEN** a step has no task after workflow, repository, and plugin
  filters, **THEN** its ordinary column is absent.
- **GIVEN** a free-text search matches no task in an occupied step, **WHEN** search results update,
  **THEN** the column structure remains stable.
- **GIVEN** an auto-hidden step, **WHEN** pointer drag begins, **THEN** that step becomes an accessible
  drop target.
- **GIVEN** an auto-hidden drop target, **WHEN** the drag is cancelled, **THEN** it disappears again.
- **GIVEN** an auto-hidden drop target, **WHEN** a task is dropped into it and the move succeeds,
  **THEN** the destination column remains visible with the task.
- **GIVEN** a manually hidden step, **WHEN** pointer drag or Bulk Move begins, **THEN** that step is not
  offered as a destination.
- **GIVEN** all live steps are empty, **WHEN** auto-hide is enabled, **THEN** the workflow lane and
  Columns menu remain visible with contextual guidance.
- **GIVEN** two workflows share step titles, **WHEN** one enables auto-hide, **THEN** the other
  workflow is unaffected.
- **GIVEN** the preference is enabled, **WHEN** the page reloads or another tab hydrates settings,
  **THEN** the same workflow resumes automatic empty-column hiding.
- **GIVEN** a phone viewport, **WHEN** the user enables auto-hide for the focused workflow, **THEN**
  focused-column navigation and drag targets remain usable without document overflow.

## Out of scope

- Automatically or semantically classifying steps as Backlog, Done, Review, or another phase.
- Matching steps by title or `stage_type` across workflows.
- Automatically changing the manually hidden step set.
- Making manually hidden steps available during drag.
- Persisting derived empty-step ids, drag state, search state, or responsive layout state.
- Redesigning the task move API, workflow engine, WIP limits, or task lifecycle.
- A global install-wide auto-hide policy or administrator-enforced preference.

## References

- [Per-workflow column visibility spec](../board-step-visibility-filter/spec.md)
- [Merged PR #2467](https://github.com/kdlbs/kandev/pull/2467)
- `apps/web/components/kanban/columns-menu.tsx`
- `apps/web/components/kanban/swimlane-container.tsx`
- `apps/web/components/kanban/swimlane-kanban-content.tsx`
- `apps/web/components/kanban/swimlane-graph-content.tsx`
- `apps/web/components/kanban/mobile-drop-targets.tsx`

## Implementation plan

[Auto-hide empty Kanban columns implementation plan](../../plans/kanban-auto-hide-empty-columns/plan.md)
