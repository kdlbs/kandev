---
status: active
system: tasks
created: 2026-09-03
owners:
  - kandev
---

# Board priority sort and filter Requirements

## Overview

`requirements/task-priority-visibility.md` makes a task's priority visible on
its kanban card and settable by a person. It excludes sorting and filtering the
board by priority and names this capability as the follow-up that decides the
ordering contract first.

That exclusion rests on two premises about the board. Both were re-measured
against `a170366e3`, and **both are false**. They are corrected here because the
decision this capability exists to make was framed by them.

**Premise 1: "the board has no sort or filter surface of any kind today."** It
has a filter surface, described under `## Prior art`. This capability adds
sections to it rather than building one. There is no *sort* surface; that half
of the premise holds.

**Premise 2: "the board is manually ordered by drag and drop via
`tasks.position`."** The default view is not ordered by `position`.
`useTasksByStep` in `apps/web/components/kanban/swimlane-kanban-content.tsx`
orders each step's cards by `compareTasksByCreatedDesc`, that is `createdAt`
descending, and `swipeable-columns.tsx` does the same on mobile. Only the
pipeline view (`swimlane-graph2-content.tsx`) orders by `position`. Drag and
drop moves a card *between* steps and appends it (`calcNextPosition` returns the
target step's count); there is no within-step drag reordering and no task
reorder endpoint. `sortIdsByCreatedDesc` in `apps/web/lib/kanban/task-order.ts`
exists precisely to sort a selection into "the board's visible created-desc
order" before a bulk move assigns sequential positions.

So there is no manual within-step ordering to override or to tiebreak inside,
and the decision this card was filed to make is narrower than its brief assumed.
It is made under `## The ordering decision`.

This capability adds no migration, no snapshot query parameter and no new task
field: two user-settings values, two sections on an existing surface, and a
comparator.

## Terminology

- **Priority token:** one of `critical`, `high`, `medium`, `low`. The canonical
  four-value vocabulary; a wire and storage value, never translated. Defined by
  `requirements/task-priority-visibility.md`. The store does **not** enforce it
  everywhere: the `CHECK` constraint exists only on the Postgres path
  (`task_priority_postgres.go`) and on Office's separate table. The SQLite `tasks`
  table, which is the default store, declares `priority INTEGER DEFAULT 0` in
  `initTaskSchema` (`apps/backend/internal/task/repository/sqlite/base_schema.go`)
  with no `CHECK`, so on SQLite the vocabulary is a convention the writers uphold
  rather than an invariant the schema guarantees.
- **Unranked task:** a task whose priority is absent, empty, or not one of the four
  tokens. Because SQLite does not constrain the column, this is a **persistable**
  state and not merely a transient one: a row written before the priority writer
  existed carries the column default and reads back outside the vocabulary. It is
  additionally true of every task transiently on first paint until the fix in
  `## Dependencies` lands. Both are the same state and are governed by
  `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.10` and `002.3`. This capability never
  writes a priority, so it never converts an unranked task into a ranked one.
- **Priority rank:** `critical` before `high` before `medium` before `low`.
- **Board sort token:** the persisted value naming how cards are ordered within
  a step. One of `created_desc` or `priority_desc`. Stored, transmitted and read
  under the key `kanban_sort`.
- **Priority filter selection:** the persisted set of priority tokens selected.
  The empty set means no priority filtering. Stored, transmitted and read under
  the key `kanban_priority_filter_tokens`.
- **Display surface:** the desktop "Display Options" dropdown
  (`apps/web/components/kanban-display-dropdown.tsx`) and the mobile menu sheet
  (`apps/web/components/kanban/mobile-menu-sheet.tsx`). A change to one is a
  change to both.
- **Occupancy set:** the task set used for step occupancy rather than display,
  produced by `projectWorkflowTasks` in
  `apps/web/lib/kanban/task-projections.ts`. It drives auto-hide-empty-columns.

## Prior art

**Wiki leg: DID NOT RUN, vault unreadable.** Routed with `@henry`. Resolved
`~/.obsidian-wiki/config` to `config.henry`, giving
`OBSIDIAN_VAULT_PATH="/Users/henry/Documents/henry/wiki"` and
`QMD_WIKI_COLLECTION="wiki"`. Every read of that path returns `Operation not
permitted`, with the tool sandbox enabled and again with it disabled, so this is
macOS TCC protection on `~/Documents` rather than a harness restriction. No
`qmd` or `obsidian-wiki` binary on `PATH` and no `qmd` MCP server registered, so
there was no fallback transport and no degraded grep leg. This independently
reproduces the failure recorded in `requirements/task-priority-visibility.md`.
No wiki content informed this document.

**Vendor leg: DID NOT RUN, tool unreachable.** The `saas-kb` MCP server and its
`search_fsm_docs` tool are not registered in this session, so the `ai_sdlc`
category was not queried. No vendor claims informed this document.

**In-repo leg: read the three existing sort-and-filter implementations here.**
The leg that produced everything.

*The board's own display surface* already filters: a workflow filter, a
repository filter, a preview toggle and a plugin filter extension point
(`registerTaskFilter`) rendered as checkbox groups, mirrored on mobile, with
selections persisted through `useKanbanDisplaySettings` into server-held user
settings alongside `repositoryIds` and `kanban_hidden_step_ids`. That hook does
**not** carry `tasks_list_sort`. Despite sharing the same settings record, the
list sort is written by a separate narrow delta, `persistPreferences` in
`apps/web/app/tasks/tasks-page-client.tsx`, which sends only `tasks_list_sort`
and `tasks_list_group`. These are two different persistence paths with
different concurrency behavior, and this capability adopts the list view's
token *spelling* (below) while explicitly **not** adopting its *persistence
path*: both new values travel with `repositoryIds` and
`kanban_hidden_step_ids`, per `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.5`. Its
filter pipeline is `filterTasks` in `apps/web/lib/kanban/task-projections.ts`:
hidden steps, repositories, search text, then plugin filters, each a predicate
over tasks already in the store, so a priority filter is one more predicate in
that chain. That module already draws the distinction this capability needs,
returning `visibleTasks` and `occupancyTasks` separately, where occupancy
ignores search text and hidden steps but honors repository and plugin filters.

*The sibling task list view* owns this repository's sort conventions, in
`apps/web/lib/tasks/tasks-list-options.ts`. Three are adopted verbatim: sort
values are `<field>_<dir>` tokens rather than a field plus a direction control;
`parseTasksListSort` resolves an unrecognized value to the default rather than
failing, mirrored server-side by `NormalizeTasksListSort`; and every comparator
in `compareTasksForList` carries an explicit second key, so no two cards are
left in an unspecified order. That view has no priority sort either; see
`## Out of scope` in
[Board priority sort and filter ordering](board-priority-sort-filter-order.md).

*The Office task tree*, in `apps/web/app/office/tasks/`, is the closest
functional analogue. Its `matchesFilters` treats an empty selection as "no
filter" and otherwise requires membership, which is the filter semantics
adopted below, and its `FALLBACK_PRIORITY_ORDER` ranks `critical:0, high:1,
medium:2, low:3, none:4`, which is the rank adopted by
`AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.3` in [Board priority sort and filter
ordering](board-priority-sort-filter-order.md). Office is the model for
semantics, not for the control's shape: it pairs a sort field with a separate
asc/desc toggle and carries a fifth `none` token with workspace-overridable
labels, whereas kanban has exactly four tokens with no override. This
capability follows the list view's single-token convention instead, because
that is what is already persisted in the same user-settings record it writes
to, and two sort spellings in one settings object would be a defect.

## Dependencies

This capability cannot be verified on `main` alone. A scheduling constraint on
implementation, not an acceptance criterion.

This dependency is the *transient* source of unranked tasks, and it is not the only
source. Landing the branch below fixes first paint; it does not make unranked
impossible, because the SQLite column is unconstrained. See `## Terminology`.

`mapKanbanTaskState` in `apps/backend/internal/backendapp/boot_state_routes.go`
is a camelCase whitelist that does not list `priority`, so the Go boot payload
hydrating the board's first paint carries no priority for any task. On `main`
every task is therefore an unranked task until a later event corrects it, and a
priority sort or filter built on that base would be observably wrong on first
paint for every board.

Both the fix and the shared token module this capability reuses live on the
unmerged branch `feature/surface-task-priorit-x5h`, which carries
`requirements/task-priority-visibility.md` itself: `boot_state_routes.go` gains
`"priority": task.Priority` with `TestMapKanbanTaskStateIncludesPriority` as its
regression guard, and `apps/web/lib/kanban/task-priority.ts` provides
`KANBAN_PRIORITY_TOKENS`, `KANBAN_PRIORITY_LABEL_KEYS` and `isKanbanPriority`,
with the four priority labels added to `apps/web/src/locales/*/kanban.json` in
all five locales plus pseudo.

Implementation shall build on that work rather than duplicating it. If it has
not landed when implementation starts, that is a sequencing decision for the
plan, not a licence to re-declare the token list, the label keys or the
boot-payload field a second time.

## The ordering decision

**The board sort is a view lens. It never writes `position` and never mutates a
task.** Selecting `priority_desc` changes only the order cards are drawn in. It
is not a re-prioritisation of the queue and not an input to WIP-queue pulls or
to any agent's task selection. The backend pull queries in
`apps/backend/internal/task/repository/sqlite/task.go`
(`NextPullCandidateExcluding`, `NextQueuedTaskForStepExcluding`) keep their
existing `position ASC` then priority `CASE` ordering, untouched.

**Priority sort refines the existing order rather than replacing it.** Cards are
ordered by priority rank first, and cards of equal rank keep the order they have
today wherever today's comparator determines one: `createdAt` descending in the
kanban and mobile column views, `position` ascending in the pipeline view. Where
it determines none, because those keys tie, the final task `id` key of
`AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.4` decides rather than the incidental
input order; that is the only place this promise yields, and it yields to a
named key. Turning the sort on can only lift higher-priority
cards up; it never scrambles anything else. This is what "tiebreak inside the
existing order" means once the existing order is correctly identified, so it
honors the intent behind the original framing.

**The priority filter is a view lens too, scoped like search rather than like
the repository filter.** The board's existing filters split on whether they feed
the occupancy set. A priority filter belongs with search, being a transient
question asked while triaging rather than a statement about which work exists in
a step.

These are recorded as `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.6`, `002.4` and
`001.6` respectively, so each is independently testable. The `002` criteria are
in [Board priority sort and filter ordering](board-priority-sort-filter-order.md);
`001.6` is in this document.

## Requirements

### REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-001: A person can filter the board to selected priorities

**Intent:** Let someone triaging a full board narrow it to the work that matters
now, without losing the board's structure or changing anything about the tasks
themselves.

**User story:** As someone triaging a crowded board, I want to show only the
critical and high tasks, so that I can see what needs attention without reading
past everything else.

#### Acceptance criteria

- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.1:** The display surface shall offer
  a priority filter presenting exactly the four priority tokens, each shown by
  its localized priority label, each independently selectable.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.2:** When the priority filter
  selection is empty, the system shall display every task the board's other
  filters admit. The empty selection is the default and means "no priority
  filtering", never "show nothing".
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.3:** When the priority filter
  selection is non-empty, the system shall display a task only when its priority
  token is a member of the selection. An unranked task shall not be displayed
  under any non-empty selection, because it holds none of the selected tokens.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.4:** The priority filter shall
  compose with the board's existing workflow, repository, hidden-step, search and
  plugin filters, such that a task is displayed only when it satisfies all of
  them. Applying the priority filter shall not weaken, bypass or reorder any
  existing filter.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.5:** When a non-empty selection
  admits no task in a workflow step, the system shall render that step's column
  as empty rather than removing it, omitting it, or rendering an error.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.6:** The priority filter shall not
  contribute to the occupancy set. No priority filter selection shall cause a
  workflow step's column to be auto-hidden as empty. This matches the board's
  existing treatment of search text and differs deliberately from its treatment
  of the repository filter; see `## The ordering decision`.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.7:** When a task's priority changes
  from any source while a non-empty selection is active, the board shall add or
  remove that task from the display to match the stored value, without a page
  reload and without the person re-opening the display surface.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.8:** Selecting the same priority
  token that is already selected, or clearing a token that is already clear,
  shall leave the selection unchanged. The stored selection shall contain no
  duplicate tokens regardless of how many times a token is toggled.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.9:** When a person changes a task's
  priority from the board itself and the new token is outside a non-empty
  selection, the system shall remove that card from the display rather than
  retaining it until a refresh. The board follows the stored value even when the
  person's own action is what removed the card from view.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.10:** An unranked task shall be
  displayed under the empty selection, which is the default, and shall be excluded
  by every non-empty selection. This shall hold identically whether the task is
  unranked transiently, before the boot payload carries priority, or persistently,
  because its stored value lies outside the vocabulary; the display surface shall
  not distinguish the two. The control shall offer no option that selects unranked
  tasks, so clearing the priority filter is the only way to see them, and that is
  the decision recorded under `## Out of scope` rather than an oversight. Applying
  or clearing the filter shall never write a priority to an unranked task and shall
  never coerce one to `medium`, or to any other token, in order to match a
  selection.

Requirement 002, ordering each workflow step's cards by priority, is in
[Board priority sort and filter ordering](board-priority-sort-filter-order.md),
which also carries the `## Out of scope` exclusions governing all three files.
Requirements 003 to 005, covering breakpoint reachability, persistence and
convergence of the two stored values, and token-versus-label fidelity, are in
[Board priority sort and filter view state](board-priority-sort-filter-view-state.md).
The three documents are one contract and none is complete alone.
