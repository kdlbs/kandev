---
status: current
system: ui
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-001
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
---

# Sidebar Task Browsing System Design

## Purpose and boundaries

UI owns saved sidebar views and reusable task navigation. The task system owns
archive state, relationships, sessions, and activity publication. This design repairs
view loading and navigation without changing those task lifecycle contracts.

The [shared pagination requirement](../requirements/sidebar-task-pagination.md) governs every sidebar view.
The existing archive requirements remain authoritative for archive membership and navigation. This design replaces the eager
loading and URL-only navigation described in the original implementation plan.
[The proposed decision](../../../decisions/2026-09-26-bounded-archived-sidebar-queries.md)
records the pagination tradeoff. The implementation retains those query boundaries.
The [bounded view reuse decision](../../../decisions/2026-09-28-sidebar-view-page-reuse.md)
revises its single-page retention rule for responsive return switching.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-SIDEBAR-ARCHIVED-FILTER-001 | Existing-session navigation; Cache and live updates; Failure behavior |
| REQ-UI-SIDEBAR-ARCHIVED-FILTER-002 | Query contract; Global view evaluation; Cache and live updates; Responsive surfaces |

## Current defect evidence

`loadSidebarTaskdTasks` loops until the workspace total is exhausted.
`useSidebarTaskdTasks` publishes only after that loop finishes and repeats it on foreground refresh.
The original loader test explicitly expects every page to load.

`selectTaskWithLayout` and `selectTaskFromSheet` return early for archived tasks.
They call `setActiveTask`, which clears the selected session, then update the URL.
On a task route, `replaceTaskUrl` only calls `history.replaceState`.
It does not load route data or select the archived conversation.
The existing desktop regression explicitly expects session loading to be skipped.
These facts establish repair targets independently of the screenshot's unknown task ID.

The captured browser layout exceptions are separate evidence. This package does not
attribute the screenshot to them or change Dockview deserialization internals.

## Query contract

Add `POST /api/v1/workspaces/:id/sidebar/query` as a read-only query.
Register it beside workspace task reads in `internal/task/handlers/task_handlers.go`.
Use the same authenticated workspace authorization and missing-workspace behavior.
Leave `GET /workspaces/:id/tasks` and its archive flags unchanged.

The request contains:

- `filters`: existing typed sidebar clauses; every clause is ANDed.
- `sort`: existing sort key and direction.
- `group`: existing group key.
- `collapsed_group_keys` and `collapsed_task_ids`: effective display preferences.
- `page`: positive integer; `page_size`: 1 through 100, default 100.
- `locale`: a supported UI locale for localized group labels.

Read pin, root-order, and child-order preferences from the authenticated user's
existing settings. Include draft filters directly so unsaved views also work.
Normalize the request and return its `query_key`. The key includes effective preferences
and locale, but never grants authorization. Reject invalid dimensions, operators, types,
or sizes with 400; do not silently broaden a query. Reuse existing sidebar validation
limits and impose a 256 KiB request-body limit. No arbitrary SQL or regular expressions.

Validate scalar strings by decoded UTF-8 byte length (at most 256), not by the
encoded size of an entire membership array. For `in` and `not_in`, accept up to
1,000 elements and validate every element using the dimension's scalar type and
size rules. Preserve existing empty-array semantics. Keep the 20-clause cap and
body limit; never truncate a clause or split it into ANDed fragments. Tests must
exercise the maximum combined parameter count on SQLite and PostgreSQL.

Represent validation failures with a typed model error and an additive HTTP
envelope: `error` remains for existing clients; `error_code` is
`sidebar_query_invalid`; `details` carries an allow-listed `reason`, optional
zero-based `filter_index`, and numeric `limit` when applicable. Reasons cover
malformed query, invalid clause/type/operator, scalar length, list count, clause
count, page bounds, locale, grouping, sorting, and collapsed-entry count.
Do not include submitted values or SQL. The frontend uses `ApiError.body` and
`errorCode` to localize known reasons; unknown or older-server errors use a
localized generic query error. Do not render backend English verbatim.

