---
status: draft
system: tasks
requirements:
  - REQ-TASKS-TASK-ACTIONS-MENU-001
  - REQ-TASKS-TASK-ACTIONS-MENU-002
  - REQ-TASKS-TASK-ACTIONS-MENU-003
  - REQ-TASKS-TASK-ACTIONS-MENU-004
---

# Task Actions Menu on Preview and Detail Surfaces System Design

## Purpose and boundaries

This design places the existing task actions menu on the desktop preview panel
and the desktop task detail top bar. It owns how a surface obtains the subject
task, how the shared entry list is built for that surface, and what each
confirmed action does to task state and navigation.

Contracts this design uses but does not own:

- The task update, archive, delete, move, detach, and link request contracts,
  owned by their existing task-system requirements.
- The archive confirmation contract, owned by
  [Archive confirmation](../requirements/archive-confirmation.md).
- The dropdown, context-menu, popover, and dialog primitives, owned by the UI
  system.
- The plugin task-menu action contract, owned by the plugin system.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-TASK-ACTIONS-MENU-001` | [Components and responsibilities](#components-and-responsibilities) |
| `REQ-TASKS-TASK-ACTIONS-MENU-002` | [Data and contracts](#data-and-contracts) |
| `REQ-TASKS-TASK-ACTIONS-MENU-003` ([own file](../requirements/task-actions-menu-outcomes.md)) | [Control flow](#control-flow) |
| `REQ-TASKS-TASK-ACTIONS-MENU-004` ([own file](../requirements/task-actions-menu-concurrency.md)) | [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

**Shared menu state (new).** The Kanban card today builds its entry list and
its dialog state inside a card-scoped hook that also carries board concerns:
board-supplied edit/delete/archive/move callbacks, the multi-selection, and the
card's own step list. Parity in
`AC-TASKS-TASK-ACTIONS-MENU-002.1` cannot be guaranteed by a second,
independent implementation, so the surface-independent part of that hook is
extracted into a shared hook that takes a subject task and a presentation, and
returns the entry list plus the dialog state. The card hook keeps the
board-only concerns (multi-selection move targets, board callbacks) and
composes the shared one. The extracted hook carries no pending state: each
caller supplies its own in-flight flags, which is what keeps the card's existing
board-scoped flags working untouched and what makes the surface-local rule below
fall out of the structure rather than needing enforcement.

This extraction sits on the board's hot render path, one instance per visible
card. It must not change the card's rendered entries, their identity keys, or
the number of store subscriptions per card. That is the constraint the
implementation is held to, and the existing card menu tests are the evidence.

**Preview trigger (new).** Renders inside the preview header's control group,
before Maximize. The preview already receives the full task object, so its
subject task needs no store lookup.

**Detail trigger (new).** Renders inside the top bar's right control group,
after the tools cluster. The top bar receives only identifiers, so it resolves
its board row from the store (see below) and degrades per
`AC-TASKS-TASK-ACTIONS-MENU-002.5` when the row is absent.

**Dialog host (new, one per surface).** Mounts the confirmation and link
dialogs the entries open. Each surface hosts its own so a dialog outlives the
menu that opened it and is not nested inside a portal that unmounts on select.

## Data and contracts

**Subject task resolution.**

- Preview surface: the task object the panel already receives. When it is
  `null` the trigger is not rendered.
- Detail surface: the task identifier from the route, plus a board-row lookup
  against the workflow snapshots with the flat task list as fallback (the same
  lookup the sidebar and the shared archive-confirmation hook already perform
  from inside the detail view, so the snapshots are populated there).

**Entry availability tiers.** Two tiers, which is what makes
`AC-TASKS-TASK-ACTIONS-MENU-002.5` implementable:

- *Identifier-only*: Archive, Delete. Available whenever a task identifier is
  known.
- *Board-row*: Edit, Move to, Send to workflow, Link, Detach from parent. Each
  needs fields carried only by the board row (workflow membership, step,
  repositories, parent task id), and is omitted while that row is unresolved.

The tiers describe availability, not precedence. An archived subject is filtered
by the archived rule first, which is why Archive -- an identifier-only entry --
is still absent for it.

The tiers are evaluated live. The entry list is derived from store state on each
render rather than captured when the menu opens, so a board row that resolves
while the menu is open promotes the board-row entries in place
(`AC-TASKS-TASK-ACTIONS-MENU-002.6`). Nothing needs to close and reopen the menu
to converge. Two things do close it: the subject task disappearing
(`AC-TASKS-TASK-ACTIONS-MENU-004.5`) and the subject being replaced by a
different task (`AC-TASKS-TASK-ACTIONS-MENU-004.5a`). Losing the board row is
neither, and demotes entries in place.

**There is no memoized-at-open tier.** Plugin entries are live on exactly the
same terms as every other entry (`AC-TASKS-TASK-ACTIONS-MENU-002.8`), including
inside a menu that is currently open. This matches the card rather than relaxing
against it: `usePluginRegistry()` is a `useSyncExternalStore` subscription that
re-renders the card on any registry mutation, and the card's entry list is
rebuilt unmemoized on each render, so an action a plugin registers after the
card mounted already appears and one whose plugin was just disabled already
stops appearing. A single liveness rule for every tier is also the cheaper
contract to hold: a plugin-only exception would need its own snapshot point,
its own invalidation, and its own tests, to buy nothing a user can see.

**Ordering.** The entry order is fixed by the shared builder, so parity is
structural rather than asserted per surface.

Step ordering is ascending `position`, tie-broken by ascending step `id`. That
rule is already implemented, once, by `sortWorkflowStepsByPosition`
(`lib/kanban/auto-hide-empty-columns.ts`), and the card's Move to submenu
already renders through it: the board derives its move targets with that helper
and passes them down, and the shared move-target derivation then uses that
supplied list verbatim for the current workflow. So the tiebreak is not new
behavior being introduced here, and the card's Move to submenu does not change.

The one place the shared derivation is weaker is *other* workflows: it sorts
their steps with `position` alone. Adopting `sortWorkflowStepsByPosition` there
gives the new surfaces the same order the card's Move to already has, and
changes the card's Send to workflow submenu only where two steps of a
non-current workflow share a `position` -- an order that is arbitrary today.
`AC-TASKS-TASK-ACTIONS-MENU-002.10` names that as its one permitted exception.

Two exclusions travel with the target list. User-hidden steps are filtered out
for the current workflow and not for other workflows, which is what the card
does today and what `AC-TASKS-TASK-ACTIONS-MENU-002.3b` reproduces rather than
corrects. The orphan sentinel `__kandev_orphan__` ("Needs Reassignment") is a
display-only node synthesized per view and never reaches a snapshot, so it is
absent from these surfaces' targets by construction; the criterion states it so
that a future view-specific target list cannot leak it in.

Workflow ordering is the workflow collection's own order, minus hidden
workflows, unchanged.

**Plugin action groups.** Only the `primary` group crosses to these surfaces.
Group `edit` actions, which the card nests under its Edit entry, are documented
as card-only (`docs/plans/plugins/PLUGIN-API.md`), and this design does not
widen that contract from the task system's side, so the Edit entry here is
always the flat item and never the card's submenu form
(`AC-TASKS-TASK-ACTIONS-MENU-002.2a`). Concretely, `buildEditMenuEntry` returns
a flat item when no `edit`-group action is visible and a submenu when one is;
these surfaces take the flat form unconditionally rather than calling that
builder with the plugin context.

**Archived subject.** The archived case reaches only the detail surface: the
board excludes archived tasks, so the preview panel cannot hold one. That same
exclusion is why an archived subject is normally board-row-unresolvable too, so
the archived rule and the unresolved-row rule overlap by default rather than by
accident. `AC-TASKS-TASK-ACTIONS-MENU-002.4a` resolves the overlap in one
direction: archived wins, and Archive is not offered on a task that is already
archived.

The archived entry set is therefore the admitted plugin primary actions, a
separator, and Delete -- stated in full because the card menu has no archived
branch for it to inherit from (`buildKanbanCardMenuEntries` takes no archived
input). "Admitted" is the ordinary `visible(context)` result in registration
order; the plugin task-menu context carries no archived signal and this design
does not add one, so archived-awareness stays the plugin's own concern.

The detail surface already receives `isArchived`, so no new input is needed to
evaluate this branch.

**Accessible names and test ids.** Both triggers use the existing
`More options` string. Because the preview renders beside a board whose cards
carry the same accessible name, each trigger also carries a surface-specific
test id (`task-preview-actions-menu`, `task-topbar-actions-menu`) so a test can
address one surface unambiguously.

**Localization.** No new key. Every entry label and both trigger names already
exist in `en`, `pt-pt`, `zh-cn`, `zh-tw`, `zh-hk`, and `pseudo`.

## Control flow

1. The user activates a surface's trigger. The surface opens its menu. No
   navigation, no active-task change, no preview close: the trigger stops
   pointer and click propagation so the surrounding header controls and the
   board's card click dispatch never see the event.
2. Selecting an entry closes the menu and either opens a dialog hosted by that
   surface or dispatches a move.
3. Archive and Delete open the shared confirmation; nothing is requested until
   the user confirms.
3a. Except when the archive-confirmation preference is off. That preference is
   read *inside* the shared confirmation component, not by its callers: when it
   is disabled the component renders no dialog, immediately invokes its confirm
   callback with no cascade, and closes. A surface that mounts the same shared
   confirmation therefore inherits the preference-honouring path for free, which
   is why `AC-TASKS-TASK-ACTIONS-MENU-003.1a` is parity with the card rather
   than an exception to it: the card mounts that same component. Neither new
   surface reads the preference itself, and this design adds no second bypass.
4. On confirmed Archive or Delete:
   - Preview surface: the removal prunes the task from the board collections.
     The preview's existing missing-subject guard then closes the panel. No
     new navigation code path is introduced.
   - Detail surface: the removal routes through the existing
     archive-and-switch and remove-from-board flows, which pick the next
     eligible task or fall back to the task overview.
4a. Escape while a preview menu is open must close only the menu
   (`AC-TASKS-TASK-ACTIONS-MENU-001.11`). The preview panel's own Escape
   handler is currently a bare `window` keydown listener that does not consult
   `defaultPrevented`, so without a change it would close the panel on the same
   keypress that closes the menu. The panel's handler is the one that changes:
   it must ignore an Escape the open menu has already handled. The menu
   primitive's own Escape behavior is not modified.
4b. The preview panel re-renders with a different subject task rather than
   unmounting, so an open menu can outlive the task it was opened on, and its
   terminal entries include Delete. Both triggers therefore use the default
   modal dropdown behavior the card already uses, so a single outside click
   dismisses the menu rather than also landing on another card; and the preview
   additionally closes its menu when the subject's identity changes
   (`AC-TASKS-TASK-ACTIONS-MENU-004.5a`). Modality alone is not relied on: it is
   a presentation default a later `modal={false}` could remove, whereas the
   identity check is behavioral. That check compares the subject's task *id*
   across renders, never the identity of the task object: `useSelectedTask`
   memoizes on the board's task collections and returns a freshly constructed
   object, so its result gets a new reference whenever any task on the board
   changes while the selected id stays put. Keying the close on the object would
   therefore close the menu on unrelated board churn and contradict
   `AC-TASKS-TASK-ACTIONS-MENU-002.6`, which requires an open menu to survive an
   in-place entry update. The detail route remounts on navigation and needs
   neither.
5. On a move: the subject task alone is moved. Both surfaces stay on the task.
   The detail top bar's stepper re-reads the current step from state.
5a. If the board row is lost while a Move to or Send to workflow submenu is
   open, the live demotion of `AC-TASKS-TASK-ACTIONS-MENU-002.6` removes the
   submenu's parent entry, so the submenu closes with it and the top-level menu
   stays open on the identifier-only entries. A move already in flight is not
   cancelled: it was dispatched against the server, which remains the arbiter,
   and it lands or fails on its own terms
   (`AC-TASKS-TASK-ACTIONS-MENU-004.1c`). This is a rare flash rather than a
   state machine -- the row is lost by a store update, and the alternative,
   pinning an open submenu to a target list the store no longer backs, would
   mean offering move targets that may no longer exist.
6. Edit opens the existing edit dialog seeded from the subject task; save uses
   the existing task update contract.

## Failure and recovery

**In-flight state is surface-local, and that is a decision, not an oversight.**
Each surface owns the pending flag for the requests it starts. There is no
task-keyed pending registry, and this design does not add one
(`AC-TASKS-TASK-ACTIONS-MENU-004.1b`).

The alternative -- lifting archive/delete/detach pending state into shared,
task-keyed store state so every surface showing the task disables together --
was rejected. It buys strict parity during a window that is hard to even
observe (`AC-TASKS-TASK-ACTIONS-MENU-004.2` closes the menu on a terminal
activation, and archive/delete/detach are terminal, so the
disabled state is only reachable by reopening mid-flight), and it pays for that
with a new piece of client state architecture on the board's hot render path.
It would also be a divergence dressed as parity: the card has no such registry
either. `isDeleting` and `isArchiving` arrive as props from board-level state
(`deletingTaskId === task.id` in `virtualized-column-task-list.tsx` and
`swimlane-graph2-content.tsx`), and `isDetaching` comes from `useDetachTask`,
whose `detachingTaskId` is a `useState` inside the hook and therefore local to
each caller. Two cards for different tasks already do not share it, and nothing
outside the board can read it.

The consequence is explicit and testable: a card mid-archive and a preview panel
open on that same task will show different enabled states until the request
settles. `AC-TASKS-TASK-ACTIONS-MENU-002.1` names this as its one in-flight
exception rather than leaving the parity rule to over-claim.

- In-flight archive, delete, and detach requests mark that surface's entries
  disabled, and the menu is already closed by the selection, so a second
  request cannot be started from the same menu. The disabled state is therefore
  observable by reopening the menu mid-flight, not by watching the menu that
  dispatched the request.
- Move to, Send to workflow, and Link are outside that rule by decision, not by
  omission (`AC-TASKS-TASK-ACTIONS-MENU-004.1a`). Link starts no task-state
  request on select -- it opens a dialog, which owns its own submit state. Move
  carries no menu-level pending state on the card either, so adding one here
  would be a new divergence rather than parity. Two moves issued in quick
  succession are two independent requests and the server arbitrates, which is
  the same rule the detail surface's own workflow stepper already applies with
  its last-request-wins guard.
- Two surfaces acting on one task issue independent requests. The server is the
  arbiter: the losing request fails and raises the existing per-action failure
  feedback. No new error surface, no client-side lock across surfaces.
- Detach is the exception, because it is idempotent rather than one-shot
  (`AC-TASKS-TASK-ACTIONS-MENU-004.3b`). Two existing layers already guarantee
  that, and this design leans on both rather than adding a third: the client
  detach request layer keys in-flight requests by task id and hands a second
  caller the existing promise instead of issuing another request, and the detach
  endpoint treats a detach of an already-parentless task as success with no row
  update. A second detach of one task therefore produces no second request and
  no failure feedback. This is a REQUEST-level coalesce and does not contradict
  `AC-TASKS-TASK-ACTIONS-MENU-004.1b`, which governs the per-surface pending
  flag behind the disabled state, not the request layer.
- When the losing request fails *because* the subject was archived or deleted by
  another actor, the subject is gone, so the surface's missing-task behavior
  wins over the keep-the-surface-open rule
  (`AC-TASKS-TASK-ACTIONS-MENU-004.4a`). The failure feedback is raised at
  application level, so it still reaches the user after the preview has closed
  or the detail route has switched away.
- **That cause is detected from store state, not from the failed response, and
  that is the authoritative mechanism rather than an incidental race.** The
  surface never inspects the rejected request to decide whether the subject
  survived; it reacts to the subject leaving the store, which is the same signal
  `AC-TASKS-TASK-ACTIONS-MENU-004.5` already runs on. The response could not
  carry that decision anyway without a backend change this capability does not
  make: a delete of a removed task resolves through the handler's not-found
  branch, but the already-archived sentinel matches none of the handler's typed
  branches and is answered as an undifferentiated server error, so the archive
  path has no distinguishable losing-race status to read. Ordering between the
  rejection and the store update is therefore not load-bearing. If the rejection
  lands first, the surface stays open, shows the failure feedback, and closes
  when the store update arrives; if the store update lands first, the surface is
  already gone and the application-level feedback still reaches the user. If the
  store update never arrives, the surface simply stays open under
  `AC-TASKS-TASK-ACTIONS-MENU-004.4`, which is the safe default.
- A failed request leaves the task at its last confirmed state and leaves the
  surface open on it. The archive-and-switch flow already restores the previous
  active task and route when its archive request rejects after an optimistic
  switch.
- A task removed by another client is handled by each surface's existing
  missing-subject behavior. The menu is closed by then, because the removal is
  observed through state rather than through a menu interaction.

## Persistence

None. This capability adds no storage, no migration, and no restart behavior.
All state is client-side and derived from the existing task store.

## Security

None added. Every action reuses its existing request, so backend authorization
is unchanged and no surface bypasses it.

## Observability

None added. Failures surface through the existing per-action feedback, and
failed archive and delete requests keep their existing console diagnostics.

## Testing

- Unit: shared builder parity for a not-archived task, an archived task, and an
  unresolved board row; the step ordering tiebreak.
- Component: each trigger renders, opens, closes on Escape with focus returned,
  and does not activate its neighbouring controls.
- End-to-end: the two surfaces are user-visible, so each gets a scenario that
  opens the menu by its surface test id and confirms Archive, asserting the
  post-action outcome named in `REQ-TASKS-TASK-ACTIONS-MENU-003`. The existing
  Kanban card-menu scenarios must continue to pass unchanged, as the evidence
  for `AC-TASKS-TASK-ACTIONS-MENU-002.10`.
