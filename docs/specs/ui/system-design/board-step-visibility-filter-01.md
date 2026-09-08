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
# Per-workflow column visibility on the kanban board System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-UI-BOARD-STEP-VISIBILITY-FILTER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-BOARD-STEP-VISIBILITY-FILTER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Revision R2 — 2026-08-10 (control relocation)

R1 put the control in the board's global **Display** dropdown as a "Steps" section
listing every eligible workflow's steps at once. R2 **moves the control onto the
swimlane header of the workflow it configures**, and renames it from a *filter* to
**Columns**.

Nothing about the board's resulting behaviour changes: the same tasks are hidden, the
same columns collapse, the same value is persisted under the same wire field, and the
same move-target rules apply. R2 changes **where the user configures it** and, as a
consequence, deletes the requirements that only existed to make a global list of N
workflows × S steps survivable.

Why the relocation, stated once:

- **Density is structural, not cosmetic.** A global list must offer every eligible
  workflow, so it grows as N × S. Reviewer feedback on PR
  [#2467](https://github.com/kdlbs/kandev/pull/2467)
  ([comment 1](https://github.com/kdlbs/kandev/pull/2467#issuecomment-5233457869),
  [comment 2](https://github.com/kdlbs/kandev/pull/2467#issuecomment-5233467664))
  reported the section rendering ~381×981px. Collapsible groups would treat the
  symptom; a per-lane control removes the cause, because a lane has exactly one
  workflow.
- **The control is miscategorised.** Hiding a column is not a predicate on tasks — it
  is a property of how *this board* is laid out. It belongs next to the board, with the
  other per-lane controls (collapse, multi-select), not among the Workflow/Repository
  task filters.
- **Adjacency is the discoverability mechanism.** You configure a lane's columns from
  that lane. No section label, expand affordance, or shown-count summary has to explain
  which workflow a checkbox belongs to.

What R2 deletes from R1 outright, because a per-lane control cannot produce the
problems they addressed: the eligible-vs-rendered workflow distinction, per-workflow
group containers and headers, disclosure/expansion state of any kind, and the
"which Display surface owns the section" question. These are recorded under
[Out of scope](#out-of-scope) so they are not re-proposed.

**Delivery:** commit onto `feature/board-filter-per-wor-p2u` (PR #2467's head branch).
PR #2467 is **open and unmerged**, so this is a revision of unshipped work, not a
migration — no persisted value exists in the wild and no compatibility shim is needed.

As of 2026-08-10 the PR head (`2ddba494d`) already carries the work R2 supersedes:
`feat(kanban): per-workflow collapsible Steps section disclosure`,
`feat(kanban): add the Steps filter to the phone Display surface`, and
`docs(spec): fold R2 review follow-ups into the step-visibility-filter spec`. Those
three are reverted as part of this revision — the disclosure machinery and the
dropdown-based phone surface are exactly what the relocation removes. The selector
relocation commit (`refactor(kanban): move workflow swimlane selectors out of the
component layer`) is **kept**: it is independent layering hygiene and this spec's
`Lane reachability` rule touches the same file.

---

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
**per-workflow column show/hide keyed on the real `workflowStepId`**, configured
per workflow, from that workflow's own lane.

## What
- Each rendered workflow lane SHALL offer a **Columns** menu listing **that
  workflow's steps only**, as checkbox items in `position` order (tiebreak by `id`,
  see [Ordering](#ordering--determinism)).
- Every step SHALL be **shown (ticked) by default**. A step is hidden only when
  the user explicitly unticks it.
- Unticking a step SHALL, on every board surface that renders that workflow
  (single-workflow board and multi-workflow swimlane, kanban and pipeline views):
  1. **hide that step's tasks**, and
  2. **collapse (remove) that step's now-empty column** — not merely empty it.

  Both effects are required together (see
  [The dual-filter contract](#the-dual-filter-contract)).
- The selection SHALL be **scoped strictly per workflow id**: hiding step `S` in
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
- Column visibility SHALL **compose (AND)** with the Workflow filter, Repository
  filter, search query, and any plugin task filters: a task is visible only if it
  passes all of them.
- The board's global **Display** dropdown SHALL NOT gain a Steps section
  (see [Out of scope](#out-of-scope)).

## Control surface
The control has exactly one shape — *one workflow's steps, as checkbox items* — and
two homes, chosen by which of them is on screen. Both render the **same**
presentational component, and neither ever shows more than one workflow.

### Desktop and tablet — the swimlane header
- The menu SHALL be rendered by `SwimlaneHeader`
  (`apps/web/components/kanban/swimlane-header.tsx`), which
  `SwimlaneSection` renders once per workflow lane, alongside the existing
  collapse and multi-select controls.
- `shouldHideHeaders` (`apps/web/components/kanban/swimlane-container.tsx`)
  returns `false` whenever the viewport is not mobile, so on desktop and tablet the
  lane header is present in **every** board mode — All-Workflows swimlanes and the
  single-workflow board alike. No viewport branch, no second desktop surface.
- The trigger SHALL be reachable when the lane is **collapsed** as well as expanded:
  `SwimlaneSection` renders the header outside its `!isCollapsed` guard, and R2 SHALL
  NOT move it inside.

### Phone — the mobile menu drawer, scoped to the focused workflow
- On a phone the lane header is suppressed (`shouldHideHeaders` returns `true` for
  mobile kanban, an active workflow filter, or a single workflow), so the phone needs
  its own home or a desktop-hidden column becomes unrecoverable from a phone.
- The phone board is already **single-workflow focused** — `getRenderedWorkflows`
  narrows to `focusedWorkflowId` and the `mobile-board-navigator` switches between
  workflows. The phone control SHALL therefore offer **exactly the focused workflow's
  steps**, which is the same one-workflow shape as the lane menu.
- It SHALL render inside the existing `MobileMenuSheet` → `MobileDisplayOptions`
  block (`apps/web/components/kanban/mobile-menu-sheet.tsx`), after the Repository
  field and before the Preview-panel field, using the `mobileFieldClass` /
  `mobileFieldLabelClass` tokens from `mobile-menu-styles.ts`.
- IF `currentPage !== "kanban"`, THEN neither home renders it.
- The drawer's existing `min-h-0 flex-1 overflow-y-auto overscroll-contain` region
  remains the single scroll owner. R2 SHALL NOT add a `max-height`, a fixed height,
  or a nested scroll container on either home.
- Every interactive row SHALL measure **≥ 44 CSS px** on a phone viewport, matching
  the drawer's List-rows field (`flex min-h-11 …`), not its Preview-panel row
  (`flex h-10`, 40px). Long step titles SHALL truncate to a single line with an
  ellipsis (Tailwind `truncate`) and carry the full title in a `title` attribute;
  `document.documentElement.scrollWidth` SHALL NOT exceed its `clientWidth`.

### Exactly one home at a time
At most one of the two renders the control at any breakpoint, by construction: the
lane header is suppressed exactly where the drawer takes over. Build SHALL NOT render
both. The control is stateless apart from the persisted hidden set; the phone lane
focus is published through the existing global mobile-kanban focus bridge so the
drawer and board address the same workflow without maintaining a second local copy.

## Lane reachability (the recoverability rule)
Because the control lives on the lane, the lane must survive the state the control
creates. R1 solved this by making the global list offer *eligible* rather than
*rendered* workflows; R2 solves it at the render seam instead, with one rule:

- **A workflow with a non-empty live hidden set SHALL NOT be dropped from the board.**
  `selectVisibleWorkflows` (`apps/web/lib/kanban/workflow-swimlanes.ts`)
  currently drops, in All-Workflows view, every workflow whose filtered task list is
  empty. It SHALL additionally retain any workflow for which
  `hiddenWorkflowStepIds[workflowId] ∩ liveStepIds` is non-empty, so that workflow's
  lane — and therefore its Columns menu — remains on the board even when hiding its
  steps left it with zero visible tasks.
- A retained-but-empty lane renders its header and zero or more columns. It is not an
  error and does not render the "No tasks yet" empty state.
- A workflow with an **empty** hidden set is unaffected: it is dropped or kept exactly
  as it is today. This feature does not otherwise change which workflows render.
- On the phone, reachability is provided by the focus navigator rather than by this
  rule: the user selects the workflow, then opens the drawer.

The invariant this buys, stated as a single testable sentence: **any hidden column can
be restored from the same surface that hid it, without changing any other filter.**

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
MAY build a `Set` per workflow for O(1) membership; the "hidden set" in this design
refers to that membership semantics, realised on the wire as a `string[]`.

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
them. **R2 changes nothing here** — the relocation is presentational.

- **DB / wire (snake_case): `kanban_hidden_step_ids`**, typed as a JSON object
  `map[string][]string` (workflowId → hidden stepIds).
  - `apps/backend/internal/user/models/models.go` — `UserSettings.KanbanHiddenStepIDs map[string][]string` (`json:"kanban_hidden_step_ids"`).
  - `apps/backend/internal/user/dto/dto.go` — add to `UserSettingsDTO` (value) and `UpdateUserSettingsRequest` (as `*map[string][]string`, `,omitempty`, so an absent field is not treated as "clear"); map it in `FromUserSettings` (the `*models.UserSettings → UserSettingsDTO` mapper — cited by symbol, not line, because the function has drifted).
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
  `hiddenWorkflowStepIds`; the default is `{}`.
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

### Toggle action
Toggling SHALL go through a single `onToggleStepVisibility(workflowId, stepId)`
exposed by `useKanbanDisplaySettings` (`apps/web/hooks/use-kanban-display-settings.ts`),
used unchanged by both homes. Neither home owns persistence.

## Menu contents & selectors
- The menu's step list comes from that workflow's snapshot `steps`
  (`kanbanMulti.snapshots[workflowId].steps`) — the same source the board renders —
  in `position` order (see [Ordering](#ordering--determinism)).
- The synthetic **"Needs Reassignment"** orphan column (`ORPHAN_STEP_ID`,
  `apps/web/components/kanban/swimlane-kanban-content.tsx`) is a display-only
  fallback and is NOT a real step: it SHALL NOT appear in the menu and SHALL NOT be
  hideable.
- The menu SHALL be built from `@kandev/ui/dropdown-menu`'s
  `DropdownMenuCheckboxItem`, whose shared `DropdownMenuContent` already caps to the
  viewport and scrolls internally. No bespoke popover.

Stable `data-testid`s, matching the testid-precision of the rest of this spec
(`kanban-column-<stepId>`, `task-context-step-<stepId>`, `bulk-move-step-<stepId>`).
Build MAY choose the exact copy but NOT drop these ids:

- Menu trigger, one per lane: `data-testid="columns-menu-<workflowId>"`, with
  `aria-expanded` reflecting open state. On the phone home the trigger is the drawer's
  own field control and carries the same id for the focused workflow.
- Each step's checkbox item: `data-testid="columns-menu-step-<stepId>"`, reflecting
  checked state through the control's standard semantics (`aria-checked` /
  `data-state`) so a test can assert ticked vs unticked. The item **is** the
  interactive row, so it is both the state locator and the geometry locator measured by
  the ≥ 44px assertion — no second id is needed.

Step ids are the real `workflowStepId`, never the title. `ORPHAN_STEP_ID` has no
control and therefore no such testid.

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
from the collapsed array — see [Move targets](#move-targets).

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
seam — and both render `SwimlaneHeader`, so both carry the Columns menu.

## Move targets
A hidden step SHALL NOT be offered as a manual move destination **within its own
workflow** while it is hidden, on all three of these same-workflow **board** surfaces.
Two hold by construction; one requires an explicit filter:

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
hidden set is a per-board *display* preference for the board you are viewing;
suppressing a destination in another workflow because of a display preference on your
current board would be surprising. A task's own current step is never a member of
another workflow, so this cannot resurface a same-workflow hidden step.

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
regardless of the hidden set — it is a navigation surface, not "this board". Build SHALL
NOT filter it, and MUST NOT rely on the collapse to remove hidden steps there (it does
not derive from the collapsed array). Consequently, any conformance/E2E assertion on the
absence of a `task-context-step-<hiddenStepId>` entry is scoped to the **board card
menu** (and the bulk-move toolbar for `bulk-move-step-*`), NOT to the sidebar/mobile
task-switcher menu.
