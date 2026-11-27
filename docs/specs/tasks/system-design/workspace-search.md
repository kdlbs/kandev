---
status: current
system: tasks
created: 2026-10-14
requirements:
  - REQ-TASKS-WORKSPACE-SEARCH-001
  - REQ-TASKS-WORKSPACE-SEARCH-002
owners:
  - kandev
---

# Workspace task search system design

## Purpose and boundaries

Tasks owns the workspace-list match set and its total. Literal matching is
local to `searchTasks` and `taskSearchSelectQuery` in
`apps/backend/internal/task/repository/sqlite/task.go`, which also serves
PostgreSQL. Idle command-panel eligibility uses an additive opt-in request
parameter; the response shape and existing requests stay unchanged.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKSPACE-SEARCH-001` | Literal predicate, Count and page consistency, Consumer wiring, Verification |
| `REQ-TASKS-WORKSPACE-SEARCH-002` | Idle command-panel eligibility, Verification |

## Idle command-panel eligibility

The idle hook currently requests twenty workspace rows and filters loaded step
IDs afterward, so hidden rows can exhaust the page. For a settled selected
workflow, add an opt-in `command_panel_visible=true` to the existing workspace
list request, together with the selected `workflow_id`. It is an idle-only
contract: reject the option without a workflow ID or with a nonempty query.
The existing default list and all search requests remain unchanged. The hook
obtains the workflow ID from the same Kanban state that owns its loaded steps;
when steps are not loaded it keeps the existing ordinary-list fallback. If the
workflow identity is temporarily unavailable, preserve the existing local
fallback until the identity is available, rather than inventing an identity.

The authorized service passes the opt-in to a narrowly scoped repository
capability, following the optional `workspaceArchiveModeLister` pattern rather
than changing the positional `TaskRepository.ListTasksByWorkspace` interface
and its unrelated mocks. An unavailable capability must fail the opt-in
request, never silently return an unfiltered page. Reuse the existing list
filter, hydration, and DTO path. For the opt-in, the repository adds a bound
`EXISTS` predicate against `workflow_steps` for the task's step and selected
workflow with `show_in_command_panel` enabled. Apply it to both COUNT and
SELECT before LIMIT/OFFSET, and retain the existing default ordering and all
other filters; do not create a second listing implementation. Missing steps do
not qualify in this loaded-workflow path. No schema or index migration is
required: step identity is keyed. Both SQLite and PostgreSQL use the existing
portable integer representation. `queryAllTasks` currently uses an unaliased
COUNT and an aliased SELECT plus `rewriteFilterForAlias`; build the correlated
predicate with the correct outer alias in each query, rather than allowing
string rewriting to bind an inner step column to the wrong table.

Request page one of twenty eligible rows once for either scope, sort that
window by the existing client step-position map, then slice to five or twenty.
Retain the existing abort controller, response guard, and active-only defensive
check. Search uses neither the new option nor this eligibility predicate; it
keeps its active then archived fallback and conversation merge.

The existing sidebar query has a step filter, but its tree ordering and
configuration-task exclusions differ from workspace-list semantics; it is not
interchangeable. Increasing over-fetch or walking all pages would not bound
work and does not guarantee a correct first eligible page.

This changes shared result data inside an existing surface, not composition,
navigation, touch handling, scrolling, or breakpoint behavior. The mobile-parity
state/data exception applies: an independently authored rendered component test
and repository/handler tests cover the shared path. The existing phone entry
is navigation menu > Search tasks, with the same task result action; see
`mobile-command-palette-scopes.spec.ts`. Documentation assessment: the additive
`GET /api/v1/workspaces/:id/tasks` option is internal UI transport; that list
route is not documented as a supported public API in `docs/public`, the root
README, or the screenshot catalog. Other documented REST routes have separate
contracts. No public API reference needs a new parameter entry; the existing
`docs/public/workflow-tips.md` describes user-facing step visibility.

## Literal predicate

`ListTasksByWorkspaceWithArchiveMode` keeps its empty-query branch. Only
nonempty queries enter `searchTasks`. Build one bound substring pattern by
escaping original backslashes, percent signs, and underscores with a local
`strings.NewReplacer`, then adding the two intentional outer percent signs.
Do not trim, lowercase, normalize paths, or repeatedly escape replacements.

Use the existing `SearchMessages` implementation in `message.go` as the local
precedent. Every comparison for `t.title`, `t.description`, `r.name`, and
`r.local_path` must specify the same explicit `ESCAPE '\'` clause in both the
COUNT and SELECT SQL. Keep `dialect.Like` selecting SQLite `LIKE` or PostgreSQL
`ILIKE`, and keep values bound through the existing placeholders and `Rebind`.
The escape declaration belongs to each comparison, not just the last OR arm.

