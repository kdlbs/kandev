---
status: draft
system: tasks
created: 2026-09-03
owners:
  - Kandev
---

# Task Actions Menu on Preview and Detail Surfaces Requirements

## Overview

Every task action Kandev offers (edit, move to step, send to workflow, link an
external item, run a plugin task action, detach from parent, archive, delete) is
reachable from a Kanban card, through its visible "more options" button or its
context menu. Two other surfaces show a single task and offer none of them: the
**desktop task preview panel**, whose header renders only Maximize and Close,
and the **desktop task detail top bar**, which renders metrics, plugin actions,
PR/MR/issue status, unarchive, and tools, but no task actions.

From either surface, archiving the task means returning to the board to find its
card, right-clicking a task switcher sidebar row that only appears on hover, or
knowing the command palette shortcut. This requirement makes the same actions
reachable from both surfaces through the affordance the card already uses.

The task system owns this contract, not the UI system. Per
[structure and ownership](../../guide/structure-and-ownership.md), user
visibility does not make a capability UI-owned: the contract is *which task
actions are reachable, from which task surface, and what each does to task state
and navigation*, which has no meaning without task state. The UI system keeps
the menu, dropdown, and popover primitives this composes.

## Terminology

- **Actions menu:** The menu of task actions this requirement places on a
  surface, opened from a visible trigger.
- **Card actions menu:** The existing menu opened from a Kanban card's visible
  "more options" button. It is this capability's reference for content and
  ordering.
- **Preview surface:** The desktop task preview panel rendered beside the
  Kanban board. It has no mobile equivalent.
- **Detail surface:** The desktop task detail top bar rendered above an open
  task. It has a separate mobile top bar, which is out of scope.
- **Subject task:** The single task an actions menu acts on. On the preview
  surface it is the previewed task; on the detail surface it is the open task.
- **Control group:** A cluster of controls rendered together at one end of a
  surface's header or top bar. On the preview surface it is the single group
  holding Maximize and Close; on the detail surface it is the top bar's
  right-hand group, whose last member today is the tools cluster.
- **Board row:** The task's entry in the board's task collections. Entries such
  as Edit, Detach, Move to, Send to workflow, and Link need fields that only
  the board row carries.

## Prior art

**Wiki leg (receipt).** Searched the `@henry`-pinned vault
`OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`, collection
`QMD_WIKI_COLLECTION="wiki"`. No `qmd` MCP server and no `obsidian-wiki` CLI, so
both the semantic and GraphRAG passes were skipped; the grep fallback then hit
`Operation not permitted` on every content read while `ls -la` on the same paths
succeeded. **This leg returned nothing useful.**

**Cross-product leg (receipt).** Intended query: `saas-kb` `search_fsm_docs`,
`category: "ai_sdlc"`. Server and tool not connected to this session.
**This leg returned nothing useful.**

**In-product prior art (read directly; the basis for the decisions below).**

- The card actions menu is already built by a pure, surface-independent builder
  emitting an entry list that a dropdown or a context-menu renderer draws.
  Bringing it to a new surface is wiring, not a new menu.
- [Sidebar Task Editing](sidebar-task-edit.md) is the same shape of change,
  owned by this system, and set the precedent that a second surface reuses the
  first's dialog and update contract rather than inventing its own.
- The task switcher sidebar already exposes task actions from a visible dots
  button, but only on hover, and only per sidebar row. Its archive flow is the
  same shared confirmation the PR banners and command palette use.
- The command palette already offers Archive from the detail surface, but it is
  keyboard-driven and not a visible affordance, so it does not satisfy this.

**What this capability does differently:** it standardises on the *card's*
entry set and ordering rather than the sidebar's divergent one, because two
menus that already disagree should not become four.

## Requirements

### REQ-TASKS-TASK-ACTIONS-MENU-001: Actions menu trigger on the preview and detail surfaces

**Intent:** A user looking at a single task can reach that task's actions from
the surface they are already on, without returning to the board, hovering to
reveal a control, or knowing a keyboard shortcut.

#### Acceptance criteria

- **AC-TASKS-TASK-ACTIONS-MENU-001.1:** When the preview surface has a subject
  task, the system shall render an actions-menu trigger in the preview header's
  control group, positioned before the Maximize control, with the accessible
  name `More options`.
