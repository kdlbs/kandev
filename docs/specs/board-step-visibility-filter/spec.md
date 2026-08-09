---
status: draft
created: 2026-08-09
owner: nova28
---

# Per-workflow step visibility filter on the kanban board

## Why
Users want to declutter the kanban board by hiding tasks that sit in steps they
do not currently care about. The motivating request is "show all tasks except
the ones in Done" — but it generalises to any step on any board (hide `Backlog`,
hide `Review`, etc.).

There is deliberately **no** cross-workflow semantic layer to lean on:

- `workflow_steps.stage_type` is `"custom"` on nearly every step. The office /
  workflow engine *does* branch on `stage_type` in places (e.g. review-phase
  routing in `apps/backend/internal/office/service/prompt_builder.go` and
  `scheduler_integration.go`), but that signal is meaningless on the kanban board:
  because nearly every step is `"custom"`, the board has no reliable per-column
  phase signal and cannot know which column is "the Done phase". The board UI must
  therefore never classify columns by `stage_type` (see the field comment in
  `apps/web/lib/state/slices/kanban/types.ts`).
- The same step title in two workflows is not the same step — `Review` in one
  workflow has a different id, prompt, and meaning than `Review` in another.
- `task.state` is the agent-session lifecycle, not the board column, and the two
  can disagree (a task can sit in the Done column with `state: REVIEW`). Filtering
  on `state` would misreport what is actually in a column.

The only ground truth is: each workflow owns an ordered set of steps, and a task
is in exactly one step of exactly one workflow. So the feature is, precisely,
**per-workflow column show/hide keyed on the real `workflowStepId`.**

## What
- The kanban board's **Display** dropdown
  (`apps/web/components/kanban-display-dropdown.tsx`) SHALL gain a new "Steps"
  section, a sibling to the existing Workflow and Repository sections.