The response contains `query_key`, `page`, `page_size`, `total_entries`, `total_tasks`,
`has_previous`, `has_next`, `entries`, and optional `continuation`.
`entries` contains at most 100 task-row records plus the relevant group headers.
`total_tasks` counts all matching tasks. Add `total_visible_tasks` for task rows after collapse visibility.
`total_entries` is display metadata only and never drives pagination.
Paginator visibility is `total_visible_tasks > 100`; headings and continuation labels do not count.
Use this endpoint for uncovered views, with no eager-fetch threshold probe.
Complete eligible shared data can serve a view locally under the [shared-state design](sidebar-shared-task-state.md).
Small views return their full task-row set in the first response and hide pagination controls.

Task entries contain identity, parent ID, depth, filtered descendant count, and the bounded
fields needed by the existing row renderer. The count keeps the expand control available when
collapse or page boundaries omit descendant rows. Exclude descriptions, messages, plans,
environments, and session lists.
Keep task status summaries and repository labels within the current row projection contract.
Group entries carry stable group identity, label data, and full matching counts.
The client localizes built-in group labels. Repository and workflow names remain data.

When a page starts inside a group or tree, `continuation` contains the group identity,
immediate parent ID/title, and depth. Render one context strip, not duplicate task rows.
Long titles truncate. Do not return an unbounded ancestor path.
A parent link navigates to that task; it does not alter the filter or pretend the parent
is a member of this page. An out-of-range page clamps to the last available page.
An empty result returns page 1 with both directions disabled.

## Global view evaluation

Add a focused repository query module beside `repository/sqlite/task.go`.
Use parameterized SQL and existing dialect helpers for SQLite and PostgreSQL.
Page metadata and row identities come from one consistent read transaction.
Batch-enrich only the selected task IDs. Never call the all-pages workspace loader.

The logical stages are:

1. Select authorized workspace tasks using the effective archive filter.
   Positive Archived clauses choose archived candidates; absent/Hide clauses choose active candidates.
   Preserve contradictory-clause behavior, ephemeral and automation-origin exclusions.
   There is no archive-specific pagination gate.
2. Join bounded summary fields and ordered repository metadata.
   Use `task_status_summaries.summary` for activity and projected status.
   Missing summaries use existing task fallbacks, without synchronous transcript backfill.
3. Apply all supported clauses before forming the included parent graph.
4. Resolve complete included tree activity and effective state. Ignore filtered descendants.
   Promote a child only when its parent is excluded from the complete filtered set.
5. Apply sort, grouping, pins, and manual child ordering in the existing precedence.
6. Flatten expanded task rows in display order, then apply LIMIT/OFFSET to task rows only.
   Attach their group headings and boundary context afterward; collapse is resolved before slicing.
7. Read the bounded row projection and continuation context for those entries.

Port the semantics of `apply-view.ts`, `task-tree-activity.ts`, and
`effective-task-tree-state.ts`, with shared conformance fixtures. Cover every filter:
`archived`, `state`, `workflow`, `workflowStep`, `executorType`, `repository`, `hasDiff`,
`hasPR`, `isPRReview`, `isIssueWatch`, and `titleMatch`.
Preserve primary-repository filter semantics, complete-combination grouping, missing-value
behavior, literal case-insensitive title matching, and all six operators.
A missing field is not SQL NULL-equality: match the existing clause truth table explicitly.

Support `state`, `updatedAt`, `lastActivityAt`, `createdAt`, `title`, and `custom` sorts,
and all existing groups. Use task-owned Last activity fallbacks, never summary freshness.
Compare valid activity instants at nanosecond precision; retain malformed-value fallback.
For equal sort values, retain the canonical input ordinal from the existing default
workspace order: task updated time descending, title ascending, ID ascending.
This extends stable input ordering across pages instead of using a page-local ordinal.