- **AC-TASKS-TASK-ACTIONS-MENU-001.2:** When the detail surface has a subject
  task, the system shall render an actions-menu trigger in the top bar's right
  control group, positioned after every other control in that group, with the
  accessible name `More options`.
- **AC-TASKS-TASK-ACTIONS-MENU-001.3:** The system shall render each trigger at
  full opacity whenever its surface is rendered, without requiring pointer
  hover or keyboard focus to reveal it.
- **AC-TASKS-TASK-ACTIONS-MENU-001.4:** The system shall expose each trigger
  with `aria-haspopup="menu"` and an `aria-expanded` value that is `false` while
  its menu is closed and `true` while its menu is open.
- **AC-TASKS-TASK-ACTIONS-MENU-001.5:** The system shall give the preview
  trigger the stable test id `task-preview-actions-menu` and the detail trigger
  the stable test id `task-topbar-actions-menu`, so that a test can address one
  surface while the board's card triggers carry the same accessible name.
- **AC-TASKS-TASK-ACTIONS-MENU-001.6:** When the preview surface has no subject
  task, the system shall render no actions-menu trigger, and shall leave the
  Close control unchanged.
- **AC-TASKS-TASK-ACTIONS-MENU-001.7:** When the detail surface has no subject
  task identifier, the system shall render no actions-menu trigger.
- **AC-TASKS-TASK-ACTIONS-MENU-001.8:** When the user activates a trigger, the
  system shall open that surface's actions menu right-aligned to the trigger,
  and shall not navigate, shall not change the active or previewed task, shall
  not close the preview panel, and shall not activate any other control on the
  surface.
- **AC-TASKS-TASK-ACTIONS-MENU-001.9:** When an actions menu is open and the
  user presses Escape, the system shall close the menu and return keyboard
  focus to the trigger that opened it.
- **AC-TASKS-TASK-ACTIONS-MENU-001.10:** The system shall introduce no new
  localization key for either trigger, reusing the existing `More options`
  string, which is already present in every shipped catalog.
- **AC-TASKS-TASK-ACTIONS-MENU-001.11:** When an actions menu is open on the
  preview surface and the user presses Escape, the system shall close only the
  menu and shall leave the preview panel open on the same subject task; a
  second Escape, with no menu open, shall then close the preview panel as it
  does today.
- **AC-TASKS-TASK-ACTIONS-MENU-001.12:** When a dialog opened from an actions
  menu closes, whether confirmed, cancelled, or dismissed, and its surface is
  still mounted on the same subject task, the system shall return keyboard
  focus to the trigger that opened the menu.
- **AC-TASKS-TASK-ACTIONS-MENU-001.13:** The system shall place each trigger in
  the document tab order at its rendered position within its control group, so
  a keyboard user reaches it by tabbing through that group and opens its menu
  with Enter or Space.

### REQ-TASKS-TASK-ACTIONS-MENU-002: Menu content parity with the card actions menu

**Intent:** A user learns one task menu. The same task offers the same actions,
in the same order, with the same labels, wherever the menu is opened.

#### Acceptance criteria

- **AC-TASKS-TASK-ACTIONS-MENU-002.1:** When an actions menu is open on a
  subject task that is not archived, the system shall present the same entries,
  in the same order, with the same labels, the same submenu nesting, and the
  same enabled or disabled state as the card actions menu presents for that
  same task at that same moment, except where
  AC-TASKS-TASK-ACTIONS-MENU-002.4 through
  AC-TASKS-TASK-ACTIONS-MENU-002.9 state otherwise, except for plugin
  `edit`-group actions (AC-TASKS-TASK-ACTIONS-MENU-002.2a), and except for the
  in-flight disabled state, which is per surface under
  AC-TASKS-TASK-ACTIONS-MENU-004.1b and so may differ from the card's for the
  duration of a request.
- **AC-TASKS-TASK-ACTIONS-MENU-002.2:** The order named in
  AC-TASKS-TASK-ACTIONS-MENU-002.1 is: Edit, Move to, Send to workflow, plugin
  primary actions, Link, Archive, Detach from parent, a separator, Delete. The
  system shall omit any of Move to, Send to workflow, Link, and Detach from
  parent whose availability condition in the card actions menu is unmet, and
  shall not reorder the entries that remain.