- The Steps section SHALL present **one group per workflow selected by
  `selectWorkflowSwimlanes`** — the board's *eligible* workflow set (workflows with a
  loaded snapshot, honouring the active Workflow filter), NOT the post-filter set the
  board actually renders. This distinction is load-bearing: a workflow whose every step
  is hidden is dropped from the rendered board (it has zero visible tasks) but its group
  MUST remain in the Steps section so the user can re-tick a step (see
  [Steps-section scope](#steps-section-scope)). Within each group the section SHALL list
  that workflow's steps as checkboxes in `position` order (tiebreak by `id`, see
  [Ordering](#ordering--determinism)). The group set is the same on mobile as on desktop:
  the mobile board's single-workflow focus navigator changes which board is rendered, not
  which groups the Steps section offers.
- Every step SHALL be **shown (ticked) by default**. A step is hidden only when
  the user explicitly unticks it.
- Unticking a step SHALL, on every board surface that renders that workflow
  (single-workflow board and multi-workflow swimlane, kanban and pipeline views):
  1. **hide that step's tasks**, and
  2. **collapse (remove) that step's now-empty column** — not merely empty it.
  Both effects are required together (see
  [The dual-filter contract](#the-dual-filter-contract)).
- The filter SHALL be **scoped strictly per workflow id**: hiding step `S` in
  workflow `A` SHALL NOT affect any step of workflow `B`, including a step in `B`
  that shares `S`'s title. Cross-workflow bleed SHALL be impossible by
  construction (the selection is keyed by workflow id, and a task is matched only
  against its own workflow's hidden set).
- Re-ticking a step SHALL restore its column and its tasks with no other side
  effect.
- The selection SHALL **persist** across reloads and sessions in backend user
  display settings (the same tier as the Workflow and Repository filters), not
  session-only.
- The selection SHALL track step **id**, never title, so renaming a step keeps
  its hidden/shown state.
- The Steps filter SHALL **compose (AND)** with the Workflow filter, Repository
  filter, search query, and any plugin task filters: a task is visible only if it
  passes all of them.

## Data model & state shape

### Store (frontend)
`UserSettingsState` (`apps/web/lib/state/slices/settings/types.ts`) gains one
field holding the **hidden** set, keyed by workflow id:

```
hiddenWorkflowStepIds: Record<string /* workflowId */, string[] /* hidden stepIds */>
```

Rationale for storing *hidden* (not *shown*): a step absent from the map defaults
to visible, so a newly-added step is shown by default rather than silently hidden,
and the map only grows with explicit user choices.

The persisted/serialized form is arrays (JSON-friendly). The runtime predicate
MAY build a `Set` per workflow for O(1) membership; the "hidden set" in the
locked design refers to this membership semantics, realised on the wire as a
`string[]`.

Normalisation: within each workflow's array the ids SHALL be de-duplicated and sorted
ascending (mirroring how `buildNormalizedSettings` normalises `repositoryIds` via
`Array.from(new Set()).sort()`), so the serialized form is canonical; a workflow whose
array becomes empty SHALL be removed from the map entirely (no empty-array keys
persisted), mirroring how the plugin-filter store drops an empty selection. Because the
persisted form is already sorted, `isSettingsUnchanged`'s order-insensitive per-workflow
comparison is stable.

### Persistence round-trip (exact contract)
The setting persists through the same path as the other display filters. All
field names below are the contract; they exist so Build is not forced to invent
them.

- **DB / wire (snake_case): `kanban_hidden_step_ids`**, typed as a JSON object
  `map[string][]string` (workflowId → hidden stepIds).
  - `apps/backend/internal/user/models/models.go` — `UserSettings.KanbanHiddenStepIDs map[string][]string` (`json:"kanban_hidden_step_ids"`).
  - `apps/backend/internal/user/dto/dto.go` — add to `UserSettingsDTO` (value) and `UpdateUserSettingsRequest` (as `*map[string][]string`, `,omitempty`, so an absent field is not treated as "clear"); map it in `FromUserSettings` (the `*models.UserSettings → UserSettingsDTO` mapper at dto.go:217 — there is no `ToAPI`).
  - `apps/backend/internal/user/controller/controller.go` and `.../service/service.go` — copy the field in the update path (`if req.KanbanHiddenStepIDs != nil { settings.KanbanHiddenStepIDs = *req.KanbanHiddenStepIDs }`), include it in the settings event data map and in the boot-state settings map.
  - `apps/backend/internal/user/store/sqlite.go` — include it in the marshalled payload and the scan struct (settings persist as a JSON blob in `users.settings`).
  - `apps/backend/internal/backendapp/boot_state_routes.go` (`mapUserSettingsState`, and `boot_state.go` if it mirrors the map) — emit under the boot key **`hiddenWorkflowStepIds`**.
- **Boot payload key: `hiddenWorkflowStepIds`** — the **store-field name**, NOT the
  camelCased wire name. The boot payload's `userSettings` object is deep-merged into
  the store by direct key match (`deepMerge(draft, source)` in
  `apps/web/lib/state/hydration/hydrator.ts`), with no snake→camel remapping, so the
  emitted key MUST equal the Zustand field. This matches the existing precedent:
  `mapUserSettingsState` emits `WorkflowFilterID` under `workflowId` and `RepositoryIDs`
  under `repositoryIds` — store names, not `workflowFilterId` / `repository_ids`. Emitting
  `kanbanHiddenStepIds` (the DTO json name camelCased) would write a dead key that no
  selector reads, leaving `hiddenWorkflowStepIds` at `{}` on every cold boot.
- **Frontend hydration:** the REST / SSR path (`apps/web/lib/ssr/user-settings.ts`,
  `mapUserSettingsData`) reads the **snake_case wire** field and maps
  `s.kanban_hidden_step_ids ?? current.hiddenWorkflowStepIds` into
  `hiddenWorkflowStepIds`; the default is `{}`. (Boot payload uses the store-name key
  above; REST uses snake_case — both resolve to the same store field.)
- **Frontend persist:** `apps/web/hooks/use-user-display-settings.ts` —
  `CommitPayload` gains the field, `buildNormalizedSettings` normalises it,
  `isSettingsUnchanged` compares it (deep, order-insensitive per workflow), and
  `persistSettingsPayload` sends `kanban_hidden_step_ids` in the
  `user.settings.update` payload.
- **WS echo:** the `user.settings.updated` broadcast handler
  (`apps/web/lib/ws/handlers/users.ts` — the server echo, distinct from the
  `user.settings.update` *request* action the client sends via `persistSettingsPayload`)
  that refreshes `userSettings` from a server echo SHALL carry the new field through the
  same mapping used at REST hydration.

Postgres compatibility: the field is stored inside the existing JSON settings
blob, so no new column and no dialect-sensitive SQL is introduced.

## Steps-section scope
The Steps section mirrors the board's own workflow grouping so it never offers
controls for boards the user is not looking at:

- **Workflow filter = "All Workflows":** one group per workflow that has a loaded
  snapshot and is not hidden — i.e. the set returned by
  `selectWorkflowSwimlanes(null, workflows, snapshots)` in
  `apps/web/components/kanban/swimlane-container.tsx`.
- **Workflow filter = a specific workflow:** exactly one group, for the selected
  workflow (hidden or not), matching `selectWorkflowSwimlanes(workflowId, …)`.
- Group membership is independent of the Repository filter, the search query, the
  render-time `getVisibleWorkflows` drop, and the mobile single-workflow focus
  navigator (those hide tasks or narrow the rendered board, not the Steps controls).
  A workflow group with all-hidden steps still lists its steps so the user can
  re-tick them, even though that workflow is no longer rendered on the board.
- Each group's steps come from that workflow's snapshot `steps`
  (`kanbanMulti.snapshots[workflowId].steps`), the same source the board renders,
  and are listed in `position` order (see [Ordering](#ordering--determinism)).
- The synthetic **"Needs Reassignment"** orphan column (`ORPHAN_STEP_ID`,
  `apps/web/components/kanban/swimlane-kanban-content.tsx`) is a display-only
  fallback and is NOT a real step: it SHALL NOT appear in the Steps section and
  SHALL NOT be hideable.

## Steps-section selectors
The new Steps-section controls SHALL carry stable `data-testid`s so the required
Playwright spec has a selector contract (matching the testid-precision of the rest of
this spec — `kanban-column-<stepId>`, `task-context-step-<stepId>`,
`bulk-move-step-<stepId>`, `display-button`). Build MAY choose the exact copy but NOT
drop these ids:

- Each per-workflow group container: `data-testid="steps-filter-group-<workflowId>"`.
- Each per-step checkbox (or its clickable row): `data-testid="steps-filter-step-<stepId>"`,
  reflecting checked/unchecked state through the control's standard checked semantics
  (e.g. `aria-checked` / `data-state`) so a test can assert ticked vs unticked.

Step ids are the real `workflowStepId` / `workflow.id`, never the title. The synthetic
"Needs Reassignment" orphan column (`ORPHAN_STEP_ID`) has no Steps-section control and
therefore no such testid.

## The dual-filter contract
This is the single most important behavioural detail and the easiest to get
wrong. For a workflow with hidden set `H`:

1. **Task hiding:** tasks whose `workflowStepId ∈ (H ∩ liveStepIds)` are removed from
   the tasks passed to the view — where `liveStepIds` is the set of `id`s in that
   workflow's current snapshot `steps`. The seam is `filterTasks` in
   `apps/web/components/kanban/swimlane-container.tsx`, applied per snapshot, AND scoped
   to the task's own `workflowId`. The intersection with `liveStepIds` matters ONLY for a
   stale hidden id (a hidden step that no longer exists): such an id hides nothing, so a
   task still pointing at it is left for orphan-remap exactly as with an empty hidden set
   (see [stale-id boundary](#nil--empty--error--defaults--boundary)). For any hidden step
   that still exists, `H ∩ liveStepIds` contains it, so its tasks are removed as required.
2. **Column collapse:** steps whose `id ∈ H` are removed from the `steps` list
   passed to the view component (the seam is the sorted `snapshot.steps` in
   `WorkflowItemContent`, `swimlane-container.tsx`). A stale id in `H` matches no rendered
   step, so this is a no-op for it.

Both are mandatory **and interdependent**:

- If only the column were removed but its tasks kept, the board's orphan-remap
  (`remapOrphanTasks` / `useOrphanDisplay`) would re-key those now-column-less
  tasks into the "Needs Reassignment" column, resurfacing exactly the tasks the
  user asked to hide. **Hidden steps MUST have their tasks removed before
  orphan-remap runs.**
- If only the tasks were removed but the step kept, the column would render empty,
  violating the collapse requirement.

Because a hidden step is removed from the rendered `steps` array, the two
**same-workflow** move affordances that derive from that array lose the hidden step **by
construction, with no extra filtering**: drag-and-drop (its column is not rendered, so it
is not a drop target) and the **board card's** per-card "Move to" step menu
(`task-context-step-<stepId>`, whose current-workflow list is `moveTargetSteps` = the
collapsed `steps` prop set in `swimlane-kanban-content.tsx`). The multi-select
**bulk-move** step list is the one same-workflow **board** surface that does NOT derive
from the collapsed array — it is built by `useMultiSelectDerived` / `multiSelectSteps` in
`apps/web/components/kanban-board.tsx`, which reads the **raw** `kanbanMulti.snapshots[…].steps`
of the selection's workflow, not the collapsed `steps` prop — so Build MUST filter that
list explicitly (see [Move targets](#move-targets)). None of this extends to the
cross-workflow "Send to workflow → step" submenu (sources *other* workflows' steps
directly from the store) or to the sidebar / mobile task-switcher "Move to" step menu (a
non-board surface that also reads raw snapshots and shares the `task-context-step-<stepId>`
testid); both are out of scope (see [Move targets](#move-targets)).

Note on the render path: the live board renders through `SwimlaneContainer` for
**both** the single-workflow case (`workflowFilter` set) and the multi-workflow
case. `filteredTasks` returned by
`apps/web/hooks/domains/kanban/use-kanban-data.ts` is not on the column-render
path today; if that hook's task/step derivation is touched it SHALL stay
consistent with this contract, but conformance is judged on observable board
behaviour, not on which hook computes it.

Note on the pipeline (graph) view: it renders through the same `WorkflowItemContent`
`steps` prop, so removing a hidden step from that array applies identically. The
pipeline view has no "columns"; there, "collapse the column" means the hidden step's
lane/node is removed from the rendered graph, and its tasks are hidden the same way as
in the kanban view. Both views therefore satisfy the dual-filter contract from the one
seam.

## Move targets
A hidden step SHALL NOT be offered as a manual move destination **within its own
workflow** while it is hidden, on all three of these same-workflow **board** surfaces —
the surfaces reachable from the kanban board itself. Two hold by construction; one
requires an explicit filter:

- drag-and-drop — its column is not rendered, so it is not a drop target. **By
  construction** (derives from the collapsed `steps` array).
- the board card's per-card "Move to" step menu for the task's own workflow
  (`task-context-step-<stepId>`, rendered by `useKanbanCardMoveTargets` /
  `kanban-card.tsx`, whose current-workflow list is overridden to `moveTargetSteps` = the
  collapsed `steps` prop — `kanban-card-menu-items.tsx` sets
  `result[currentWorkflowId] = steps`). **By construction** (same collapsed array).
- the multi-select bulk-move step list (`bulk-move-step-<stepId>`). **Requires an explicit
  filter.** This list is built by `useMultiSelectDerived` / `multiSelectSteps` in
  `apps/web/components/kanban-board.tsx` from the **raw** `kanbanMulti.snapshots[…].steps`
  of the selection's workflow, NOT from the collapsed `steps` prop, so the collapse does
  not remove hidden steps here on its own. Build SHALL filter `multiSelectSteps` (or the
  toolbar's step list it feeds) against the hidden set of the selection's own workflow, so
  that no `bulk-move-step-<hiddenStepId>` entry is rendered for a step hidden in that
  workflow. This filter uses the SAME per-workflow hidden set as the task predicate; it
  introduces no new state.

To move a task into a hidden step in its own workflow, the user re-ticks the step first.
This keeps "hidden" meaning "absent from this board" rather than "invisible but still a
target".

**Explicitly out of scope — the cross-workflow "Send to workflow → step" submenu.**
The per-card menu can also reassign a task into a *different* workflow's step
(`useKanbanCardMoveTargets` in `apps/web/components/kanban-card-menu-items.tsx` builds
those other-workflow step lists straight from `kanbanMulti.snapshots[…].steps`, not
from any collapsed array). Those destination lists SHALL continue to show the target
workflow's full step set regardless of that workflow's hidden set. Rationale: the
hidden set is a per-board *display* filter for the board you are viewing; suppressing a
destination in another workflow because of a display preference on your current board
would be surprising, and cross-workflow reassignment is not "this board". A task's own
current step is never a member of another workflow, so this cannot resurface a
same-workflow hidden step.

**Explicitly out of scope — the sidebar and mobile task-switcher "Move to" step menus.**
A *fourth* same-workflow "Move to step" surface exists that is NOT part of the kanban
board: the sidebar task switcher (`apps/web/components/task/task-session-sidebar.tsx` →
`task-switcher-row.tsx` → `TaskItemWithContextMenu`) and its mobile counterpart
(`apps/web/components/task/mobile/session-task-switcher-sheet.tsx`). Both render
`TaskMoveContextMenuItems` (`apps/web/components/task/task-move-context-menu.tsx`), whose
current-workflow "Move to step" submenu sources its steps from the **raw**
`kanbanMulti.snapshots[…].steps` (via `useWorkspaceSidebarTasks` → `aggregateSidebarTasks`
in `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts`), NOT from the board's
collapsed `steps` prop, and emits the **same** `task-context-step-<stepId>` testid as the
board card menu. This surface SHALL continue to show the workflow's **full** step set
regardless of the hidden set — it is out of scope for this feature, for the same reason as
the cross-workflow submenu above: the hidden set is a *display* filter for the kanban
board you are viewing, and the sidebar/mobile task switcher is a navigation surface, not
"this board". Build SHALL NOT filter it, and MUST NOT rely on the collapse to remove
hidden steps there (it does not derive from the collapsed array). Consequently, any
conformance/E2E assertion on the absence of a `task-context-step-<hiddenStepId>` entry is
scoped to the **board card menu** (and the bulk-move toolbar for `bulk-move-step-*`), NOT
to the sidebar/mobile task-switcher menu (see [Scenarios](#scenarios-acceptance-criteria)).

## Ordering & determinism
- **Steps section (new UI):** within a workflow group, steps are ordered by ascending
  `position`, ties broken by ascending step `id` (lexicographic), so the checkbox list
  is fully deterministic when two steps share a position.
- **The board is left exactly as today.** The board continues to order columns by
  ascending `position` via the existing stable sort in `WorkflowItemContent`
  (`swimlane-container.tsx`); this feature adds NO id tiebreak there and does not
  otherwise reorder columns. This is deliberate: adding a board tiebreak would reorder
  tied-position columns even when nothing is hidden, breaking the "identical to the
  pre-feature board" guarantee (see [Scenarios](#scenarios-acceptance-criteria)). On the
  rare tied-position workflow, the board's left-to-right column order and the Steps
  section's top-to-bottom order MAY therefore differ; that is accepted and the board side
  is unchanged from today.
- **Workflow-group order in the Steps section** follows the board's workflow order
  (the order of `selectWorkflowSwimlanes`, which derives from `workflows.items`).
- The predicate is a pure set-membership test; it introduces no ordering of its
  own and does not reorder surviving columns or tasks.

## Idempotency & concurrency
- Toggling a step is idempotent per `(workflowId, stepId)`: unticking adds the id
  to the workflow's hidden set (no-op if already present); re-ticking removes it
  (no-op if already absent).
- Persisting is idempotent: if the normalised `hiddenWorkflowStepIds` equals the
  current value (order-insensitive per workflow), `isSettingsUnchanged` short-
  circuits and no write is sent (matching the existing filters).
- Two concurrent writers (e.g. two tabs) resolve **last-write-wins on the
  `kanban_hidden_step_ids` field**. The backend update request uses per-field
  pointers, so a write that omits this field does not clobber it; a write that
  includes it replaces it wholesale. This matches how the Repository and Workflow
  filters already behave. No merge of individual step ids across concurrent
  writers is attempted.

## Nil / empty / error / defaults / boundary
- **Default (fresh user, no map):** `hiddenWorkflowStepIds` is `{}`; the board is
  identical to today (every column and task shown).
- **Empty array for a workflow:** treated as "nothing hidden" and normalised away
  (key removed).
- **All steps of a workflow hidden** (assuming that workflow has **no orphan
  tasks** — for the orphan interaction see the next bullet):
  - In "All Workflows" view **while at least one other workflow still has visible
    tasks**, that workflow has zero visible tasks and is dropped from the swimlane
    list by the existing `getVisibleWorkflows` logic — the whole workflow
    disappears from the board (consistent with "a workflow with no visible tasks is
    not shown").
  - In single-workflow view, the swimlane renders with zero columns (an empty
    board region), not an error and not "No tasks yet".
- **Every eligible workflow filters to zero visible tasks in "All Workflows" view**
  (e.g. every step of every workflow hidden, or the last workflow with visible
  tasks is fully hidden): this is the existing empty-board fallback, unchanged by
  this feature. On desktop (`getVisibleWorkflows` sees `showEmptyBoard=false`) the
  board shows the existing **"No tasks yet"** empty-state message — the same result
  the Repository filter or search query already produce when they filter everything
  out; the hidden tasks are not gone, they are re-shown by re-ticking a step. On
  mobile kanban (`showEmptyBoard=true`) the board instead renders all workflows with
  zero columns. Neither is an error. This is the reason the "dropped by
  `getVisibleWorkflows`" claim above is scoped to the ≥1-visible-workflow case.
- **Orphan tasks interacting with the hidden set:** a task whose `workflowStepId`
  matches no step in its workflow's current snapshot is an *orphan* and is remapped
  by `remapOrphanTasks` into the synthetic "Needs Reassignment" column. Because task
  hiding uses `H ∩ liveStepIds` (see [dual-filter](#the-dual-filter-contract)), a
  hidden id that no longer exists does NOT hide such a task; the task orphan-remaps
  exactly as it would with an empty hidden set. Consequently, **if a workflow has
  orphan tasks, hiding all of its real steps does NOT yield zero columns — the
  "Needs Reassignment" column remains** (holding those orphans), and in "All
  Workflows" view the workflow is therefore still rendered (it has visible tasks).
  The "zero columns" and "identical to empty hidden set" statements elsewhere assume
  no orphan tasks are present.
- **Unknown / stale step id in the hidden set** (step deleted, or belongs to a
  workflow with no current snapshot): it matches no rendered step (column collapse
  is a no-op) and, per `H ∩ liveStepIds`, hides no task — so it is inert and never
  changes what renders, even for a task still pointing at the deleted id (that task
  orphan-remaps as it would with an empty set). Pruning stale ids is **not required**.
  See [Out of scope](#out-of-scope).
- **Newly added step** (created after the user last set the filter): absent from
  the hidden set, therefore shown by default.
- **Renamed step:** id is unchanged, so its hidden/shown state is preserved; the
  new title appears in the Steps section and (if shown) as the column header.

## Failure modes
- **Persist request fails** (WS and REST fallback both error): the UI keeps the
  user's in-memory selection for the session (the board reflects it immediately);
  the failure is swallowed/logged, exactly as the existing display-settings
  writes do. The selection may not survive the next cold load if every write
  failed — this is the same durability contract as the Workflow/Repository
  filters and is acceptable.
- **Snapshot for a workflow not yet loaded:** the workflow is **omitted** from the
  Steps section until its snapshot arrives (`selectWorkflowSwimlanes` returns only
  workflows with a loaded snapshot, so there is no group to show). No crash. Once the
  snapshot loads, the group appears and any persisted hidden ids for that workflow take
  effect. (This is the single chosen behaviour — not "show an empty group".)
- **Corrupt/missing persisted value:** hydration falls back to `{}` (all shown).

## Scenarios (acceptance criteria)

- **GIVEN** two workflows `A` and `B`, each with a step titled `Done` (different
  ids) that each holds a task, **WHEN** the user unticks `Done` in workflow `A`'s
  group, **THEN** workflow `A`'s `Done` column disappears and its task is hidden,
  **AND** workflow `B`'s `Done` column and task remain visible unchanged.

- **GIVEN** a workflow with a `Done` step that holds a task, **WHEN** the user
  unticks `Done`, **THEN** the `Done` column is removed from the board (not shown
  empty) **AND** the task does not reappear in a "Needs Reassignment" column.

- **GIVEN** a step has been unticked, **WHEN** the user re-ticks it, **THEN** the set
  of rendered column step-ids for that workflow, their left-to-right order, and the set
  of visible task-ids in each column all return to exactly what they were before the
  step was unticked (no column added or removed beyond the re-shown one, no task moved).

- **GIVEN** a step is unticked, **WHEN** the user reloads the page, **THEN** the
  step's column (`kanban-column-<stepId>`) and its task cards are still absent and the
  Steps-section checkbox for that step (`steps-filter-step-<stepId>`, see
  [Steps-section selectors](#steps-section-selectors)) is still unticked.

- **GIVEN** a step is unticked, **WHEN** the user opens that task's **board-card**
  "Move to" step menu for the task's own workflow, or the multi-select **bulk-move
  toolbar**, **THEN** no `task-context-step-<hiddenStepId>` / `bulk-move-step-<hiddenStepId>`
  entry is present. **AND** the cross-workflow "Send to workflow → step" submenu for a
  *different* workflow still lists that other workflow's steps unaffected by this
  workflow's hidden set. **AND** the **sidebar / mobile task-switcher** "Move to" step
  menu (a non-board surface) is out of scope: it MAY still list the hidden step and this
  AC does NOT assert its absence there (per [Move targets](#move-targets)). This
  absence assertion is therefore scoped strictly to the board card menu and the bulk-move
  toolbar, the two board surfaces whose `task-context-step-*` / `bulk-move-step-*` lists
  this feature controls.

- **GIVEN** no steps are hidden (`hiddenWorkflowStepIds` is `{}`), **WHEN** the board
  renders, **THEN** the set and order of rendered column step-ids and the set of visible
  task-ids per column are identical to a build without this feature, and every
  Steps-section checkbox is ticked.

- **GIVEN** a Repository filter is also active, **WHEN** a step is hidden, **THEN**
  the visible tasks are those that pass **both** filters (AND semantics), and
  hiding/showing a step leaves `repositoryIds` unchanged.

- **GIVEN** a workflow whose every step is unticked in "All Workflows" view (with at
  least one other workflow having visible tasks), **WHEN** the board renders, **THEN**
  that workflow's swimlane is not present in the DOM, while every other workflow renders
  the same columns and tasks it did before.

- **GIVEN** a single workflow (with no orphan tasks) is selected in the Workflow filter
  and every one of its steps is unticked, **WHEN** the board renders, **THEN** the
  swimlane renders zero columns (no `kanban-column-*` for that workflow) and shows neither
  an error nor the "No tasks yet" empty-state message, **AND** the Steps section still
  lists all of that workflow's steps unticked so they can be re-ticked.

- **GIVEN** "All Workflows" view and every eligible workflow filtered to zero visible
  tasks (all their steps hidden, and no orphan tasks), **WHEN** the board renders, **THEN**
  the board shows the existing "No tasks yet" empty-state on desktop (and renders all
  workflows with zero columns on mobile kanban) — an empty board, not an error — and every
  step re-appears once its Steps-section checkbox is re-ticked.

- **GIVEN** the hidden set for a workflow contains a step id that no longer exists in
  that workflow's snapshot, **WHEN** the board renders, **THEN** the set of rendered
  column step-ids and visible task-ids for that workflow is identical to rendering with
  that workflow's hidden set empty — including a task still pointing at the stale id, which
  orphan-remaps into "Needs Reassignment" exactly as it would with an empty hidden set (the
  stale id hides nothing because task hiding uses `H ∩ liveStepIds`).

## E2E requirement
This feature changes rendered UI under `apps/web/` and is **not** on the E2E
exemption allowlist, so a Playwright spec under `apps/web/e2e/` is **required**.
It SHALL assert real behaviour, seeded via the existing `apiClient` helpers
(`createWorkflow`, `listWorkflowSteps`, `createWorkflowStep`, `createTask`,
`saveUserSettings`) used by `apps/web/e2e/tests/kanban/workflow-filter.spec.ts`,
and drive the Display dropdown (`display-button`) the way that spec does. It SHALL
cover at minimum:

1. **Per-workflow isolation:** two workflows each with a same-titled step holding
   a task; unticking that step in workflow A hides A's column
   (`kanban-column-<stepId>` absent) and A's task, while B's same-titled step
   column and task remain visible. Uses `KanbanPage.columnByStepId` /
   `taskCardByTitle`.
2. **Persistence across reload:** untick a step, reload, and assert the column and
   its tasks remain hidden and the checkbox (`steps-filter-step-<stepId>`, see
   [Steps-section selectors](#steps-section-selectors)) is still unticked.

(The spec author recommends also asserting no orphan/"Needs Reassignment" column
appears for the hidden step, since that is the dual-filter trap.)

## i18n
All new copy goes through `t()` with keys in the `kanban` namespace
(`apps/web/src/locales/en/kanban.json`); `pnpm run i18n:ratchet` fails on
hardcoded strings. Recommended keys (Build may adjust the values, not the
through-`t()` requirement): `"steps": "Steps"` (section label) and a short
description string for the section. Do not call `t()` at module scope. Step
titles and workflow names are user/domain data and are rendered verbatim, never
translated.

## Out of scope
- Any semantic/phase classification of steps (no reading or branching on
  `stage_type`).
- Any cross-workflow "status category" concept or matching steps by title.
- Any `task.state`-based hide shortcut.
- Changes to the plugin task-filter API (`registerTaskFilter`); this is the
  first-party equivalent, built with direct store access, not as a plugin.
- Pruning stale step ids from the persisted hidden set (inert; may be added later
  but is not required for correctness).
- A "hide/show all" bulk toggle for a workflow's steps (per-step toggles only in
  this iteration).

## Pinned decisions (locked with the user — do not re-open)
1. Hiding a step also collapses its now-empty column, not just its cards.
2. The selection persists in backend user display settings, like the
   Workflow/Repository filters — not session-only.
3. The same predicate applies to both the single-workflow board and the
   multi-workflow swimlane.
4. Manual, per-workflow, keyed on `workflowStepId`. No phase grouping, no
   name-matching across workflows, no `task.state` shortcut. Store the hidden set
   (unticked steps), keyed by workflow id, tracking step id not title.
