---
status: draft
system: ui
requirements:
  - REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001
  - REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002
  - REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003
---

# Kanban Preview Workflow Step Navigation System Design

## Purpose and boundaries

The UI system owns the preview panel header and the compact stepper it now
hosts. The task system owns workflow data and the task-move API. This design
adds a consumer of the existing compact stepper; it does not add a backend
contract, a store field, or a persisted preference.

## Requirement mapping

| Requirement                                       | Design section                                                                                                                                             |
| ------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001`       | [Components](#components-and-responsibilities), [Step resolution](#step-resolution), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |
| `REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002`       | [Header layout](#header-layout)                                                                                                                             |
| `REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003`       | [Components](#components-and-responsibilities), [Header layout](#header-layout)                                                                             |

## Components and responsibilities

- `TaskPreviewPanel` owns the preview header. It gains a step indicator slot
  between the title and the panel controls, and a move-failure slot below the
  header row.
- `CopyTaskUrlButton` (`components/task/copy-task-url-button.tsx`) renders the
  copy-task-link control (REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003). It
  builds the task's detail URL from `linkToTask` (`lib/links.ts`) against
  `window.location.origin` and writes it with the shared `copyToClipboard`
  utility (`lib/utils/copy-to-clipboard.ts`), which already covers the
  non-secure-context fallback this control reuses rather than reimplementing.
  It is a leaf presentation component that owns only its own transient
  copied-state timer; it reads no store state beyond the task id it is given.
- The compact stepper and its disclosure body stay the single implementation of
  the indicator, the step list, and eligibility. They do not implement the move
  request: they take an `onMove` callback and call it. The preview renders the
  compact presentation unconditionally rather than measuring available width:
  the preview header is never wide enough for the full stepper, and a width
  measurement here would only add a flicker.
- The move request is today private to `handleMove` inside `WorkflowStepper`,
  which the preview does not render, so there is nothing for the preview to
  inherit. That logic is extracted into one shared hook, `useWorkflowStepMove`,
  which becomes the single implementation of the move request **for the compact
  stepper surfaces** and owns: the
  move call itself, the plan-mode cleanup for the task's active session, the
  in-flight step id, the request-identity guard that discards a superseded or
  late response, and the move-start and move-error callbacks. `WorkflowStepper`
  and the preview both consume the hook; neither reimplements any part of it.
  Extraction rather than duplication is what makes the plan-mode invariant in
  the requirement's Decisions section hold on both surfaces, and it is why a
  second copy of this logic is not an acceptable implementation.
- **That claim is scoped to the compact stepper, and deliberately so.** Moving a
  task between workflow steps is not otherwise centralised in this codebase, and
  this design does not centralise it. `moveTask` / `moveTaskById`
  (`lib/api/domains/kanban-api.ts`, wrapped by `hooks/use-task-actions.ts`) has
  several independent callers besides the stepper: the board's drag and drop
  (`hooks/use-drag-and-drop.ts`), the swimlane movers
  (`hooks/domains/kanban/use-swimlane-move.ts` and the
  `components/kanban/swimlane-*-content.tsx` surfaces), multi-select bulk moves
  (`hooks/use-task-multi-select.ts`), plan actions
  (`hooks/domains/kanban/use-plan-actions.ts`), and the task session sidebar's own
  mover, `useMoveToStep` (`components/task/task-session-sidebar-move.ts`).
  `useWorkflowStepMove` replaces exactly one of them, `handleMove`, and leaves
  every other caller untouched. Read unqualified, "the single implementation of
  the move request" would tell a builder to consolidate all of them, which is a
  different and much larger feature than this one.
- **`useMoveToStep` in particular stays where it is.** It is not a duplicate of
  the hook being extracted: it applies the move optimistically to
  `kanbanMulti.snapshots[workflowId].tasks`, keeps a per-task generation counter so
  a rejection cannot roll back a newer in-flight move, and rolls the store back
  when the backend refuses. Those semantics serve the sidebar's own interaction and
  carry their own tests. Folding them into `useWorkflowStepMove` would turn the
  preview's and the task top bar's moves from authoritative into optimistic, which
  no acceptance criterion here asks for and which would change a surface this
  requirement puts out of scope. The two hooks coexist.
- **They can be on screen together, and the existing rules already resolve it.**
  `TaskSessionSidebar` renders from
  `components/app-sidebar/sections/tasks-section.tsx`, the persistent app sidebar,
  not from inside the preview, so one user can move the same task from the sidebar
  and from the preview header. Neither hook needs to know about the other: the
  backend serialises the moves and the preview renders whatever task state it then
  receives, per the requirement's Decisions section. If the sidebar's optimistic
  write, or its rollback, changes the previewed task's step, the indicator
  re-derives from the newest task state exactly as it does for any other
  out-of-band step change
  (AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.12), including while the disclosure is
  open. A rollback moving the indicator backwards is therefore correct behaviour,
  not a race to guard against: the indicator is showing the step the task is
  actually on.
- `canMoveToStep` remains the single UI policy for target eligibility.
- `useTouchDrawer` continues to select the popover or the drawer surface from
  pointer precision.
- `KanbanWithPreview` resolves the previewed task's own workflow step list and
  passes it, the task id, the workflow id, and the current step id down to
  `TaskPreviewPanel`. It also owns the move-failure state for the panel, cleared
  on a new move and on a task change. It also resolves the rendered panel width
  from the user's chosen width and the live pointer mode
  (REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002, see
  [Header layout](#pointer-aware-minimum-panel-width)).

## Step resolution

The previewed task carries its own workflow id, which
`KanbanWithPreview` already derives (from the task record, or from the snapshot
key when a boot-hydrated snapshot task omits it).

Steps resolve from the store in one rule, the same one `plugin-context-api`
uses:

- when the task's workflow id equals `kanban.workflowId`, use `kanban.steps`;
- otherwise use `kanbanMulti.snapshots[workflowId].steps`;
- when neither resolves, the list is empty and no indicator renders.

`useAllWorkflowSnapshots` runs whenever the board mounts, for every workflow in
the workspace, so the second branch covers the multi-workflow board and a
preview opened on a task outside the active workflow filter. The snapshot step
shape already carries `allow_manual_move`, `events`, and `agent_profile_id`, so
the mapping to the stepper's step type is the same field-for-field mapping the
task page performs.

Board column visibility (`hiddenWorkflowStepIds`) is a board rendering filter
and is deliberately not consulted here. That is the point of the feature: the
disclosure is the way to reach a step whose column is hidden.

## Ordering and determinism

The ordering AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.4 states is already
implemented in this repository, and this design adopts it rather than writing it
again. `sortWorkflowStepsByPosition`, exported from
`apps/web/lib/kanban/auto-hide-empty-columns.ts`, sorts a copy by ascending
`position` and breaks ties on ascending `id` compared as a string, which is
exactly the rule that AC names. It already has a unit test for that determinism,
and the board already orders its columns through it, from
`components/kanban/columns-menu.tsx` and `components/kanban/swimlane-container.tsx`.

So AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.4 holds on the board surfaces today.
What does not hold is the stepper: `WorkflowStepper` sorts with its own inline
comparator on `position` alone, private to the component, with no tiebreak. That
is the only non-compliant ordering, and the preview cannot inherit the fix by
rendering `WorkflowStepper`, because it does not render it.

**The comparator moves; it is not rewritten and it is not duplicated.**
`sortWorkflowStepsByPosition` moves, unchanged in name and in body, out of
`auto-hide-empty-columns.ts` and into `apps/web/lib/kanban/workflow-step-order.ts`,
a module that owns workflow step ordering and nothing else. The name and location
follow the existing `lib/kanban/task-order.ts`, which is the in-repo precedent for
a small ordering-only module and orders the board's tasks the way this one orders
its steps. Its existing unit test moves with it, to
`workflow-step-order.test.ts`. Its two existing
board consumers change their import path and nothing else. `WorkflowStepper` and
the preview's step resolution then both import that one function, and
`WorkflowStepper`'s private inline sort is deleted.

Three things this rules out, each of which a builder would otherwise have to
decide alone:

- **A second utility is not acceptable**, whatever it is called. Two shared
  functions with the same comparator leave two sets of call sites free to drift,
  which is the failure this section exists to prevent, and under drift the board
  columns and the disclosure list could order equal-position steps by two
  different rules, in a feature whose whole premise is that a disclosure row
  stands in for a board column the user cannot see.
- **Importing it from `auto-hide-empty-columns.ts` where it sits today is not
  acceptable either**, even though it would work. That module is named for a
  different feature, and shared stepper code depending on it would break for a
  reason nobody could predict the day the auto-hide feature is removed.
- **The exported name stays `sortWorkflowStepsByPosition`.** `position` is the
  primary key and the tiebreak is secondary, so the name is accurate; renaming
  would churn two call sites and a test for no behavioural gain. A builder should
  not improve it in passing.

Moving the function changes no rendered board column order, because the
comparator is byte-identical before and after, so the requirement's exclusion of
changes to which columns the board renders is untouched. The two board files are
edited only in their import statement.

The function returns a new array and does not mutate its input, which is what
lets every surface sort the same store-owned step list independently. An empty
step list sorts to an empty list, and there is no missing-`position` case to
define: `position` is a required `number` on the store's step type, so the
comparator never sees one absent. Two steps cannot share an `id`, so the tiebreak
is total.

No other ordering exists in this surface: the disclosure renders the sorted list
top to bottom, and the position count is the current step's index in that same
list. When the step list changes underneath an open disclosure
(AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.12), it is re-sorted from the newest
store state by the same function, so a re-derivation cannot produce a different
order than a first render of the same steps.

## Header layout

The preview header stays one flex row: title, step indicator, control cluster.
Four widths bound it: the control cluster, the 88px title floor, the step
indicator's floor, and the minimum panel width. Earlier revisions of this
section let the indicator shrink to 0 to protect the title. With the
copy-task-link control added, that broke the indicator's 44px touch floor
(AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.17). This revision gives every floor
a fixed size and derives the minimum panel width from their sum, per pointer
mode, so no floor yields to another at any width the panel can take.

### Terms, measured

Each number is read from a class or measured in Chromium with the built
stylesheet and the app's Figtree font. The list is part of the contract: an
unlisted term cannot be re-checked when the layout changes.

- **Header content box.** Inline layout: the panel's outer container carries
  `border-l` (1px), holds the resize handle (`w-1`, 4px, non-shrinking) before
  the panel, the panel root carries `border-l` (1px), and the header row adds
  `px-4` (32px). Box sizing is `border-box` throughout. So the content box is
  `W - 38` for an outer width `W`. The floating layout has no outer
  `border-l`, so its content box is `W - 37`, one pixel wider. The inline
  layout is therefore the binding case.
- **Control widths.** `@kandev/ui`'s `Button` resolves `size="icon"` to
  `size-7 max-md:size-11 [@media(pointer:coarse)]:size-11`
  (`control-sizing.tsx`). tailwind-merge overrides only classes in the same
  variant scope, so an instance's unprefixed `h-8 w-8` replaces `size-7` but
  never the `pointer:coarse` variant. The built stylesheet emits that variant
  after `.h-8` and `.w-8` at equal specificity, so it wins at a coarse pointer.
  At a fine pointer, `CopyTaskUrlButton` and the open-full-page control render
  at 32px and the close control at 28px. At a coarse pointer all three render at
  44px. The `max-md:` variant is unreachable here, because `KanbanWithPreview`
  renders no preview below 768px.
- **Task actions menu trigger.** A raw `button` with `p-1 -m-1` around a 16px
  icon. The padding and negative margin cancel in layout, so it occupies 16px
  of row width at both pointer modes. It is counted in the budget, because it
  renders whenever the task offers actions, which is the common case.
- **Control cluster.** `flex items-center gap-1`, so one 4px gap between each
  pair of adjacent children. With the trigger present: fine
  `16 + 32 + 32 + 28 + 3 * 4` = **120px**, coarse `16 + 44 * 3 + 3 * 4` =
  **160px**. Without the trigger: 100px fine, 140px coarse. The budget uses
  the larger figures.
- **Title floor.** `min-w-[88px]` on the `h2`: **88px**
  (AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.3).
- **Indicator floor.** `CompactWorkflowTrigger` is `px-2` (16px) with
  `gap-1.5` (6px) between its children: the marker group (8px `h-2 w-2`
  marker, a 6px gap, and the step name span, which truncates to 0), the
  position count (`text-[11px] tabular-nums`, not shrinking), and at a coarse
  pointer the 14px disclosure cue. Measured: the count is 17.4px for one digit
  on each side (`3/5`) and 31.4px for two (`12/15`). The indicator floor is
  therefore 53.4px fine and 73.4px coarse for workflows of at most 9 steps,
  and 67.4px fine and 87.4px coarse for 10 to 99 steps. A single-step
  workflow omits the count, which gives 30px fine and 50px coarse. The budget
  takes the 10-to-99-step ceiling, rounded up: **68px fine, 88px coarse**.
  Every coarse value is at least 44px, so the indicator's content already
  satisfies AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.17. The height comes from
  the trigger's existing `min-h-11`.
- **Gaps.** `g` is the total of the header row's own inter-element gaps,
  between the title, the indicator, and the control cluster. The header
  carries no gap class today, so `g = 0`.

### The budget

Required content width = cluster + title floor + indicator floor + `g`:

- Fine: `120 + 88 + 68 + g` = `276 + g`. Inline outer width `>= 314 + g`.
- Coarse: `160 + 88 + 88 + g` = `336 + g`. Inline outer width `>= 374 + g`.

The minimum panel widths in AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.7 are
320px fine (content box 282px) and 380px coarse (content box 342px). Each
leaves 6px of slack, so the header satisfies every floor while **`g <= 6px`**
in the inline layout and `g <= 7px` in the floating layout. That is the bound
AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.4 names. Both pointer modes bind at
the same `g`, by choice of the two minimums rather than by coincidence.

At the old 300px minimum the same sums fail. Coarse: content box 262px, and
`160 + 88` leaves 14px for an indicator whose floor is 50px to 88px, so the
indicator's hit area falls below 44px. Fine: `120 + 88` leaves 54px, enough
for a one-digit count (53.4px) but not a two-digit one (67.4px), so the
count's non-shrinking box overflows the indicator. Both failures are caused by
this requirement's third control, which is why this requirement fixes them.

Three-digit counts (100 or more steps) are out of scope, per the requirement.
Their indicator floor (about 81.4px fine, 101.4px coarse) passes the 74px and
94px left beside the 88px title by 7.4px, so the `min-w-0` group overflows onto
the control cluster, not into a second row.

### Mechanism

- **The indicator does not shrink below its floor.** Today the preview's
  indicator wrapper (`min-w-0 max-w-[50%] shrink [&>button]:w-full`) and the
  trigger (`min-w-0`) can both shrink to 0, which lets the trigger's
  non-shrinking children overflow its box. The preview's wrapper enforces the
  floor from the preview side: it gives the trigger `min-width: min-content`
  (for example `[&>button]:min-w-min`, which outranks the trigger's own
  `min-w-0` by selector specificity) and lets its own width fall back to the
  trigger's min-content instead of 0. Because the step name span keeps
  `min-w-0 truncate`, the min-content is exactly the indicator floor. The
  shared trigger's own default classes stay unchanged, so the task top bar
  keeps its current presentation, per the requirement's Out of scope.
- **The cap stays a share of the remainder.** The indicator's half-share cap is
  half of the row's content width after the control cluster and `g`, not 50%
  of the whole row. The nested shrinkable group holding the title and the
  indicator is what excludes the cluster from the percentage basis. A plain
  `max-width: 50%` on the indicator would resolve against the full content box
  and give a different bound. When the cap and the title floor conflict, the
  title's `min-w-[88px]` wins and the indicator shrinks below its cap, down to
  its floor and never past it. At the minimum panel width, both floors fit by
  construction of the budget above.
- **Free space goes to the title.** The title is the shrinkable and growable
  element. The indicator sits next to the control cluster at the end of the
  row, so a short title does not leave it mid-row and its position does not
  move as the title's length changes.
- **Truncation.** The title and the step name truncate with an ellipsis. The
  marker, the position count, and the disclosure cue do not shrink. The
  indicator's accessible name carries the step name, number, and total, so
  truncation loses no information.

### Pointer-aware minimum panel width

- `PREVIEW_PANEL` (`lib/settings/constants.ts`) sets `MIN_WIDTH_PX` to 320 and
  adds `COARSE_MIN_WIDTH_PX: 380`. `MIN_WIDTH_PX` stays the storage floor:
  `useKanbanPreview` keeps clamping the chosen width to it, both when restoring
  from `kandev.kanban.preview.width` and in `updatePreviewWidth`. So a stored
  300px, or the E2E seed of `1`, reads as 320px, and the persisted value is
  always the chosen width, never a pointer-adjusted one.
- One pure helper computes the rendered width: `max(chosenWidthPx,
  isFinePointer ? MIN_WIDTH_PX : COARSE_MIN_WIDTH_PX)`. It lives next to the
  constants so it has one owner and a unit test.
  `KanbanWithPreview` reads `isFinePointer` from `useResponsiveBreakpoint`,
  which already tracks pointer precision live. It passes the rendered width
  to `useKanbanLayout` (the floating-versus-inline decision), to both
  layouts' `width` style, and to the resize handler's start width. A pointer
  change re-renders through the hook's subscription, so no reload is needed
  and no effect writes the rendered width back to state.
- The resize handler clamps the width it reports to the rendered minimum in
  effect during the drag before calling `updatePreviewWidth`. The handle
  accepts mouse input only, so in practice that is the fine minimum.
- The 500px default and the 95vw maximum are unchanged; at 768px, the
  narrowest viewport with a preview, 95vw is 729px.

The binding layout is the inline layout at each pointer mode's minimum panel
width. The header must be proven there at both pointer modes, not only at the
500px default and not only in the floating layout.

## Control flow

1. The user opens the preview on a task. `KanbanWithPreview` resolves the task's
   own workflow steps and current step id.
2. `TaskPreviewPanel` renders the step indicator when the list is non-empty.
3. Hover, focus, or touch activation opens the disclosure with every step of that
   workflow in sorted order.
4. The disclosure marks the current step and enables only eligible targets.
5. Selecting a target disables every move control in the surface and sends the
   existing move request with the task's own workflow id, the target step id,
   and position `0`.
6. A success closes the disclosure. The preview stays open on the same task and
   the same session; live task state then drives the indicator to the new step.
7. A failure leaves the disclosure open and raises the move-failure message
   below the header.

## Dismissal and stacking

`KanbanWithPreview` closes the preview from a window-level Escape listener. The
disclosure closes from its own document-level Escape handling. Without a guard,
one Escape press would close both, because the disclosure's handler runs on
`document` and the preview's runs on `window` for the same event.

The preview's Escape handler must therefore ignore an Escape that a disclosure
open inside the panel is already consuming. Whatever mechanism carries that
signal, the observable contract is the one in
AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.15: the first Escape closes the
disclosure only, the second closes the preview.

The disclosure content is portalled to the document body at `z-50`, above the
floating preview panel at `z-40` and its backdrop at `z-30`. Because the
disclosure is not a descendant of the backdrop, a click inside it never reaches
the backdrop's close handler.

## Failure and recovery

The move request stays the authority for transition errors. On error the
in-flight state clears, every move control re-enables, and the panel renders the
existing move-failure banner below the header. The banner clears when the next
move starts and when the previewed task changes. A failure that arrives after
the preview has stopped showing the task that issued the request is discarded
rather than rendered, which is the case clearing alone cannot cover: clearing
acts on a message that already exists, and a late response would otherwise
create one after the fact. Per
AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.11 the test is whether the preview
stayed open on that same task **continuously** since the request was issued, so
all three of switching to another task, closing the preview, and closing it and
reopening it on the same task must discard. Together the two rules give the
invariant that a stale error is never rendered against a presentation that did
not issue it.

That test is not decidable from the task id, and it is not decidable from the
in-flight counter the current move code carries. `handleMove` guards its late
response with a monotonic request counter held in a component ref, which
distinguishes only one request from a newer one issued by the same live
component; on a close and reopen of the same task the task id is unchanged and
the counter is not advanced by either event, so both signals report "same task,
newest request" for a request the user has since walked away from. Nor can the
hook lean on being unmounted: the move-failure state lives in
`KanbanWithPreview`, which stays mounted across a preview close, and the two
layouts disagree about the panel subtree — the floating layout unmounts it with
the preview while the inline layout renders it inside a pane that stays mounted.

So the identity the hook carries must be a **presentation instance**: a token
minted each time the preview begins showing a task, and invalidated when the
preview closes or switches away, such that reopening on the same task mints a
new one. A response is rendered only when the token captured at request time is
still the current one. Extracting `handleMove` verbatim does not produce this —
the counter is necessary for supersession and not sufficient for continuity, and
`useWorkflowStepMove` owns both. This is the one place where the extraction adds
behaviour rather than relocating it; the task top bar inherits it, which is
correct, because a late failure that outlives its own presentation is wrong on
both surfaces.

The token is supplied by the consuming surface, because only the surface knows
what one of its presentations is. The preview supplies its continuous
presentation of a task, per the rule above. The task top bar supplies the task
route's continuous presentation of a task, so a navigation away and back mints a
new token there for the same reason a close and reopen does here. A surface with
no finer notion than the task itself supplies a token that changes at least
whenever the task changes, which degrades to today's behaviour rather than to
something undefined.

The in-flight step id is scoped to the same token. A new presentation therefore
begins with no in-flight move and no disabled control even while an earlier
request is still outstanding, which is what keeps
AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.19's guarantee — the backend still
applies that move, the preview does not cancel it — from leaving a reopened
preview with a permanently disabled row. A discarded response neither renders an
error nor re-enables or disables anything in the presentation that replaced it;
it affects only the presentation that issued it, which by then is gone.

An absent current step falls back to showing the first step in order. The
current-step marker is suppressed in that case, so the surface never asserts a
step the task is not on. The disclosure body already behaves this way; the
shared indicator does not, and is corrected here
(AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.14). The correction is in shared
code, so it also removes the same false marker from the task top bar.

## Test strategy

Component tests cover: indicator presence and absence across resolvable,
unresolvable, empty, and single-step workflows; step resolution for a task
outside the active workflow filter; the position tiebreak; eligibility and the
current-step row; the in-flight disable; the success path leaving the panel open;
the failure banner and its clearing rules; and the
discard of a late failure across all three discontinuities
AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.11 names, the reopen-on-the-same-task
case included, since that is the one the task id and the request counter both
report as continuous.

Because the presentation-instance token lives in the shared move hook and both
surfaces inherit it, the discard rule is tested against the hook itself, on a
token change, and not only through the preview. Otherwise the top bar's half of
the third correction the requirement's Out of scope names ships with no coverage
at all, on the same argument that applies to the marker fix below: the code runs
in production on two surfaces, so a test that exercises one of them leaves the
other unguarded. The ordering utility keeps its existing unit test, which already
asserts the tiebreak and non-mutation, and moves with it; what is new there is
that `WorkflowStepper` now sorts through it, so the stepper's own tests cover
equal positions rather than assuming they cannot occur.

The correction AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001.14 makes to the shared
indicator needs an observable that distinguishes a resolved current step from
the first-step fallback, and the implementation must expose one; today the three
marker states differ only by presentation classes on unlabelled elements, which
is not a contract a test can hold. Which observable to expose is an
implementation choice, but a test that cannot tell the corrected marker from the
uncorrected one does not cover this AC. In particular the existing shared-stepper
test for the collapsed no-current-step case asserts only the absence of
`aria-current`, which is already conditioned correctly in the current code and so
passes against the uncorrected marker: it is a false negative for this
correction, not coverage of it. The correction lands in code the task top bar
renders in production, so an untested change here regresses two surfaces.

Unit tests cover the rendered-width helper: a chosen width below, at, and above
each pointer mode's minimum; the same chosen width under both pointer modes, to
show a pointer change re-derives the rendered width without changing the chosen
one; and `useKanbanPreview` reading a stored 300px, and a stored 1, as 320px.
A component test renders `KanbanWithPreview` with a mocked
`useResponsiveBreakpoint` and a stored 320px width. It flips `isFinePointer` and
asserts the panel's `width` style moves from 320px to 380px and back, with the
stored value untouched. This is the only practical coverage of a live pointer
change, because Playwright cannot switch a page's pointer media mid-test.

Desktop E2E covers the motivating scenario end to end: hide a step's column
through the board column visibility filter, open the preview on a task, open the
disclosure, confirm the hidden step is still listed, move the task to it, and
assert the task's step through the API. A third asserts the two-stage Escape.

Two E2E scenarios prove the header budget at its binding width, one per pointer
mode. They share one setup and one set of assertions, so they differ only in
the page fixture:

- **Setup.** Add workflow steps until the task's own workflow has at least 10
  steps, so the position count has two digits, which is the widest count the
  budget covers. Put the task on a step whose name is long enough to drive the
  indicator to its cap, and make sure the task offers actions, so the task
  actions menu trigger renders and the full control cluster is in play. Seed
  `kandev.kanban.preview.width` with `1` so the panel renders at exactly its
  minimum without a fragile drag.
- **Fine pointer** (`testPage`, desktop chromium at 1400x900, inline layout).
  The existing containment test is extended with this setup.
- **Coarse pointer** (`coarseDesktopTestPage`, 1280x900, `hasTouch`). A new
  test. Its board container of about 1024px keeps the 380px panel inline, the
  binding layout; `tabletTestPage` would float it.
- **Assertions, both pointer modes.** The panel is inline (no floating
  backdrop). Its outer width equals the pointer mode's minimum (320px or
  380px, within 1px). The header is a single
  row: every header element shares one vertical center within 4px. The title
  is at least 88px wide: the literal floor, because a non-zero assertion passes
  on a one-pixel title. Every control-cluster element is visible, enabled, and
  inside the panel's box. The header has no horizontal scroll
  (`scrollWidth - clientWidth <= 1`). The step indicator's box contains the
  boxes of its marker and position count (and, at a coarse pointer, its
  disclosure cue), which proves the floor holds rather than overflowing. The
  indicator's right edge does not pass the cluster's left edge. The indicator
  is at most half the title-and-indicator group's width plus 1px, the
  AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.4 cap (about 91px coarse, where it
  binds). At a coarse pointer only, the indicator is
  at least 44px wide and 44px tall.

The existing tablet touch-drawer E2E stays.

## Related decisions

No architecture decision record applies. This design reuses the compact stepper
and task-move contracts already established by
`REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001`.
