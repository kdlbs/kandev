---
status: current
system: tasks
created: 2026-10-14
requirements:
  - REQ-TASKS-WORKSPACE-SEARCH-001
owners:
  - kandev
---

# Workspace task search system design

## Purpose and boundaries

Tasks owns the workspace-list match set and its total. The correction is local
to `searchTasks` and `taskSearchSelectQuery` in
`apps/backend/internal/task/repository/sqlite/task.go`, which also serves
PostgreSQL. Existing public request and response shapes remain unchanged.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKSPACE-SEARCH-001` | Literal predicate, Count and page consistency, Consumer wiring, Verification |

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
correction introduces no operation, option, response field, or terminology.
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

## Related specifications

- [Requirements](../requirements/workspace-search.md)
- [Archive result design](command-panel-archived-task-results.md)
- [Board repository matching design](../../ui/system-design/board-repository-matching.md)

## Phone List query adapter

The draft [phone List design](../../ui/system-design/mobile-list-saved-views.md)
adds an optional request-only `query` string to `SidebarTaskViewQuery`. This
reuses `REQ-TASKS-WORKSPACE-SEARCH-001` for List results through another transport.
The saved sidebar `titleMatch` clause remains title-only; no persisted search
field or saved-view migration follows.

Apply the existing four-field literal predicate in the candidate query before
counting, complete-tree ranking, and page selection. Bind escaped values and
retain dialect-aware LIKE/ILIKE behavior. Use repository EXISTS or equivalent
deduplication so matching repositories cannot multiply task rows. Both the
count and selected page use the same candidate set. Preserve authorized
workspace scope and authenticated color preferences.

The query digest and frontend query/cache identity include the optional string.
Nonempty queries use bounded server evaluation. Existing sidebar callers omit
the field and retain their behavior. Query validation must use a documented
request bound consistent with workspace search and existing body limits.

Task 01 in the [mobile List package](../../../plans/mobile-list-layout/plan.md)
implemented the request-only adapter and added SQLite and PostgreSQL tests for
searchable fields, punctuation, repository duplicates, complete-chain ordering,
and matches after page one. SQLite tests and changed Go lint passed. PostgreSQL
execution was skipped because `KANDEV_TEST_POSTGRES_DSN` was not set. This
adapter does not change the earlier workspace-search correction's historical
test results.