This local correction introduces no shared global search abstraction. Existing
SQLite case behavior and PostgreSQL collation behavior are retained; it does
not establish a new cross-dialect Unicode folding contract.

## Count and page consistency

COUNT and SELECT receive the same pattern and existing filter arguments.
Preserve `COUNT(DISTINCT t.id)`, the SELECT DISTINCT subquery, and ordering in
the outer SELECT through `taskListOrderBy`, followed by LIMIT/OFFSET. Multiple
matching linked repositories must not duplicate a task or inflate its total.

Keep alias rewriting, workspace scoping, workflow and repository restrictions,
archive-only precedence, ephemeral flags, configuration exclusion, and
automation-origin exclusion. Repository filtering continues to constrain the
existing joined repository rows; this change does not reinterpret the filter
as an independent membership predicate. Tasks without repository links can
still match title or description through the existing LEFT JOIN.

## Consumer wiring

`GET /api/v1/workspaces/:id/tasks` reads `query` and paging/filter arguments in
`task_http_handlers.go`. `Service.ListTasksByWorkspaceWithArchiveMode` authorizes
the workspace and passes those arguments to the repository, then retains its
existing PR-number augmentation and repository hydration.

`apps/web/lib/api/domains/kanban-api.ts` uses `URLSearchParams` to transport
query text. The relevant consumers are:

- `fetchMatchingTasks` in `use-command-panel-task-results.ts`: the Tasks scope
  requests page one with twenty rows, first active and then archived fallback.
  The hook trims surrounding whitespace before this call; the backend adds no
  further normalization.
- `loadTaskMentionItems` in `task-mention-items.ts`: page one with fifty rows.
- `listAllDependencyCandidates` in `use-task-edit-dialog-dependencies.ts`:
  hundred-row pages until the returned total has been collected.

These requests already carry punctuation as text. Correcting the backend
predicate supplies literal results to all three without changing their code.

## Responsive behavior and public documentation

Desktop and phone use the same workspace-list data contract. Apply the
mobile-parity state/data exception: repository behavior tests and the audited
request wiring cover this correction without new frontend or Playwright
changes. No layout, navigation, touch, or viewport-dependent behavior changes.

Public documentation assessment: no public operator change. Existing search
and command-panel instructions do not advertise SQL wildcard syntax, and the
literal-search correction introduces no operation, option, response field, or
terminology.
The literal text contract is recorded here and in its paired requirement.

## Persistence and failure behavior

There are no writes, migrations, transaction changes, or new metrics. Existing
query errors and cancellation propagate through the current repository/service
path. Workspace authorization and parameter binding retain their existing
boundaries. This is a functional matching correction, not a security fix.

## Verification

Independently authored tests call the real repository against disposable
SQLite databases. Cover literal punctuation across all four arms, escaped
adjacent characters, counts and multiple pages, a displaced first-page match,
repository deduplication, and existing filter/ordinary/empty-query controls.
Use fixed distinct ordering values rather than sleeps.

Add an environment-gated real PostgreSQL counterpart using
`testutil.PostgresDSNFromEnv` and `OpenIsolatedPostgres`, or the existing
`openPostgresRepo` helper. Run the same assertions against `ILIKE`; SQL-string
inspection or schema replay alone does not prove this behavior. Register
database cleanup before any subsequent fatal path. An unset DSN is a reported
skip, not evidence of a PostgreSQL pass.

For the idle eligibility correction, test mixed hidden and visible steps with
at least twenty newer hidden rows, totals and page boundaries, all-hidden,
default-list controls, workspace isolation, and archived, ephemeral,
configuration and automation-origin controls. Exercise the actual HTTP/service
opt-in and authorization, including invalid combinations. Repeat the relevant
repository behavior against an environment-gated real PostgreSQL database.
Independently author a rendered hook/command-list regression with a valid
paginated API fake, real command primitives and English catalogs: verify the
older eligible task is selectable after settlement, plus an ordinary first-page
control. Cover five/twenty limits, empty-step fallback and unchanged search,
archive and abort paths in focused hook tests. Do not copy external private
qualification fixtures.

## Related specifications

- [Requirements](../requirements/workspace-search.md)
- [Archive result design](command-panel-archived-task-results.md)
- [Board repository matching design](../../ui/system-design/board-repository-matching.md)
