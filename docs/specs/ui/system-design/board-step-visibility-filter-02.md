---
status: draft
system: ui
requirements:
  - REQ-UI-BOARD-STEP-VISIBILITY-FILTER-001
created: 2026-08-09
updated: 2026-08-10
owners:
  - nova28
---
# Per-workflow column visibility on the kanban board System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-UI-BOARD-STEP-VISIBILITY-FILTER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-BOARD-STEP-VISIBILITY-FILTER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Ordering & determinism
- **Columns menu:** steps are ordered by ascending `position`, ties broken by ascending
  step `id` (lexicographic), so the checkbox list is fully deterministic when two steps
  share a position. The order is identical on both homes.
- **The board is left exactly as today.** The board continues to order columns by
  ascending `position` via the existing stable sort in `WorkflowItemContent`
  (`swimlane-container.tsx`); this feature adds NO id tiebreak there and does not
  otherwise reorder columns. This is deliberate: adding a board tiebreak would reorder
  tied-position columns even when nothing is hidden, breaking the "identical to the
  pre-feature board" guarantee (see [Scenarios](#scenarios-acceptance-criteria)). On the
  rare tied-position workflow, the board's left-to-right column order and the menu's
  top-to-bottom order MAY therefore differ; that is accepted and the board side is
  unchanged from today.
- The predicate is a pure set-membership test; it introduces no ordering of its
  own and does not reorder surviving columns or tasks.
- The mobile board navigator's `column-tab-*` indices are positional over the
  already-collapsed `steps` array, so hiding a step renumbers the remaining tabs. That is
  pre-existing behaviour; assertions use step **titles**, never a fixed index.

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
- The two homes cannot race: at most one renders at any breakpoint, and the only
  transient coordination value is the shared mobile-kanban focus. IF the viewport
  crosses the 768px boundary while a toggle's persist request is in flight, THEN the
  request completes normally — it is owned by `useUserDisplaySettings`, not by either
  home — and the newly mounted home renders from the store. No debounce and no
  cancellation are introduced.

## Nil / empty / error / defaults / boundary
- **Default (fresh user, no map):** `hiddenWorkflowStepIds` is `{}`; the board is
  identical to today (every column and task shown, every menu item ticked).
- **Empty array for a workflow:** treated as "nothing hidden" and normalised away
  (key removed).
