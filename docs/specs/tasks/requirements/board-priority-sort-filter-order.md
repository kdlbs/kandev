---
status: active
system: tasks
created: 2026-09-03
owners:
  - kandev
---

# Board priority sort and filter ordering Requirements

## Overview

Part of the three-document contract headed by
[Board priority sort and filter](board-priority-sort-filter.md), which owns the
overview, terminology, dependencies, the ordering decision, and the priority
filter requirement. Read it first: its `## Terminology` defines every term used
here, and its `## The ordering decision` records the product decision this
requirement implements.

This document carries the single requirement that orders each workflow step's
cards by priority. Its sibling
[Board priority sort and filter view state](board-priority-sort-filter-view-state.md)
carries the requirements about the two stored view values. The
`## Out of scope` exclusions live in **this** document and govern all three
files. `## Prior art` lives in the head document. The division
into three documents is driven by the specification size limit, which the set
exceeds as one file, not by a boundary in the contract. The three documents are
one contract and none is complete alone.

## Requirements

### REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-002: A person can order each step's cards by priority

**Intent:** Let the most urgent card in a step be the first one read, without
changing what the board means or what any automation will pick up next.

**User story:** As someone choosing what to start next, I want the most urgent
card at the top of each column, so that the board's reading order matches the
order I should work in.

#### Acceptance criteria

- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.1:** The display surface shall offer
  a board sort control presenting exactly two options, shown by their localized
  labels and persisted as the board sort tokens `created_desc` and
  `priority_desc`.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.2:** The board sort token shall
  default to `created_desc`, which shall order cards exactly as the board orders
  them before this capability. A person who never opens the sort control shall
  observe no change in board order, in any view. `created_desc` names **each view's
  existing native order**, not one shared comparator: `createdAt` descending in the
  kanban and mobile column views, and workflow-step index then `position` ascending
  in the pipeline view, which never reads `createdAt` at all. Implementing
  `created_desc` as a single comparator applied uniformly to every view would
  silently reorder the pipeline view by `createdAt` and shall not satisfy this
  criterion; under `created_desc` each view's comparator is left untouched.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.3:** When the board sort token is
  `priority_desc`, the system shall order the cards within each workflow step by
  priority rank, `critical` first, then `high`, then `medium`, then `low`, then
  unranked tasks last. Unranked tasks shall be ordered last rather than being
  hidden, omitted, or coerced to `medium`.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.4:** When the board sort token is
  `priority_desc`, each view shall order cards by a **total** key sequence, so that
  no two cards are ever left in an order the sequence does not determine:
  - kanban and mobile column views: priority rank, then `createdAt` descending,
    then task `id` ascending;
  - pipeline view: workflow-step index, then priority rank, then `position`
    ascending, then task `id` ascending. "Workflow-step index" is the step's index
    in the board's currently displayed step order, the order the pipeline view
    already uses, not the step's stored `position`; the two differ whenever a step
    is hidden. The step index remains the outermost key, so priority reorders
    cards **within** a step and never regroups them across steps.

  Task `id` ascending is the final key in every view and is required rather than
  decorative, because neither preceding key is unique: `compareTasksByCreatedDesc`
  compares two equal, absent, or unparseable timestamps as equal, and
  `tasks.position` carries no uniqueness constraint in any dialect. Relying on the
  input order or on sort stability in place of the named final key shall not
  satisfy this criterion. Cards of equal priority rank retain the order their view
  gives them under `created_desc` only where that view's own keys determine one;
  where those keys tie, `id` ascending decides, and the resulting order may differ
  from the order those cards happen to sit in today. The retain-order promise in
  `## The ordering decision` is bounded the same way and does not override this key
  sequence. Subject to that, turning the sort on can only lift higher-priority cards
  and never scrambles anything else. This total order applies
  to `priority_desc` only; `created_desc` leaves each view's native comparator
  untouched, per `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.2`.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.5:** The board sort token shall
  apply in every view that orders tasks within a workflow step, so that a
  selected sort is never silently inert in one view. The views are exactly the
  entries of `VIEW_REGISTRY` (`apps/web/lib/kanban/view-registry.ts`), which is the
  authority on what counts as a view: the kanban view, which renders the mobile
  column surface at mobile breakpoints, and the pipeline view. A component that
  sorts tasks within a step but is absent from that registry, `swimlane-graph-content.tsx`
  being the one such component today, is not a view, is not reachable by a person,
  and is outside this capability. Switching views shall not change or clear the
  selected sort token.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.6:** Selecting or changing the board
  sort token shall not write, recompute or reorder any task's `position`, shall
  not move a task between workflow steps, shall not change any other task field,
  and shall not start, stop or otherwise disturb any session. Ordering is a
  property of the view only.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.7:** While the board sort token is
  `priority_desc`, dragging a single card to another workflow step shall compute
  the same target `position` it computes under `created_desc`, and shall place
  the card in the same workflow step. Neither the active sort nor an active
  priority filter shall be an input to that computation, which reads the
  workflow's full task set rather than the displayed one. Bulk move of a
  multi-selection is governed by `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.10`
  instead, and the two shall not be collapsed into one rule.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.8:** Selecting, changing, or merely
  holding the board sort token shall not, by itself, change which task any
  WIP-queue pull, agent start or other automation selects. The backend's existing
  pull ordering, `position` ascending then a priority `CASE` then `created_at` then
  `id`, is unchanged by this capability, and no query gains a parameter. This
  criterion governs the sort token alone. A bulk move performed while
  `priority_desc` is active does write `position`, per
  `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.10`, and because the pull reads
  `position` as its first key, the positions such a move writes can change which
  task the pull selects next. That is the accepted and intended consequence of a
  person explicitly moving cards, not a side effect of the sort, and the two shall
  not be conflated: absent a position-writing action, the sort is inert to every
  automation.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.9:** Selecting the board sort token
  that is already selected shall complete without error and leave the order
  unchanged. Repeating a selection any number of times shall leave the board in
  the same state as applying it once.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.10:** Range selection and bulk move
  shall operate over the order the board is currently displaying, so a range spans
  the cards visibly between its endpoints and a bulk move assigns sequential
  positions in that same order. The order they operate over shall be **derived from
  the active board sort token and the active view**, never from a fixed
  created-descending order: under `priority_desc` a bulk move shall assign positions
  in the priority-refined total order of
  `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.4`, and under `created_desc` in that
  view's native order per `002.2`. A display-order helper that does not read the
  sort token satisfies neither this criterion nor `002.4`; the existing
  `sortByDisplayOrder` in `apps/web/hooks/use-task-multi-select.ts` is fixed to
  created-descending today and shall be made sort-token and view aware rather than
  left as it is. Neither range selection nor bulk move shall operate over an order
  the person cannot see.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.11:** A bulk move, and every other
  multi-selection action, shall act only on selected tasks the board is currently
  displaying. The multi-selection actions are exactly those `useBulkOperations`
  (`apps/web/hooks/use-task-multi-select.ts`) exposes, that hook being the
  authority on the set in the same way `VIEW_REGISTRY` is the authority on the
  view set under `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.5`; today the set is
  exactly `bulkDelete`, `bulkArchive` and `bulkMove`. An action added there later
  inherits this criterion. The partial-failure semantics of these actions are
  pre-existing and unchanged by this capability. When a priority filter, or a priority change under an active filter,
  hides an already-selected task, that task shall not be moved and shall not consume
  an index in the sequence of positions assigned to the cards that were moved. A
  task the person cannot see shall not be rewritten by an action they aimed at the
  cards they can see. Hiding a selected task shall not clear the rest of the
  selection, and revealing it again shall not retroactively enrol it in a move that
  has already completed.

  Eligibility shall be determined **once, at the moment the action is invoked**, from
  what the board is displaying at that moment, and shall not be re-evaluated per
  request while the action is in flight. A selected task hidden after invocation,
  whether by a filter change or by a priority change under an active filter, shall
  still be acted on, because the person aimed the action at the set they could see
  when they triggered it; a task revealed after invocation shall likewise not be
  added to it. This keeps the acted-on set deterministic rather than dependent on
  request timing, and it is what `runBulk`
  (`apps/web/hooks/use-task-multi-select.ts`) already does, reading the selection
  once and issuing every request from that one snapshot. Re-evaluating visibility
  per request, so that a card's fate depends on whether its own request happened to
  still be in flight when an unrelated update arrived, shall not satisfy this
  criterion. The preceding paragraph therefore governs tasks already hidden
  **before** invocation and this one governs the in-flight window, which matters
  most for `bulkDelete` and `bulkArchive`: they are destructive and, unlike
  `bulkMove`, leave no sequence of written positions in which an omission would
  later be visible. An invocation whose snapshot is empty shall be a no-op.

## Out of scope

Each exclusion below is a decision, not an omission.

- **A `priority_asc` board sort token.** "Least urgent first" answers no
  question a person asks of a triage board, and every token added must be
  normalized by client and server for as long as the setting exists. Adding one
  later is additive.
- **Grouping the board by priority.** The board is already grouped, by workflow
  step. A second grouping axis is a different view, not a sort option.
- **Priority sort on the `/tasks` list view.** Its own persisted vocabulary,
  `TASKS_LIST_SORT_OPTIONS`, has no priority field either. A coherent follow-up
  of the same shape, but a different surface with a different persisted value
  and default.
