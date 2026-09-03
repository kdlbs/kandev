---
status: draft
system: ui
requirements:
  - REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001
  - REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002
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

## Components and responsibilities

- `TaskPreviewPanel` owns the preview header. It gains a step indicator slot
  between the title and the panel controls, and a move-failure slot below the
  header row.
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
  on a new move and on a task change.

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

The preview header stays one flex row: title, step indicator, panel controls.

- The panel controls do not shrink.
- The step indicator sits in a shrinkable container with an upper bound on the
  share of the row it can claim, so a long step name cannot crowd the title out
  of the row. That bound is half of the row's width remaining after BOTH the
  panel controls and the row's inter-element gaps take their fixed width
  (AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.4). Both subtractions are load
  bearing, and so is the base width they are subtracted from; the next bullet
  derives all three and records why.
- The arithmetic at the 300px panel minimum, with `g` the total of the row's
  inter-element gaps. The 300px is the width of the panel's OUTER container, not
  the header's content box, and several terms sit between the two. Enumerated in
  order for the inline layout: that container carries `border-l` (1px); it holds
  the resize handle (`w-1`, 4px, non-shrinking) as a flex sibling before the
  panel; the panel's own root carries `border-l` (1px); and the header row adds
  `px-4` on both sides (32px). Box sizing is `border-box` throughout, so each
  border is inside the width it is measured against. The header's content box is
  therefore `300 - 1 - 4 - 1 - 32` = **262px**. The two `h-8 w-8` panel controls
  plus their own `gap-1` take 68px, leaving 194px, of which the gaps take `g`.
  The indicator's cap is therefore `(194 - g) / 2`, and the title is left
  `(194 - g) / 2`. The title floor of 88px
  (AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.3) holds while `g <= 18px`. That is
  the constraint the header must respect. It remains generous in practice, since
  the header carries no gap class today, so `g = 0` and the title gets 97px.
- Every term in that derivation is named on purpose. Two earlier drafts of this
  section each stated a bound computed from premises they had not inspected: the
  first omitted the inter-element gaps, the second took the header's base width
  to be the 300px minimum less only its own padding, missing the container
  border, the resize handle and the panel border between them. Both produced a
  bound that looked safe and was not. A bound whose terms are not enumerated
  cannot be re-checked when the layout around it changes, so the enumeration is
  part of the contract rather than commentary on it.
- The two layouts differ by one pixel at the same panel width. The floating
  layout's outer container has no `border-l`, so its header content box is 263px
  and its remainder 195px, giving `g <= 19px`. The inline layout is therefore the
  binding case, and `g <= 18px` is the single bound that holds for both: a header
  satisfying the inline layout satisfies the floating one by construction.
- When a row cannot satisfy both bounds — which the inequality above prevents at
  the 300px minimum, but which must still resolve deterministically at any width
  and under any future spacing — the title floor wins and the indicator shrinks
  below its cap. The cap is a maximum, never a reservation, so yielding is always
  available to it; the floor is a minimum and has nowhere to yield to. The two
  bounds are stated separately because the cap governs at every width while the
  floor is what an assertion at the minimum width checks.
- The cap is a share of the width remaining after the controls, NOT a percentage
  of the whole row. A plain `max-width: 50%` on the indicator would resolve
  against the row's full 262px content box and yield 131px, which is not this
  bound. Expressing it takes a nested shrinkable group holding the title and the
  indicator, or an equivalent that excludes the controls from the percentage
  basis. Relatedly, the header is `justify-between` with two children today; a
  third child changes how free space is distributed, so the row's alignment is
  part of what the containment assertions must cover rather than something that
  survives the change untouched. Free space goes to the title, which is the
  shrinkable and growable element: the step indicator sits adjacent to the panel
  controls at the end of the row rather than floating between the two, so a
  short title does not leave the indicator stranded mid-row and the indicator's
  position does not move as the title's length changes.
- The title stays a shrinkable, truncating element with a minimum width of 88px.
- Both the title and the step name truncate with an ellipsis. The position count
  and the current-step marker do not shrink, so the count stays readable while
  the name truncates.
- The indicator's accessible name carries step name, number, and total, so
  truncation costs no information.

The narrow bound is the panel's 300px minimum width in the inline layout, which
is the narrower of the two by the pixel derived above. The header must be proven
there, not only at the 500px default and not only in the floating layout.

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

Desktop E2E covers the motivating scenario end to end: hide a step's column
through the board column visibility filter, open the preview on a task, open the
disclosure, confirm the hidden step is still listed, move the task to it, and
assert the task's step through the API. A second scenario resizes the preview to
its minimum width **in the inline layout**, with a step name long enough to drive
the indicator to its cap, and asserts single-row containment, both panel controls
clickable, and a title of at least the 88px
AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.3 requires — the literal floor, not
merely a non-zero width, because a non-zero assertion passes on a one-pixel
title and would leave the containment budget unproven at exactly the width it
was written for. The inline layout is specified because it is the binding case:
its header content box at the 300px minimum is 262px against the floating
layout's 263px, so a title floor proven there holds for both, while one proven
only in the floating layout does not. A third asserts the two-stage Escape.

Tablet E2E reuses the existing touch-drawer path against the preview indicator.

## Related decisions

No architecture decision record applies. This design reuses the compact stepper
and task-move contracts already established by
`REQ-UI-COMPACT-WORKFLOW-STEP-NAVIGATION-001`.