Use the existing repository `taskTitleOrder` policy for sidebar title order and text group
order. This gives a concrete database ordering rule without installing custom collations.
For built-in groups, retain their explicit rank. Order user-named groups by stored name.
This deliberately replaces browser-dependent `localeCompare` for sidebar text ordering;
case and accented-title ties can move after upgrade. Last activity ranking is unchanged.
Use the same ordering policy for small and large views, so crossing the threshold cannot change collation.
Other non-sidebar consumers of the client engine remain unchanged.
Conformance fixtures record this text-order compatibility difference explicitly for each
dialect; all filter, tree, pin, and numeric ordering expectations remain shared.
Server page responses remain authoritative for their membership and order. The browser must not re-sort a partial server page.
A complete covered dataset can use the verified local evaluator described in [shared task state](sidebar-shared-task-state.md).

Cycle guards must bound recursive traversal by the candidate graph size.
Break malformed cycles deterministically at the smallest task ID and promote that node.
Do not mutate relationships. Preserve effective-state existential and universal rules:
any running member prevents a completed tree; completion requires every included member.

Database ranking can process the matching set internally. Go must not hydrate or sort
all task records. Use indexed workspace/archive membership and parent relationships.
Inspect query plans for both supported dialects. Add indexes through existing migrations
only when the measured plan needs them; no new persisted presentation aggregate is planned.

## Bounded SQLite query preparation

This section defines the proposed memory repair for AC-UI-SIDEBAR-ARCHIVED-FILTER-002.19.
The [decision](../../../decisions/2026-09-29-sidebar-query-scratch-relation.md) records the temporary-storage boundary.
The [repair package](../../../plans/sidebar-query-memory/plan.md) contains measurements and delivery checks.

`sidebarActivitySortKey` expands strict timestamp normalization into a large SQL expression.
References through the recursive sidebar CTE graph amplify its preparation cost.
A `MATERIALIZED` hint on `candidate_raw` alone does not prevent this cost.
A bounded response and a small Go heap therefore do not establish bounded native memory.

For SQLite, separate candidate evaluation from recursive page evaluation with a transaction-local scratch relation.
Use one explicitly pinned reader connection for the entire operation.
Keep the main database opened through `OpenSQLiteReader` with `mode=ro`.
Do not borrow the writer connection or weaken its read-only main-database contract.

1. Begin the existing consistent read transaction on that connection.
2. Execute `CREATE TEMP TABLE kandev_sidebar_filtered AS` followed by candidate and filter evaluation.
   Populate only the columns already selected by `sidebarBaseCandidateFields`, plus group keys and labels.
   Evaluate the existing activity expression here without changing its precision or fallback rules.
3. Add temporary indexes on task ID and parent ID for the recursive joins. Stage nonempty pin/manual-order preferences in `temp.kandev_sidebar_preferences`, keyed by kind, parent ID, and task ID; parameterize the input as JSON and retain the first position for duplicate IDs. Indexed lookups keep statement size independent of saved-list length.
4. Build the downstream query from a narrow `filtered` CTE over `temp.kandev_sidebar_filtered`.
   Apply collapse, cycle handling, tree aggregation, ranking, counts, and page selection as before.
5. Use the same relation for empty-page counts and collapsed-group headers.
   Hydrate only the selected task IDs, within the same transaction.
6. Drop both owned scratch tables before a successful commit, then return the connection.

Split `sidebarTaskBaseSQL` into candidate/filter construction and visibility construction.
Keep stage arguments separate: workspace and filter arguments populate the relation.
Collapse, preference, and paging arguments belong to downstream statements.
Never split argument lists by counting question marks or parsing generated SQL.
Use the original single-statement path for PostgreSQL.
Its request shape, authorization, ordering, and transaction guarantees remain unchanged.

The scratch relation contains lightweight rows for all matching candidates inside SQLite.
It does not copy complete tasks into Go or expose all candidates to the browser.
No descriptions, transcripts, sessions, or plans enter this relation.
Its size depends on matching task count and selected scalar columns, not browsing history.
Retain the engine's existing temporary-storage policy; do not force `temp_store=MEMORY`.
Do not add a persistent table, schema migration, custom SQLite function, or saved-view migration.

### Scratch ownership and failure handling

Use a constant, qualified scratch-table name owned only by this repository operation.
One pinned connection executes at most one sidebar query at a time.
Separate reader connections have separate temporary schemas.
Do not use `CREATE ... IF NOT EXISTS` or reuse rows from an earlier request.