- **All steps of a workflow hidden:** its hidden set is non-empty, so per
  [Lane reachability](#lane-reachability-the-recoverability-rule) the lane is retained.
  It renders its header (with the Columns menu, every item unticked) and zero columns —
  or, if the workflow has orphan tasks, only the "Needs Reassignment" column. This is
  not an error and does not render "No tasks yet".
- **Every workflow on the board has all its steps hidden:** every lane is retained by
  the same rule, each rendering a header and zero columns. The board is visibly empty of
  cards but every Columns menu is present, so the state is fully reversible. The
  pre-existing "No tasks yet" empty state is NOT shown in this case, because
  `selectVisibleWorkflows` returned a non-empty list.
- **Workflow with zero visible tasks and an EMPTY hidden set:** dropped in
  All-Workflows view exactly as today. Unchanged by this feature.
- **Orphan tasks interacting with the hidden set:** a task whose `workflowStepId`
  matches no step in its workflow's current snapshot is an *orphan* and is remapped
  by `remapOrphanTasks` into the synthetic "Needs Reassignment" column. Because task
  hiding uses `H ∩ liveStepIds`, a hidden id that no longer exists does NOT hide such a
  task; the task orphan-remaps exactly as it would with an empty hidden set.
- **Unknown / stale step id in the hidden set** (step deleted, or belongs to a
  workflow with no current snapshot): it matches no rendered step (column collapse
  is a no-op) and, per `H ∩ liveStepIds`, hides no task — so it is inert and never
  changes what renders. It also does NOT trigger the lane-retention rule, which tests
  `H ∩ liveStepIds`, not `H`. Pruning stale ids is **not required**.
- **Newly added step** (created after the user last set the filter): absent from
  the hidden set, therefore shown by default.
- **Renamed step:** id is unchanged, so its hidden/shown state is preserved; the
  new title appears in the menu and (if shown) as the column header.
- **Workflow with zero steps:** its Columns menu renders with no checkbox items. The
  menu trigger is still present so the surface is never silently absent.
- **`currentPage === "tasks"`:** no Columns control on either home.

## Failure modes
- **Persist request fails** (WS and REST fallback both error): the UI keeps the
  user's in-memory selection for the session (the board reflects it immediately);
  the failure is swallowed/logged, exactly as the existing display-settings
  writes do. The selection may not survive the next cold load if every write
  failed — this is the same durability contract as the Workflow/Repository
  filters and is acceptable.
- **Snapshot for a workflow not yet loaded:** that workflow has no lane, therefore no
  Columns menu, until its snapshot arrives. No crash. Once the snapshot loads, the lane
  appears and any persisted hidden ids for that workflow take effect.
- **Corrupt/missing persisted value:** hydration falls back to `{}` (all shown).

## Scenarios (acceptance criteria)

- **GIVEN** two workflows `A` and `B`, each with a step titled `Done` (different
  ids) that each holds a task, **WHEN** the user unticks `Done` in workflow `A`'s
  Columns menu, **THEN** workflow `A`'s `Done` column disappears and its task is hidden,
  **AND** workflow `B`'s `Done` column and task remain visible unchanged, **AND**
  workflow `B`'s Columns menu still shows its `Done` ticked.

- **GIVEN** a workflow with a `Done` step that holds a task, **WHEN** the user
  unticks `Done`, **THEN** the `Done` column is removed from the board (not shown
  empty) **AND** the task does not reappear in a "Needs Reassignment" column.

- **GIVEN** a step has been unticked, **WHEN** the user re-ticks it, **THEN** the set
  of rendered column step-ids for that workflow, their left-to-right order, and the set
  of visible task-ids in each column all return to exactly what they were before the
  step was unticked (no column added or removed beyond the re-shown one, no task moved).

- **GIVEN** a step is unticked, **WHEN** the user reloads the page, **THEN** the
  step's column (`kanban-column-<stepId>`) and its task cards are still absent and the
  menu item for that step (`columns-menu-step-<stepId>`) is still unticked.

- **GIVEN** a step is unticked, **WHEN** the user opens that task's **board-card**
  "Move to" step menu for the task's own workflow, or the multi-select **bulk-move
  toolbar**, **THEN** no `task-context-step-<hiddenStepId>` / `bulk-move-step-<hiddenStepId>`
  entry is present. **AND** the cross-workflow "Send to workflow → step" submenu for a
  *different* workflow still lists that other workflow's steps unaffected by this
  workflow's hidden set. **AND** the **sidebar / mobile task-switcher** "Move to" step
  menu is out of scope: it MAY still list the hidden step and this AC does NOT assert
  its absence there (per [Move targets](#move-targets)).

- **GIVEN** no steps are hidden (`hiddenWorkflowStepIds` is `{}`), **WHEN** the board
  renders, **THEN** the set and order of rendered column step-ids, the set of visible
  task-ids per column, and the set of rendered swimlanes are identical to a build
  without this feature, and every Columns menu item is ticked.

- **GIVEN** a Repository filter is also active, **WHEN** a step is hidden, **THEN**
  the visible tasks are those that pass **both** filters (AND semantics), and
  hiding/showing a step leaves `repositoryIds` unchanged.

- **(R2) GIVEN** "All Workflows" view and a workflow whose every step is unticked so it
  has zero visible tasks, **WHEN** the board renders, **THEN** that workflow's swimlane
  is still present with its header and its `columns-menu-<workflowId>` trigger, **AND**
  re-ticking a step from that menu restores its column and tasks — the state is
  reversible from the surface that created it.

- **(R2) GIVEN** "All Workflows" view and a workflow with zero visible tasks whose
  hidden set is **empty**, **WHEN** the board renders, **THEN** that workflow's swimlane
  is absent, exactly as before this feature — the retention rule keys on a non-empty live
  hidden set, not on emptiness of the lane.

- **(R2) GIVEN** a desktop viewport, **WHEN** the Workflow filter selects a single
  workflow, **THEN** that workflow's lane header and `columns-menu-<workflowId>` trigger
  are rendered (`shouldHideHeaders` returns `false` off mobile), so the control is
  reachable in single-workflow view without any Display-dropdown entry.

- **(R2) GIVEN** a phone viewport with workflow `A` focused in the board navigator,
  **WHEN** the user opens the mobile menu drawer, **THEN** the Columns block lists
  exactly `A`'s steps and no other workflow's, **AND** unticking one hides that column
  from the phone board.

- **GIVEN** the hidden set for a workflow contains a step id that no longer exists in
  that workflow's snapshot, **WHEN** the board renders, **THEN** the set of rendered
  column step-ids and visible task-ids for that workflow is identical to rendering with
  that workflow's hidden set empty, **AND** the stale id does not by itself retain a lane
  that would otherwise be dropped.

## E2E requirement
This feature changes rendered UI under `apps/web/` and is **not** on the E2E
exemption allowlist, so a Playwright spec under `apps/web/e2e/` is **required**.
It SHALL assert real behaviour, seeded via the existing `apiClient` helpers
(`createWorkflow`, `listWorkflowSteps`, `createWorkflowStep`, `createTask`,
`saveUserSettings`) used by `apps/web/e2e/tests/kanban/workflow-filter.spec.ts`.

`apps/web/e2e/tests/kanban/step-visibility-filter.spec.ts` (chromium) SHALL be
retargeted from `display-button` to `columns-menu-<workflowId>` and SHALL cover at
minimum:

1. **Per-workflow isolation:** two workflows each with a same-titled step holding
   a task; unticking that step in workflow A's Columns menu hides A's column
   (`kanban-column-<stepId>` absent) and A's task, while B's same-titled step
   column and task remain visible. Uses `KanbanPage.columnByStepId` /
   `taskCardByTitle`.
2. **Persistence across reload:** untick a step, reload, and assert the column and
   its tasks remain hidden and `columns-menu-step-<stepId>` is still unticked.
3. **(R2) Lane retention:** hide every step of a workflow that has tasks, in
   All-Workflows view with another workflow still populated, and assert the lane and
   its `columns-menu-<workflowId>` trigger are still present and a re-tick restores the
   column. This is the recoverability invariant and the one behaviour the relocation
   depends on.

A mobile spec `apps/web/e2e/tests/kanban/mobile-step-visibility-filter.spec.ts`
(mobile-chrome project, per the `mobile-*.spec.ts` convention in
`apps/web/e2e/README.md`) SHALL open the mobile menu drawer, toggle a step of the
focused workflow, verify the phone board result, and assert row height ≥ 44px on
`columns-menu-step-<stepId>` and no horizontal overflow.

Vitest coverage SHALL include: the hidden-set normalisation in
`use-user-display-settings.test.ts`, the retention rule in
`workflow-swimlanes.test.ts` (or the file that owns `selectVisibleWorkflows` after any
move), the bulk-move filter, and the shared menu component's ordering and checked
state.

## i18n
All new copy goes through `t()` with keys in the `kanban` namespace
(`apps/web/src/locales/en/kanban.json`); `pnpm run i18n:ratchet` fails on
hardcoded strings. Recommended keys (Build may adjust the values, not the
through-`t()` requirement): `"columns": "Columns"` for the trigger and menu label, and
an accessible label naming the workflow for the per-lane trigger. Do not call `t()` at
module scope. Step titles and workflow names are user/domain data and are rendered
verbatim, never translated. R1's `"steps"` key and its section description are removed
if nothing else references them.

## Out of scope
- **A Steps section in the global Display dropdown.** Superseded by the per-lane
  Columns menu; do not re-add. Everything it required — the eligible-vs-rendered
  workflow distinction, per-workflow group containers, group headers, shown-count
  summaries, collapsible disclosure, an ephemeral override map, and cross-surface
  disclosure lifetime rules — is out of scope with it.
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
- Unifying the board's display settings with the sidebar's saved-view filter model
  (`sidebarViews` / `applyView`). That convergence is a separate direction and gets its
  own spec; this one does not depend on it and does not block it.

## Pinned decisions (locked with the user — do not re-open)
1. Hiding a step also collapses its now-empty column, not just its cards.
2. The selection persists in backend user display settings, like the
   Workflow/Repository filters — not session-only.
3. The same predicate applies to both the single-workflow board and the
   multi-workflow swimlane, and to both the kanban and pipeline views.
4. Manual, per-workflow, keyed on `workflowStepId`. No phase grouping, no
   name-matching across workflows, no `task.state` shortcut. Store the hidden set
   (unticked steps), keyed by workflow id, tracking step id not title.
5. **(R2)** The control lives on the workflow it configures — the swimlane header on
   desktop/tablet, the focused-workflow block in the phone drawer — and never in a
   global list spanning workflows.
6. **(R2)** A workflow with a non-empty live hidden set is never dropped from the
   board, so the control that hid a column can always restore it.