- **AC-TASKS-TASK-ACTIONS-MENU-002.2a:** The Edit entry on these two surfaces
  shall always be the flat Edit item, never the card's submenu form: the system
  shall not present plugin task-menu actions registered with group `edit` on
  either surface. Group `edit` is a card-only plugin contract and this
  requirement does not widen it; group `primary` is unaffected and appears per
  AC-TASKS-TASK-ACTIONS-MENU-002.2.
- **AC-TASKS-TASK-ACTIONS-MENU-002.3:** Within the Move to and Send to workflow
  submenus the system shall order steps by ascending `position`, breaking a tie
  by ascending step `id`. This is not a new rule: `sortWorkflowStepsByPosition`
  already implements it (`position` ascending, then `id.localeCompare`) and the
  card's Move to submenu already renders through it. The new surfaces adopt that
  helper rather than add a second ordering.
- **AC-TASKS-TASK-ACTIONS-MENU-002.3a:** Within the Send to workflow submenu the
  system shall order workflows in the order the workflow collection holds them,
  which is the order the card actions menu uses, and shall omit workflows marked
  hidden.
- **AC-TASKS-TASK-ACTIONS-MENU-002.3b:** The system shall exclude from a Move to
  target list every step the user has hidden for the subject task's current
  workflow, matching the card, whose current-workflow targets are already
  hidden-filtered. Within Send to workflow the system shall not filter another
  workflow's steps by hidden state, also matching the card. This asymmetry is
  inherited deliberately; see `## Out of scope`.