Close all result sets before cleanup and transaction completion.
On cancellation or any intermediate error, roll back before returning the connection.
SQLite rolls back scratch-table creation with the transaction.
Verify cleanup on the same connection with a short, independent cleanup context.
Drop any remaining owned scratch table after rollback; preserve the original request error.
If rollback or cleanup cannot establish a clean connection, discard it with `driver.ErrBadConn`
through `sql.Conn.Raw` instead of returning it to the pool.
Never expose cleanup SQL, submitted filters, or task values in the API error.
A failed stage returns the existing query error; there is no expensive legacy-query fallback.

The tests must cover failure after creation, indexing, page read, header read, hydration, and commit.
Inject failures through test seams, without new production configuration.
Also cover cancellation and exhausted cleanup deadlines.
The next borrower must see no scratch rows from another workspace or user.
A concurrent writer must remain usable under the existing WAL snapshot rules.

### Memory and compatibility budgets

Use Linux subprocess tests with the production SQLite library and real reader-pool wiring.
Measure native allocation high-water marks separately from Go allocations.
Reset SQLite's global high-water counter only inside a dedicated subprocess.
A small test-support cgo package can expose `sqlite3_memory_used` and `sqlite3_memory_highwater`.
It must have no production callers and must not call `malloc_trim`.

For empty and 101-task fixtures, each full sequential read has a native peak increase of at most 64 MiB.
Cover every supported sort/group combination, both directions, page boundaries, and collapsed headers.
Also cover maximum legal filters and preferences without weakening existing input limits.
Four concurrent reads have an aggregate native peak increase of at most 256 MiB.
After 100 fixed-fixture reads through the four-connection pool, RSS stays within 512 MiB of the initialized baseline.
Retained SQLite allocation must return within 8 MiB of the warmed baseline after each batch.
These fixture budgets detect preparation amplification; they do not impose a constant-memory claim on arbitrary workspace sizes.
RSS and allocator samples include explicit units and the process/build identity.

Record cold and warm timings on the same host; target less than 500 ms for the empty state/activity query.
Do not use a wall-clock unit-test threshold on a shared CI host.
Retain the existing 100,000-task first/deep-page benchmark and its reported-host timing criterion.
Extend its query matrix to Last activity with State and Repository grouping.
Record native peak memory and RSS alongside duration, response size, and Go allocations.
The correction must not silently trade the preparation problem for unbounded execution cost.

Reuse chronological-order fixtures for nanoseconds, timezone offsets, equal instants, malformed timestamps,
missing summaries, filtered descendants, cycles, pins, manual order, and WIP metadata.
PostgreSQL must pass the existing conformance tests using a disposable database.
Desktop and phone E2E retain the existing sidebar/picker composition and verify complete-tree order and paging.
No new controls, copy, or rendered geometry are proposed.

## Cache and live updates

The [shared task state design](sidebar-shared-task-state.md) now owns canonical task overviews,
coverage metadata, local view eligibility, bounded page memberships, and reconciliation during reads.
It revises the previous unconditional query-on-mount and revision-mismatch rejection rules.
Keep the five-page, 2 MiB, five-minute reusable-page limits, including attributable entity bytes.
Account/workspace changes and access denial remain hard invalidation boundaries.

A complete resident dataset can serve local pages without a duplicate request.
An uncovered view uses the optimized server query and merges returned records into the same overview store.
Ordinary changes can make a response require refresh without making it unsafe to show reconciled rows.
The client separates those cases from hard identity, authorization, and deletion barriers.

### Disclosure continuity

For AC-UI-SIDEBAR-ARCHIVED-FILTER-002.25, distinguish display continuity from
query-page reuse. `useSidebarPageContext` keeps the full `viewKey`, including
`collapsed_group_keys` and `collapsed_task_ids`, for requests, cache membership,
and response fencing. Derive a companion content identity from the same inputs,
excluding only those two disclosure fields. Include workspace and account scope,
context generation, filters, sort, grouping, locale, pins, and ordering preferences.
Do not weaken the server query or cache key to avoid a loading placeholder.

