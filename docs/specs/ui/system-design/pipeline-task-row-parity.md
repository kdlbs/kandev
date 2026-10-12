---
status: draft
system: ui
requirements:
  - REQ-UI-PIPELINE-ROW-001
  - REQ-UI-PIPELINE-ROW-002
  - REQ-UI-PIPELINE-ROW-003
  - REQ-UI-PIPELINE-ROW-004
  - REQ-UI-PIPELINE-ROW-005
---

# Pipeline Task Row Parity System Design

> **Revision (2026-09-03):** REQ-UI-PIPELINE-ROW-001 was revised after this
> design shipped: collapsed (unlabelled) past/future markers proved illegible.
> Every step now renders its own pill; no-growth comes from scrolling the step
> run, anchored to the current step, instead of collapsing. See the
> requirements doc's REQ-UI-PIPELINE-ROW-001 revision for current ACs; sections
> below describing the collapsed-marker mode document the superseded design.

## Purpose and boundaries

The UI system owns this design because every outcome is a presentation and
interaction contract over task state the Tasks system already publishes. The row
reads `Task`, `WorkflowStep`, workspace repositories, and integration
availability; it writes nothing new.

Adjacent contracts this design uses but does not own:

- **Task and workflow state**, including `workflowStepId`, `position`,
  `blockedReason`, `queuedForStepId`, review state, and repository associations.
- **The move request path**, owned by `useSwimlaneMove`. That hook's failure
  path is under change here, not adjacent to it.
- **The plugin host**, whose `task-card-indicators` and `registerTaskMenuAction`
  contributions the row must render without change. Its `task-card-tags`
  contribution stays a Kanban card surface (AC-UI-PIPELINE-ROW-003.6).
- **Integration availability**, owned by the GitLab, Jira, Linear, and Sentry
  slices behind `useKanbanExternalLinkAvailability`.
- **Adaptive board sizing**, owned by
  [Adaptive Kanban](../requirements/adaptive-kanban.md), which establishes the
  board surface as the width that matters.

## Dependency on card 09404bae

Card 09404bae ("Fix pipeline row: dead preview prop, off-screen menu") lands
first and owns two changes this design builds on.

**Pinning (assumed, not branched).** 09404bae makes the actions cluster remain
visible at every horizontal scroll offset. This design assumes a pinned cluster
and specifies only what it contains. The single consequence for us is that the
cluster's width is unavailable to the step run, so "Width budget" subtracts it;
that subtraction holds under any pinning technique.

**The click decision (branched).** 09404bae decides what a plain click on the
row does when "Open preview on click" is enabled. Two branches are possible and
this design absorbs either without a redesign:

- **Branch A: click previews.** The row needs a separate affordance to reach the
  full page, mirroring `components/kanban/virtualized-column-task-list.tsx`,
  which wires `onOpenFullPage={onOpenTask}` on the Kanban card and renders it
  under `showMaximizeButton`. That affordance is a member of the actions
  cluster, costing roughly 28px of row width.
- **Branch B: click navigates.** No full-page affordance is needed; the cluster
  is roughly 28px narrower.

**What we do about it.** The actions cluster is an ordered array of optional
members (AC-UI-PIPELINE-ROW-003.8). A full-page affordance is one such member;
whether 09404bae renders it changes the array's contents, not this design. The
width budget reserves its width, so Branch A is the costed case. Nothing else
here reads the click decision: step pills are not click targets
(AC-UI-PIPELINE-ROW-001.9), the menu suppresses row activation
(AC-UI-PIPELINE-ROW-002.7), and the disclosure surface neither previews nor
navigates (AC-UI-PIPELINE-ROW-004.5).

**The one integration point.** There is no inheritance path today:
`Graph2StepNode` owns a self-contained `handleClick` calling
`router.push(linkToTask(task.id))` with no row-level prop feeding it, which is
the coupling 09404bae's dead-`onPreviewTask` fix exists to remove. This design
therefore assumes the pill inherits nothing and instead states the invariant
09404bae must satisfy and we must not break: **the row has exactly one
activation decision, and the title and the current-step pill both route through
it.** Whichever handler 09404bae installs at the row level is the single handler
both call; neither keeps a private `router.push`. If 09404bae lands first,
delete `Graph2StepNode`'s local `handleClick` and take the row handler as a
prop; if this work lands first, route both through one row-level handler still
calling `linkToTask(task.id)`. Either order ends at the same shape.