- **AC-TASKS-TASK-ACTIONS-MENU-002.3c:** The system shall never present the
  display-only orphan sentinel step (`__kandev_orphan__`, labelled "Needs
  Reassignment") as a move target on either surface.
- **AC-TASKS-TASK-ACTIONS-MENU-002.4:** When the subject task is archived, the
  system shall omit Edit, Move to, Send to workflow, Link, Detach from parent,
  and Archive from the actions menu, and shall present exactly, in this order:
  the admitted plugin primary actions, then a separator, then Delete. The card
  has no archived branch to inherit from, so the order is stated in full here;
  it is the card's own order with the omitted entries removed.
- **AC-TASKS-TASK-ACTIONS-MENU-002.4c:** When no plugin primary action is
  admitted for an archived subject, the system shall present Delete alone, with
  no leading separator, so the menu never opens on a separator.
- **AC-TASKS-TASK-ACTIONS-MENU-002.4a:** AC-TASKS-TASK-ACTIONS-MENU-002.4 takes
  precedence over AC-TASKS-TASK-ACTIONS-MENU-002.5 whenever both apply. An
  archived subject is normally board-row-unresolvable too, since the board
  excludes archived tasks; in that overlap the archived rule governs, so the
  system shall present Delete and not Archive.
- **AC-TASKS-TASK-ACTIONS-MENU-002.4b:** "The admitted plugin primary actions"
  means the ordinary `visible(context)` result for the `primary` group, in
  plugin registration order, evaluated against the unchanged plugin task-menu
  context. The system shall not add an archived field to that
  context, and shall not filter plugin actions by archived state: the context
  carries no archived signal, and a plugin that should hide itself for an
  archived task owns that.
- **AC-TASKS-TASK-ACTIONS-MENU-002.5:** When the subject task's board row is
  not resolvable, the system shall omit Edit, Move to, Send to workflow, Link,
  and Detach from parent, and shall present Archive and Delete, which need only
  the task identifier. This criterion yields to
  AC-TASKS-TASK-ACTIONS-MENU-002.4 when the subject is also archived.
- **AC-TASKS-TASK-ACTIONS-MENU-002.6:** The entry set shall be live, not
  snapshotted when the menu opens. When the subject task's board row becomes
  resolvable, or stops being resolvable, the system shall update that surface's
  entries in place, whether its menu is currently open or closed, without
  requiring a page reload and without closing an open menu. When the subject
  task itself is removed rather than its row merely becoming unresolvable,
  AC-TASKS-TASK-ACTIONS-MENU-004.5 governs and the menu closes; when the subject
  is replaced by a different task, AC-TASKS-TASK-ACTIONS-MENU-004.5a governs.
- **AC-TASKS-TASK-ACTIONS-MENU-002.7:** When the board holds a multi-task
  selection that includes the subject task, the system shall still act on the
  subject task alone, shall present the single-task labels rather than any
  count-bearing label, and shall leave the selection unchanged. Entry
  membership shall not depend on the selection either: the system shall still
  present Detach from parent for a subject that has a parent, even while a board
  multi-selection includes it. This removes the selection as a reason to omit
  Detach; it does not override AC-TASKS-TASK-ACTIONS-MENU-002.4 or
  AC-TASKS-TASK-ACTIONS-MENU-002.5, which still omit Detach for an archived
  subject and for an unresolved board row. The card suppresses Detach under a
  multi-selection because its own menu can act on that selection; these surfaces
  never can, so that cause is absent here.
- **AC-TASKS-TASK-ACTIONS-MENU-002.8:** When a plugin registers, is enabled, or
  is disabled while a surface is mounted, the system shall reflect that change
  in that surface's plugin entries immediately, including in a menu that is
  already open, and without closing it. Plugin entries are not a separate tier:
  they are live on the same terms as every other entry under
  AC-TASKS-TASK-ACTIONS-MENU-002.6, which is what the card already does, since
  it subscribes to the plugin registry and rebuilds its entries each render.
- **AC-TASKS-TASK-ACTIONS-MENU-002.9:** The system shall present no entry on
  these two surfaces that the card actions menu does not present for a single,
  unselected task, and shall present no bulk-selection variant of the menu.
- **AC-TASKS-TASK-ACTIONS-MENU-002.10:** The system shall leave the card actions
  menu, the card context menu, and the task switcher sidebar menu unchanged in
  entry membership, labels, top-level order, and behavior, with one named
  exception. Adopting `sortWorkflowStepsByPosition` inside the shared
  move-target derivation also applies its ascending-`id` tiebreak to the steps
  the card lists under Send to workflow, which are today sorted by `position`
  alone. That changes rendered order only where two steps of a non-current
  workflow share a `position` value, a case whose order is arbitrary today. No
  other card-facing change is permitted; the card's Move to submenu is
  unaffected, already sorting through that helper.

### Action outcomes and post-action navigation

`REQ-TASKS-TASK-ACTIONS-MENU-003` covers what each confirmed action does to task
state and navigation. It lives in
[Task Actions Menu Action Outcomes](task-actions-menu-outcomes.md).

### In-flight, concurrent, and failing actions

`REQ-TASKS-TASK-ACTIONS-MENU-004` covers in-flight, concurrent, and failing
actions. It lives in
[Task Actions Menu In-Flight and Concurrency](task-actions-menu-concurrency.md).

## Out of scope

- **The mobile task detail top bar.** Different chrome budget, and mobile
  already routes task actions through the task switcher sheet's per-row menus.
  A follow-up wanting parity there should start from that sheet's menu.
- **The mobile preview surface.** There is none: on mobile the board navigates
  straight to the task detail route and never renders the preview panel, so
  there is no surface to add a trigger to.
- **Changing the card actions menu, the card context menu, or the task
  switcher sidebar menu.** Their entry sets, ordering, and labels are frozen by
  this requirement, not revised by it.
- **Reconciling the card menu's entry set with the task switcher sidebar's
  divergent one.** The sidebar offers Pin, Rename, Create subtask, Duplicate,
  Color, and Nest, and omits several the card has. These surfaces standardise on
  the card's set; the divergence stays.
- **Bulk or multi-task actions from these surfaces.** Both surfaces address a
  single task; the board's multi-select toolbar remains the bulk path.
- **Unarchive.** The detail top bar already renders a dedicated Unarchive
  control for an archived task, and this requirement does not duplicate it into
  the menu.
- **A right-click context menu on either surface.** This requirement adds the
  visible trigger only; the card keeps its context menu.
- **Correcting the card's hidden-step asymmetry in move targets.** The card
  filters user-hidden steps out of Move to but not out of Send to workflow.
  `AC-TASKS-TASK-ACTIONS-MENU-002.3b` reproduces that asymmetry rather than
  fixing it, because fixing it would change card behavior that
  `AC-TASKS-TASK-ACTIONS-MENU-002.10` freezes. A follow-up should change both
  surfaces together and update the card's move-target tests.
- **Presenting plugin `edit`-group actions on these surfaces.** That group is
  documented as card-only, and `AC-TASKS-TASK-ACTIONS-MENU-002.2a` keeps it that
  way. Widening it changes a published plugin contract and every registered
  plugin's expectations, so it is a plugin-system decision, not this one's.
- **Any new backend endpoint, persistence model, permission, feature flag, or
  localization key.**