`useSidebarTaskPage` derives the accepted response's content identity from its
existing bounded displayed-page identity. Prefer complete local projection,
then a matching accepted or cached response. Only when the current content
identity matches the accepted displayed page may a disclosure change retain
that page as a transitional display. Do not retain an additional inventory,
accumulate pages, write the transition into the reusable cache, or classify it
as authoritative membership for the new query.

The existing `TaskSwitcher` group and subtask collapse preferences hide rows
immediately. On expansion, only previously available rows can appear until the
server supplies the bounded replacement. Keep eligible headings visible when
all groups collapse. Preserve known-removal reconciliation and hard context and
access invalidation; a deleted task must not reappear from the transition.
Mark transition state explicitly so intentional collapsed headings do not enter
the deletion-recovery empty-provisional loading path.

Request page 1 under the new full key, even when the source display was a later
page. Preserve the source page's identity instead of relabelling its rows as a
new authoritative page. Disable page navigation based on that transitional
snapshot until a current-query response establishes bounds. A transient failure
keeps eligible display content and uses the existing refresh-error and Retry
presentation; Retry must request page 1. Access denial clears content immediately.
Gate both accepted/transitional server display and complete local projection on
the authorized workspace during render. Collection or snapshot access-denied
state must hide rows before effect-driven request cleanup, even when workspace
generation and query identity remain unchanged.
Only an accepted current-key response clears transition state. Rapid disclosures
retain the last eligible accepted display; old completions cannot replace or
finalize the latest request. Ordinary same-context mutations retain existing
entity reconciliation and refresh scheduling rather than bypassing it.

Desktop sidebar, phone task picker, and phone app-navigation outlet consume the
same controller. Retain their existing scroll owners, group controls, touch
sizes, focus behavior, and screen-reader refresh status. Collapse does not move
the conversation, reset the list scroll explicitly, introduce a new overlay,
or create new user-facing copy. Existing normal initial-load, deletion-recovery,
view-switch, local-coverage, and exact-key cache-hit behavior stays separately tested.

## Active-task actions and independent detail

All views consume the same paged projection, derived from complete shared data or a bounded server response.
Keep task/session detail and active Kanban snapshots separate from this list cache.
Paging never calls setActiveTask, setActiveSession, or route navigation.
Only a successful user-initiated page change scrolls the list to the top.
Foreground refresh and failures preserve scroll position. Shrinking to at most 100 tasks
clamps to page 1 and removes controls without changing the open conversation.

Retain active row editing, task actions, selection, drag ordering, and nesting behavior.
Selection is ID-based and survives page navigation; select-all/range gestures apply to
the displayed page. Show the total selected count, including off-page selections, and
keep a visible clear-selection action. Mutations use selected IDs, not cached page rows.
Resolve action eligibility and hierarchy from authoritative reads when page data is insufficient.
Dragging reorders only visible siblings and merges them into the existing global order;
it must not replace off-page order or treat a missing parent as deleted.
Cross-page drag targets are not implied, but existing menu destinations remain discoverable
through bounded destination reads. Detail selection fallback must fetch eligible candidates
instead of interpreting the end of this page as the end of the workspace.

## Existing-session navigation

Replace archived URL-only shortcuts with actual SPA router navigation to `/t/:taskId`.
Reuse the task route rather than invoking active-task preparation logic.
This route navigation applies from task detail, listings, the phone picker, and app navigation.
It must not use `location.reload` or a full document navigation.

Key the archived route content by task identity. `TaskDetailRoute` must refuse data for
a different task during a route transition, including the render before its effect runs.
Route props and fallback session IDs must belong to the selected task.
The active route owns loading/error/empty state; a stale layout cannot supply its conversation.

Load the task and existing sessions, then select a remembered session only when that
session is present and belongs to this task. Otherwise choose the task's existing primary
session, then the first existing session. Hydrate the selected conversation through the
normal bounded message loader. No environment mapping is required to read archived chat.
Use a shared pure existing-session resolver so desktop and phone cannot diverge.
When selecting a non-primary existing session from a task list, include its `sessionId` in
the SPA route so route hydration preserves that selected conversation. Omit the parameter
for the primary session.