**If 09404bae has not landed** when this work starts, implement against the
current row and treat the cluster's position and the pill's click handler as
that card's. Do not re-fix either; a conflicting second fix is worse than a late
one.

## Prior art

**Leg 1 (our own prior reasoning).** Five QMD `wiki` queries on row density,
progressive disclosure and board card affordances returned nothing usable: hits
used "pipeline" and "card" in unrelated senses, leaving no prior position to
defer to.

**Leg 2 (what others shipped).** **Warp vertical tabs** is the closest shipped
analogue: compact single-line default, status badges on the row icon, a `+ N
more` overflow row, and a hover sidecar showing full un-clipped metadata.
**Warp Factories** groups work by stage, which is our Kanban view. **Augment
Code** and Warp encode status as icons, not text.

**What we do differently.** Nothing in the corpus renders a per-row stage
pipeline. We adopt the hover sidecar (REQ-UI-PIPELINE-ROW-004) and
status-as-icon density (REQ-UI-PIPELINE-ROW-003) because our Kanban card
implements both, and reject configurable density. We depart from Warp on
keyboard access: its sidecar is pointer-only, whereas
AC-UI-PIPELINE-ROW-001.4 through 001.6 keep every step name reachable without
one.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-PIPELINE-ROW-001` | [Step marker model](#step-marker-model), [Width budget](#width-budget), [Control flow](#control-flow) |
| `REQ-UI-PIPELINE-ROW-002` | [Why extraction and not direct reuse](#why-extraction-and-not-direct-reuse), [New components](#new-components) |
| `REQ-UI-PIPELINE-ROW-003` | [New components](#new-components), [Control flow](#control-flow), [Nesting constraint](#nesting-constraint), [Width budget](#width-budget) |
| `REQ-UI-PIPELINE-ROW-004` | [Existing components under change](#existing-components-under-change) |
| `REQ-UI-PIPELINE-ROW-005` | [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

This is a frontend-only design under `apps/web/`. No backend component changes.

### Existing components under change

- **`components/kanban/graph2-task-pipeline.tsx`** (`Graph2TaskPipeline`,
  `TaskButton`, `TaskActions`, `PipelineStepNodes`). Becomes the row's
  composition root: title with disclosure, repository chips, inline status,
  step run, actions cluster. Its current `TaskActions` (archive and delete only)
  is replaced by the shared menu.
- **`components/kanban/graph2-step-node.tsx`** (`Graph2StepNode`). Gains a
  labelled past/future pill mode and a hidden-step marking. Its unused
  `onPreviewTask` declaration is 09404bae's, not ours.
- **`components/kanban/swimlane-graph2-content.tsx`** (`SwimlaneGraph2Content`).
  Gains the deterministic sort tiebreak, resolves workspace-scoped values once,
  and stops discarding `onEditTask` and `onSelectRange`.
- **`hooks/domains/kanban/use-swimlane-move.ts`** (`useSwimlaneMove`). Its
  failure path is a defect against AC-UI-PIPELINE-ROW-005.3. It captures
  `originalTasks` from the snapshot read *before* the optimistic patch, then on
  failure writes
  `setWorkflowSnapshot(workflowId, { ...currentSnapshot, tasks: originalTasks })`,
  replacing the whole task array, so every task updated between the request and
  its failure is reverted along with the moved one. The rollback becomes
  per-task compare-and-swap: on failure restore only the moved task's entry, by
  id, and only while the store still holds the exact optimistic value this move
  wrote. If anything else holds it, an out-of-band update superseded this move
  and the rollback is skipped. The restored value is the pre-move step, which is
  the last step the server reported for that task, so no refetch is needed.
- **`components/task/task-title-hover-card.tsx`** (`TaskTitleHoverCard`). Today
  it early-returns `<>{children}</>` when the task has no subtasks. That
  condition becomes AC-UI-PIPELINE-ROW-004.3's: open when the task has a
  description, a parent, or at least one subtask, or when the title is visually
  truncated at its rendered width. It is deliberately not "has any content":
  every task has a title, so that reading would put a popover on every Kanban
  card, including short-titled tasks that show nothing extra.

  **Who measures truncation.** Not this component. Its props today are
  `taskId`, `title`, `children`, `side`, `align` and `triggerClassName`, and it
  early-returns before rendering a trigger it could measure, so it cannot own the
  measurement. Each call site measures its own rendered title element, comparing
  that element's `scrollWidth` against its `clientWidth` under a
  `ResizeObserver`, and passes the result as a new boolean prop. The guard
  becomes `!isFinePointer || (!hasContent && !isTitleTruncated)`. That keeps the
  measurement where the element is, lets the same task qualify on the row and
  not on the card, and closes a truncation-only popover when a widening board
  untruncates the title. `isFinePointer` is unchanged. The component gains
  description and parent content and stays shared with the Kanban card
  (AC-UI-PIPELINE-ROW-004.4).
- **`components/kanban-card.tsx`** (`KanbanCard`). Loses the private menu
  assembly to a new shared module and consumes that module instead. Its rendered
  output is unchanged (AC-UI-PIPELINE-ROW-002.6).

### New components

- **A shared task-menu module.** Owns what is today private to
  `kanban-card.tsx`: menu state, the seven dialog flags, move-target
  computation, link-dialog handlers, external-link handlers, the dialogs
  element, and the existing Radix dismissal workarounds. Exposes one hook
  returning menu entries plus handlers, and one component rendering the dialogs.
  Consumed by both `KanbanCard` and `Graph2TaskPipeline`.
- **A shared inline-status strip.** The ordered, optional-member sequence from
  AC-UI-PIPELINE-ROW-003.8. This is an extraction on the same terms as the menu,
  not an assembly from what is already public. `components/kanban-card-content.tsx`
  exports only `KanbanCardBody`, `renderTaskStatusIcon`, `renderSubagentCountChip`
  and `KanbanCardShell`; the pieces the row actually needs are module-private:
  `RepoChip`, `OverflowRepoTooltip`, `RepoChipRow`, `REPO_CHIPS_VISIBLE`,
  `KanbanCardBadges` (which renders blocked, queued-for-step, session count and
  review state as one inline block rather than as separable pieces),
  `BlockedBadge`, `hasCardBadges`, and `KanbanCardRelationship`. Of what that
  file exports, only the type `KanbanCardShellProps` is relevant here. The data
  resolver `resolveTaskRepositoryChips` is **not** in that file at all: it lives
  in `components/kanban-card-repositories.ts` and is re-exported through
  `kanban-card.tsx`, so the extraction moves it from there, not from the content
  module.

  Move those symbols into the shared strip module and have `KanbanCardBody`
  consume them from there, so both surfaces render the same components rather
  than two implementations that happen to agree. `KanbanCardBadges` is moved
  whole; the row is not to re-split blocked, queued, session count and review
  into fresh JSX, because that split is the fork. Session count is the one
  member the row seats outside the strip: it belongs to the information column
  (AC-UI-PIPELINE-ROW-003.12), at the row's more-than-zero threshold, so
  `KanbanCardBadges` takes a `hideSessionCount` flag the row passes and the card
  does not. That is one boolean on the shared component rather than a second
  badges implementation, and it keeps the row at one session count
  (AC-UI-PIPELINE-ROW-003.8). Kanban's rendered output is unchanged, on the
  same terms as AC-UI-PIPELINE-ROW-002.6 sets for the menu, and the shared-source
  test in [Observability](#observability) covers the indicators as well as the
  menu entries.

### Why extraction and not direct reuse

`components/kanban-card-menu-items.tsx` already exports
`buildKanbanCardMenuEntries` and `useKanbanCardMoveTargets`, so entries alone
are reusable today. Everything that makes those entries work is not:
`useKanbanCardMenus`, `useKanbanCardDialogState`,
`useKanbanCardMoveMenuActions`, `buildLinkDialogHandlers`,
`externalLinkHandlers`, and `KanbanCardDialogs` are module-private to
`kanban-card.tsx`. Calling the entry builder without them would mean
re-implementing dialog state, the detach flow, the deferred-dismissal
workarounds, plugin actions, and five dialogs a second time: the fork
AC-UI-PIPELINE-ROW-002.4 forbids.

`dispatchKanbanCardClick` is the exception and is **not** private: it is an
`export` marked `@internal` for testing, and `kanban-card-click.test.ts` already
imports it. It is also exactly the four-branch dispatch
AC-UI-PIPELINE-ROW-005.10 needs, covering the modifier toggle, the shift range
select, the multi-select toggle and the plain click, where
`Graph2TaskPipeline`'s current `handleTaskClick` has two. The row imports it as
it stands; no move, no fork, and its existing test keeps its import.

The extraction is a move, not a rewrite. `KanbanCard` keeps its current call
shape and rendered output; the moved code changes only in taking its inputs as
parameters instead of closing over `KanbanCard`'s scope.

## Data and contracts

### `ViewContentProps` gains no member

`ViewContentProps` in `lib/kanban/view-registry.ts` lacks `workspaceId`,
`externalLinkAvailability`, and `repositoryChips`, which the Kanban card
receives. The row obtains those three from the store rather than widening the
view contract, following the precedent already set by
`kanban-card-plugin-slots.tsx`, whose `TaskCardIndicators` and `TaskCardTags`
read `workspaceId` from the store themselves. No fourth store read is needed:
[Failure and recovery](#failure-and-recovery) shows why the step set the row
already receives is sufficient.

Two `ViewContentProps` members that `SwimlaneGraph2Content` currently destructures
away are wired: `onEditTask` reaches the menu's edit entry, and `onSelectRange`
reaches the shift-click path in AC-UI-PIPELINE-ROW-005.10.
`showMaximizeButton` belongs to 09404bae's click decision (Branch A) and is left
to that card.

`onDeleteTask` is declared as `(task: Task) => void` on `ViewContentProps` while
the pipeline declares an `opts?: { cascade?: boolean }` variant, reconciled only
by the `as ComponentType<ViewContentProps>` cast at registration. The shared menu
needs the cascade argument for its delete entry, so correct the existing
`onDeleteTask` declaration to the cascade-bearing signature and drop the cast.

This is inside the requirement's out-of-scope bullet, not a breach of it: that
bullet bars **adding members** to `ViewContentProps`, and this adds none. It
corrects one existing member's declared type to match the call site it already
has, which is why the cast exists. No member is added or removed, no runtime
behavior changes, and Kanban's `(task) => void` handler stays assignable to the
wider signature.

### Step marker model

Each entry in the step run carries: the step id, its title, and its phase
(`completed`, `current`, `future`). Every entry renders its own label.
Phase is derived from the entry's index in the displayed steps against the
current step's index in that same list, which is today's derivation. There is no
"hidden" flag on an entry, because no row can render with its current step
hidden; [Failure and recovery](#failure-and-recovery) proves that and gives the
one case where no entry is `current`, in which every step entry is `future` and
a single synthetic labelled entry precedes them. That entry carries no phase; a
connector renders between it and the first displayed step in the `future` form,
so a nine-step run in this state carries nine connectors, not eight
(AC-UI-PIPELINE-ROW-005.6).

### Width budget

Per row, at a 1280px board surface with 9 steps (AC-UI-PIPELINE-ROW-001.3):

| Element | Today | After |
| --- | --- | --- |
| Information column | 200px | 200px |
| Step run (9 steps) | 1370px | ~400px |
| Inline status strip | 0px | ~160px |
| Actions cluster | ~28px | ~56px |
| Padding and gaps | ~48px | ~48px |
| **Row total** | **~1646px** | **~864px** |

The information column's 200px is the truncation width
AC-UI-PIPELINE-ROW-001.3 measures at, so it is contract. Repository chips are
inside that column rather than beside it, so they cost the row no width of
their own. The step run
budget above is superseded by the REQ-UI-PIPELINE-ROW-001 revision: every step
is a 130px pill (not a collapsed marker), so a nine-step run is ~1170px plus
connectors and routinely exceeds what is left, which is exactly when
AC-UI-PIPELINE-ROW-001.8 has it scroll, anchored to the current step, instead
of compressing. The actions cluster is ~56px for Branch A's full-page
affordance, ~28px for Branch B.

**Past the budget.** The inline status strip's ~160px assumes no plugin
contribution, and plugin width is third-party and unbounded, so the budget can
be exceeded by input this design does not control: enough plugin indicators, or
a board surface below the 1280px minimum. AC-UI-PIPELINE-ROW-003.11 names the
terminus for that case and it is deliberately not "clip something". Once the
information column's lines are truncated and the step run is already scrolling,
the row's region between the column and the actions cluster becomes
horizontally scrollable, carrying the step run and the status strip together. The actions
cluster is pinned by card 09404bae and stays out of that scroll, so the menu
remains reachable at any width, which is the defect this whole contract exists
to avoid re-introducing. Nothing wraps, nothing is clipped, and no plugin
contribution is dropped; the row simply becomes a scrollable strip of fixed
height.

The mechanism is one scroll container whose *scope* widens, not a second
container added beside or inside the first. At the second stage that container
wraps the step run alone; at the terminus the same container wraps the status
strip and the step run together. A row therefore never holds two horizontal
scroll regions, which is what AC-UI-PIPELINE-ROW-003.11 requires and what keeps
the two stages observably different: an inner scroll region can never widen its
parent, so nesting one inside the other would make the terminus indistinguishable
from the stage before it. The container carries the repo's existing
`.scrollbar-hide` utility (`app/globals.css`, `scrollbar-width: none` plus the
`::-webkit-scrollbar` rule), so it reserves no scrollbar height and row height is
identical at every stage, including on platforms that render non-overlay
scrollbars.

## Control flow

**Render.** `SwimlaneGraph2Content` resolves the displayed steps and the
deterministically sorted task list once, resolves the workspace repository list
and external-link availability once, then renders one `Graph2TaskPipeline` per
task passing per-task props plus the two shared workspace values. Resolving them
at the list rather than in each row is the mechanism by which every row of one
list agrees about repositories and link availability
(AC-UI-PIPELINE-ROW-005.9): the criterion states the agreement, this states how
it is obtained.

**Row composition.** Left to right, per AC-UI-PIPELINE-ROW-003.8: the
information column (the title wrapped in the disclosure surface, the repository
chips, then relative time and session count), the step run, the inline status
strip, then the pinned actions cluster. Each status member renders `null` when
it has nothing to show, so absent state costs no width.

The column is a fixed `w-[200px] shrink-0`, never a flex basis: a basis yields
to its own row's content, which is how the run's starting x came to differ row
to row (AC-UI-PIPELINE-ROW-003.13). The strip follows the run for the same
reason. It is the one part of the row whose width is genuinely per-task and,
through plugin indicators, unbounded, so anything seated after it inherits that
variance; the run flexes, settling the strip against the actions cluster. The
repository line reserves its height whether or not the task has a repository,
so a repo-less row matches its neighbours (AC-UI-PIPELINE-ROW-005.8).

**Step run.** Every displayed step renders its own labelled pill; the current
one also carries its move controls on hover or focus, and past and future pills
stay non-focusable. A visually hidden text node on the row carries the
accessible summary required by AC-UI-PIPELINE-ROW-001.6.

**Overflow.** Two overflow policies apply to different parts of the row and must
not be confused. The step run scrolls (AC-UI-PIPELINE-ROW-001.8). Everything
else yields width in the fixed order of AC-UI-PIPELINE-ROW-003.11: each line of
the information column truncates within the column's fixed width, then the step
run begins to scroll, then the region between the column and the actions
cluster scrolls as the terminus described under
[Width budget](#width-budget), by widening that same scroll container rather than
adding a second one. Repository chips are deliberately **not** a stage:
`REPO_CHIPS_VISIBLE` is a static cap of 2 with an unconditional `slice`, so they
are already in their overflow form at every width, and making them
width-reactive would fork the component AC-UI-PIPELINE-ROW-003.7 reuses.
Status indicators and the actions cluster never truncate, and nothing wraps
(AC-UI-PIPELINE-ROW-003.9).

**Menu.** The trigger and the row's context-menu target both open the shared
entry set. A selection runs the entry's handler and stops propagation before the
row's activation handler sees it (AC-UI-PIPELINE-ROW-002.7). Dialogs render from
the shared dialogs component at the row level.

**Move.** The request path is unchanged: the move controls and the move-to
entries call the same `onMoveTask` that `useSwimlaneMove` executes today. That
hook's failure path is corrected as described under
[Existing components under change](#existing-components-under-change). The other
addition is the in-flight guard in AC-UI-PIPELINE-ROW-005.2, held at the row so the move
controls and the move-to entries of that row both observe it. The guard is
deliberately row-local: it is not a cross-surface mutation registry, and a
conflicting move from another client is handled by
AC-UI-PIPELINE-ROW-005.3 and AC-UI-PIPELINE-ROW-005.4 instead.

**Move races.** The row is not a second source of truth for the task's step: it
renders whatever the store holds, so an out-of-band change landing mid-move is
simply the new rendering. Only a *failure* writes back, and the compare-and-swap
above stops that write from undoing a newer value: it restores the pre-move step
only while the store still holds this move's optimistic value. A failed move can
therefore neither resurrect a stale step nor revert an unrelated task. The guard
is presentation only, and is released on every terminal outcome, so a rejected
move can be retried immediately (AC-UI-PIPELINE-ROW-005.3). There is no
cancellation outcome, because the move path holds no `AbortController`.

### Nesting constraint

The current row nests its content inside a `<button>`. Repository chip tooltips,
the title disclosure surface, the step tooltips, and several status indicators
are interactive, and an interactive element inside a button is invalid and
breaks pointer behavior (AC-UI-PIPELINE-ROW-003.10). The row's activation target
must therefore be a non-button element carrying the click handler. A title-only
button is not an alternative: `TaskTitleHoverCard`'s fine-pointer branch renders
its own `<button data-testid="task-title-preview-trigger">` *wrapping* its
children, reproducing the same invalid nesting.

Activation composes because the trigger is selective about what it consumes. Its
`handleTriggerClick` acts only when `event.detail === 0`, the keyboard
Enter/Space case, and returns early for pointer clicks, which therefore reach the
row's activation handler on the non-button ancestor.

## Failure and recovery

- **No resolvable current step.** `currentStepIndex` resolves to `-1`, which
  makes every step read as `future`, leaving the task's location unstated; that
  is the failure AC-UI-PIPELINE-ROW-005.6 exists to prevent.

  **How the state is reached.** By exactly one route: a task whose
  `workflowStepId` is empty. `toKanbanTask` in `lib/kanban/map-task.ts` coerces a
  missing `workflow_step_id` to `""`, and `remapOrphanTasks` in
  `components/kanban/swimlane-orphan-display.ts` deliberately skips falsy ids
  (`!task.workflowStepId` returns the task unchanged), so such a task is never
  rewritten to `ORPHAN_STEP_ID` and `findIndex` over the displayed steps returns
  `-1`. Two further call sites defend against the same value, which is why it is
  a real state rather than a type-level impossibility:
  `deriveAutoHiddenStepIds` filters occupancy through `.filter(Boolean)`, and
  `auto-hide-empty-columns.ts` types its task as
  `{ workflowStepId: string | null }`.

  **A hidden step cannot reach this state.** All three hiding routes are closed
  before a row renders:

  1. *Manually hidden.* `filterTasks` in `lib/kanban/task-projections.ts` drops
     every task whose `workflowStepId` is in both the manually hidden set and the
     live step set: the same intersection `swimlane-container.tsx` uses to build
     the `steps` and `moveTargetSteps` props, passed as `hiddenStepIds` on the
     `projectWorkflowTasks` call whose `visibleTasks` become the view's `tasks`.
     A hidden step contributes zero rows, not a row on a hidden step.
  2. *Auto-hidden.* `deriveAutoHiddenStepIds` only hides steps holding no tasks,
     so no task can sit on one.
  3. *Deleted.* `remapOrphanTasks` rewrites the id to `ORPHAN_STEP_ID`, which
     `getGraph2DisplayState` appends to `displaySteps` with a title, so that
     step is present and labelled.

  Because no hidden step's title is ever needed, **this design requires no
  data-layer change.** The row preserves no pre-remap `workflowStepId`, does not
  read `kanbanMulti.snapshots[workflowId].steps`, leaves `remapOrphanTasks`
  untouched, and adds no `ViewContentProps` member. Kanban's "Needs
  reassignment" column is unaffected for the same reason.

  **Marker and phase.** With no resolvable current step every displayed marker
  is `future`, and one labelled marker carrying fixed unassigned-step copy
  renders before them. When the current step is displayed, phase is today's
  `currentStepIndex` comparison, unchanged.
- **Empty displayed steps.** The row renders its information column and actions
  with no step run (AC-UI-PIPELINE-ROW-005.7). It is checked first and wins over
  the no-current-step case above, which would otherwise also match an empty
  displayed list. The list's existing empty state is unchanged.
- **In-flight move.** Concurrent requests for the same task are dropped, not
  queued (AC-UI-PIPELINE-ROW-005.2); a queue would let a person build a backlog
  of moves whose end state they cannot predict. Failure falls back to the
  server-reported step and the existing `onMoveError` surface.
- **Out-of-band step change with a menu open.** The row re-renders at the new
  step and the menu stays open (AC-UI-PIPELINE-ROW-005.4). Keying menu or dialog
  state on the step, rather than the task id, would close a person's menu
  whenever an agent advanced the task, so the state is keyed on the task id.
- **Task removed while a surface is open.** The row unmounts and its portalled
  menu and dialogs unmount with it, without an error
  (AC-UI-PIPELINE-ROW-005.5).
- **Missing workspace-scoped data.** When repositories or link availability have
  not resolved, chips and link entries render in their empty or disabled state
  rather than blocking the row.

## Persistence

None. The row is presentation state. The view-mode preference
(`kandev.taskListing.view.v1`) is read but never written here, and no new
preference is introduced.

## Security

No new data is fetched, no new endpoint is called, and no permission boundary
moves. Plugin contributions render through the existing slot and menu-action
contracts with their existing isolation. Repository paths shown on hover are
already visible on the Kanban card in the same workspace, so the row exposes
nothing a person could not already see.

## Observability

No new metrics or logs. Behavior is observed through tests:

- **Component tests** for the labelled-pill invariants, including the
  no-current-step case and its leading position, the empty-steps
  case taking precedence over it, the fixed inline order, absent-state
  rendering, the sort tiebreak including the no-current-step group sorting last,
  the row-local in-flight move guard and its release on every terminal outcome,
  an out-of-band step change surviving a move that settles afterwards, the
  first-step and last-step move-control boundaries, the
  AC-UI-PIPELINE-ROW-003.11 yield order through to its scroll terminus, the
  information column's fixed width and contents, and the menu-stays-open case. The no-current-step fixture is easy to build wrong: the
  task's `workflowStepId` must be the **empty string**, not an id merely absent
  from the step list, because an absent id is remapped to `ORPHAN_STEP_ID` and
  renders as the titled orphan step instead.
- **Shared-source tests**, two of them. One asserts that the Kanban card and the
  pipeline row produce the same menu entry identities and order for the same
  task (AC-UI-PIPELINE-ROW-002.4). The second does the same for the inline
  status strip: for one task fixture carrying a blocked state, a queued step, a
  review state, two repositories, more than one session and a subagent count,
  both surfaces render the same indicator set in the same relative order, and
  the row renders exactly one session-count element, inside its information
  column. Session count is compared for presence rather than position, because
  the row seats it in the column ahead of every strip member while the card
  seats it among them (AC-UI-PIPELINE-ROW-003.8). Without it the
  duplicated-JSX fork the extraction exists to prevent would pass every other
  test in this list.
- **A Kanban regression test** for AC-UI-PIPELINE-ROW-002.6 and for the widened
  disclosure surface, including the negative case: a short-titled task with no
  description, parent, or subtasks still opens no popover.
  AC-UI-PIPELINE-ROW-002.6's fixture matrix is six cards: a bare task; a task
  with two repositories and an overflow chip; a task blocked by a failed
  predecessor; a task queued for a step; a task with a review state and more
  than one session; and a task with both plugin slots contributing. For each,
  the menu entry identity, order, labels, enablement and destructive styling,
  and the inline indicator set and order, match the rendering at this contract's
  base commit. The disclosure change AC-UI-PIPELINE-ROW-004.4 makes is the one
  sanctioned difference and is asserted separately.
- **E2E** for the 1280px nine-step fit (AC-UI-PIPELINE-ROW-001.3): the row's
  width no greater than the list's `clientWidth`, and the current step's pill
  within the step run's visible bounds without further scrolling; the pointer
  hover paths that component tests cannot exercise (chip paths, the disclosure
  surface); right-click opening the context menu; and one narrow-surface case
  proving the actions cluster stays reachable once the row reaches
  AC-UI-PIPELINE-ROW-003.11's scroll terminus. Follow
  `e2e/README.md`: arm a causal wait before the action, then assert with default
  timeouts.

## Localization

New copy is the accessible row summary (AC-UI-PIPELINE-ROW-001.6) and the
unassigned-step marker's label (AC-UI-PIPELINE-ROW-005.6); everything else
reuses existing `kanban:` keys. The row summary is ordinal-bearing and must use
`{{count}}`-style interpolation rather than assembled fragments; the
unassigned-step label is a fixed string with no interpolation and no plural
form, because it names an absence rather than a position. All five locales are required and
`pnpm run i18n:zh-hant` generates the Traditional Chinese pair. No U+2014.

## Related decisions

None. This design introduces no new architecture, public contract, or
persistence boundary; it moves module-private code into a shared module and
changes a rendering rule.
