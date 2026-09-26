---
status: draft
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
records the pagination tradeoff. The design remains draft until implementation review.

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

The response contains `query_key`, `page`, `page_size`, `total_entries`, `total_tasks`,
`has_previous`, `has_next`, `entries`, and optional `continuation`.
`entries` contains at most 100 task-row records plus the relevant group headers.
`total_tasks` counts all matching tasks. Add `total_visible_tasks` for task rows after collapse visibility.
`total_entries` is display metadata only and never drives pagination.
Paginator visibility is `total_visible_tasks > 100`; headings and continuation labels do not count.
Use this same endpoint for small and large lists, with no eager-fetch threshold probe.
Small views return their full task-row set in the first response and hide pagination controls.

Task entries contain identity, parent ID, depth, and the bounded fields needed by the
existing row renderer. Exclude descriptions, messages, plans, environments, and session lists.
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
The query response is authoritative for every sidebar view order; the browser must not re-sort it.

Cycle guards must bound recursive traversal by the candidate graph size.
Break malformed cycles deterministically at the smallest task ID and promote that node.
Do not mutate relationships. Preserve effective-state existential and universal rules:
any running member prevents a completed tree; completion requires every included member.

Database ranking can process the matching set internally. Go must not hydrate or sort
all task records. Use indexed workspace/archive membership and parent relationships.
Inspect query plans for both supported dialects. Add indexes through existing migrations
only when the measured plan needs them; no new persisted presentation aggregate is planned.

## Cache and live updates

Replace the archive accumulator and active sidebar aggregation with one shared sidebar page cache per mounted query owner.
The effective workspace/view hook owns the query key, current page, request generation,
loading state, error state, and current response. Desktop and phone consumers share it.
Retain only the active workspace's sidebar page and at most one in-flight replacement.
An individual active-task detail record is separate and does not expand this cache.

Changing filters, sort, group, locale, or collapse preferences resets to page 1.
Changing workspace clears the page immediately. Preserve saved preferences themselves.
Previous/Next replaces the page after success and scrolls its owner to the top.
While a page request runs, disable both paging controls and keep the last accepted page.
Keep a requested page separate from the displayed page so errors never mislabel old rows.

Deduplicate requests by store-owned query/page identity, including StrictMode mounts.
Abort superseded requests and reject late responses by workspace and request generation.
A view-key mismatch never commits. A server-normalized query key becomes the accepted key.

Archive, unarchive, delete, activity, state, and relevant repository/settings events
invalidate affected membership or ordering. Immediately remove deleted rows and rows known to leave the current filter.
Archive removes a row from an active view; unarchive removes it from an archived view.
Update visible scalar fields from accepted live revisions. Do not append off-page tasks.
Coalesce invalidations into one current-page request, with at most one trailing refresh.
Use a 250 ms coalescing interval and a two-second maximum wait during an event burst.
Cancel timers when the owner is disabled or changes workspace.

An invalidation that arrives during a read schedules one follow-up; it does not cause
an immediate unbounded retry loop. Foreground refresh reads only the current page.
After settlement, clamp a vanished last page once using server page metadata.
Page traversal is live browsing, not a historical snapshot. Mutations can move boundaries
between reads, but one accepted page contains no duplicate task IDs.

Update existing archive-cache WS handlers so none can accumulate the complete workspace.
Do not populate active Kanban snapshots with archived rows.
Stop requesting full workspace workflow snapshots solely to populate the sidebar.
Boards can retain their independent snapshot subscriptions while mounted.
Audit boot hydration and workspace-context loading too; moving only the final render behind
pagination does not bound an earlier sidebar-owned load.

## Active-task actions and independent detail

All views consume the same paged projection, including lists of at most 100 tasks.
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
Use one status announcement and stable button names. Keep Retry separate from page actions.
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
Require first and deep-page queries to finish within one second after warm-up on the same
reported four-vCPU fixture host. Record hardware and database backend with results.
Do not claim constant database query time; window output and browser work are bounded.

Use existing request tracing for route duration and structured query counts.
Do not log titles, filter values, transcripts, or high-cardinality metric labels.
No new runtime feature flag or general monitoring subsystem is required.

## Delivery

[Plan and work orders](../../../plans/archived-sidebar-loading/plan.md).
The original implemented package remains historical; its eager-loader and navigation
instructions are superseded by this package, not recorded as successful new validation.