Pass known archive state into `useEnsureTaskSession`. Unknown archive state blocks ensure;
archived state always blocks it, including successful zero-session reads and retries.
Retain the existing archived resumption guard. Do not launch, prepare, or resume as fallback.
A truly sessionless archived task shows read-only empty content with Unarchive available.
A failed session or message read shows loading/error/retry, never a successful empty transcript.

Task-route requests have task identity and cancellation guards. Rapid A-to-B navigation
cannot apply A after B. Phone dismissal invalidates a pending picker selection before commit.
After successful selection, close the picker and focus the detail heading.

## Responsive surfaces

Desktop retains the existing Tasks sidebar and density. Add Previous, page status, and Next
below the current page, inside its existing scroll owner, only when more than 100 task rows are displayable. No new nested scroller.
Use `@kandev/ui` pagination/button primitives with 28px fine-pointer controls.

Phone retains `SessionTaskSwitcherSheet` and the shared app-navigation task outlet.
These are temporary task choices, so keep their existing drawer/menu composition.
Both consume the same query and page controls, with touch targets at least 44px.
The picker body owns scrolling; app navigation uses its existing menu scroller.
Keep dynamic viewport containment, safe-area clearance, focus return, and keyboard dismissal.
Phone row taps navigate directly to the single conversation surface.

Loading, errors, continuation, and page status use localized copy in all six languages.
Use one shared task-query status presenter per visible task-list surface. Remove
the duplicate archive banner for this query from desktop and phone paths; actual
workspace-context failures remain separately typed and take presentation priority.
Initial query failure replaces the list's loading state; refresh failure appears
once alongside retained rows. Place status immediately below the view controls,
with pagination below the rows. An invalid filter names its one-based position,
translated dimension where available, and correction; the existing Filters control
remains available. Retry is for reads that can succeed unchanged. A screen-reader-only
`Updating tasks...` status uses a polite announcement without moving or removing rows.
Covered local views issue no request and have no refreshing announcement. Hide status after success; distinguish empty success.
Keep stable button names and Retry separate from page actions.
A standalone current-task marker outside the page must not affect page totals or sort order.

## Verification and observability

Conformance fixtures compare complete expected view order with concatenated pages.
Use task IDs and group identities, not translated display strings, as test identities.
Active and archived 10,000-task fixtures must yield at most 100 task rows per response and one initial query.
Test 0, 1, 99, 100, 101, 200, and 201 tasks in built-in, saved, and draft views.
Group headings never trigger pagination at 100 tasks. Filters and collapse changes can
cross the threshold; hidden descendants still affect tree rank.
Prove paging never changes the active task/session or scrolls the conversation pane.
A 100,000-task benchmark records SQL plans, query duration, response bytes, and allocations.
Record first and deep-page warm timings on the same reported four-vCPU fixture host,
including hardware and database backend. The 100,000-task timing measurement is
informational; it does not gate delivery. Native memory budgets and bounded output
remain deterministic acceptance criteria.
Do not claim constant database query time; window output and browser work are bounded.

Use existing request tracing for route duration and structured query counts.
Do not log titles, filter values, transcripts, or high-cardinality metric labels.
No new runtime feature flag or general monitoring subsystem is required.

## Delivery

[Plan and work orders](../../../plans/archived-sidebar-loading/plan.md).
The [view loading repair](../../../plans/sidebar-view-loading-repair/plan.md)
owns filter validation, bounded return switching, and unified query status.
The original implemented package remains historical; its eager-loader and navigation
instructions are superseded by this package, not recorded as successful new validation.

The [memory repair](../../../plans/sidebar-query-memory/plan.md) adds native-allocation evidence and SQLite scratch-relation ownership.

[Shared task reuse](sidebar-shared-task-state.md) extends this repair to homepage reuse and initial-load progress under live invalidation.

The [disclosure continuity repair](../../../plans/sidebar-collapse-continuity/plan.md) extends same-view display continuity without changing query-page identity.