- **Any backend sort or filter query surface.** `GET /workflows/:id/snapshot`
  and `GET /workspaces/:id/snapshot` gain no parameter. The board sends no
  `task_limit`, so the client already holds every task in the workflow and a
  client-side predicate is complete rather than operating over a truncated page.
  Should the board ever paginate, this must be revisited, because filtering a
  truncated page silently under-reports.
- **Correcting the occupancy-set inconsistency.** Repository and plugin filters
  feed the occupancy set while search and hidden steps do not, so a repository
  filter can auto-hide a column that a search cannot. That asymmetry predates
  this capability, which neither relies on it nor worsens it:
  `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.6` puts the priority filter on the
  search side. A plausible follow-up; changing it would alter behavior no
  criterion here covers.
- **Within-step manual reordering by drag and drop.** The board has no such
  capability today and this does not add one. Its absence is why the ordering
  decision above is available at all.
- **Any change to `tasks.position`, to what writes it, or to the backend
  WIP-queue pull ordering.** Named as a decision in
  `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.6` and `002.8` rather than left
  silent, because the original framing of this follow-up assumed the opposite.
- **Any schema or migration work on `tasks`.** The `priority` column already
  exists on both store paths and is not touched; neither is either path's column
  default, which differs between them (`0` on SQLite, as `## Terminology`
  records, and `medium` on Postgres), nor the `CHECK` constraint, which exists on
  the Postgres path only. This capability adds no constraint to the SQLite
  `tasks` table: it stays unconstrained, which is why an unranked task is a
  persistable state these criteria handle rather than one the schema prevents.
  The two new values live in the existing user-settings record, where
  `tasks_list_sort` and `kanban_hidden_step_ids` already live.
- **Rendering the priority indicator on the card, setting priority at creation,
  and changing priority from the card menu.** Owned by
  `requirements/task-priority-visibility.md`. This capability reads the value
  that one makes visible and writable; it adds no writer of its own.
- **MCP or REST support for filtering or sorting by priority.** No agent-facing
  or API-facing contract changes. These are view options held per user.
- **The Office task UI.** Nothing here changes it, and the two vocabularies stay
  separate.
- **A default sort or filter per workflow, step, workspace or role.** Both
  values are per user and global, per
  `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.2`.
- **A URL parameter overriding either value.** The `/tasks` list view supports
  `?sort=`; the board does not. Adding one would introduce a second source of
  truth whose precedence against the persisted value is a contract this
  capability does not need.
- **Bulk priority changes from a filtered board.** Multi-select bulk actions
  exist, but changing priority across a selection is a separate interaction with
  its own partial-failure semantics.
- **An unranked option on the priority filter.** The control offers exactly the
  four tokens, so a non-empty selection always hides unranked tasks and only
  clearing the filter reveals them, per
  `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.10`. Unranked is not a priority a person
  assigns; it is the absence of one, arising either transiently before the boot
  payload carries priority or persistently from a row written before the priority
  writer existed, since the SQLite `tasks` table does not constrain the column. A
  fifth option would put a storage artifact in a product vocabulary that new tasks
  never enter, because the task service applies its `defaultPriority` constant
  (`medium`) whenever a create request carries no priority. That default is the
  service's, not the store's: on SQLite the column default is `0`, as
  `## Terminology` records, and only the Postgres path defaults to `medium`. A
  create path bypassing the service therefore writes an unranked row, which is a
  further reason the sort ranks these tasks last rather than hiding them. A fifth
  option would also import the very five-token shape this capability deliberately
  keeps separate from Office's. The
  sort still ranks these tasks last rather than hiding them
  (`AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.3`), so they stay reachable on a board
  with no filter applied. Backfilling the column, or adding a SQLite `CHECK` to
  match Postgres, is a data-migration question this view capability does not
  answer; it is a plausible follow-up and would make this exclusion moot.
- **A failure-reporting persistence path for these two values.** A pipeline that
  surfaces a failed write exists (`apps/web/lib/user-settings-sync.ts`,
  `updateUserSettingsWithRetry`, which throws once retries are exhausted), but it is
  not the pipeline that carries the board's other display settings. Routing only
  these two values through it would put two persistence spellings in one settings
  record, which is the same defect this capability rejects for sort vocabularies;
  changing the shared pipeline's failure semantics would alter behavior for the
  repository filter, hidden step ids and the preview toggle, none of which any
  criterion here covers. So `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.6` is written
  to the shared channel's actual best-effort semantics. Giving every setting in this
  record a real failure surface is a coherent follow-up on its own terms.
- **Priority on ephemeral tasks.** Quick-chat and other ephemeral tasks are
  filtered out before the board renders, so they have no card to sort or filter.
