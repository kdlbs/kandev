---
status: current
system: ui
created: 2026-10-18
requirements:
  - REQ-UI-PROVIDER-WATCHER-TYPES-001
---

# Provider Watcher Task Types System Design

## Purpose and boundaries

UI owns reusable watcher classification for saved task views. This design
extends the categories in [Threads saved views](threads-saved-views.md) and the
boolean projections in [Sidebar browsing](sidebar-archived-filter.md), including
[complete local evaluation](sidebar-shared-task-state.md). Neither neighboring
design currently defines provider-neutral watcher identifiers across both surfaces.

Task persistence and provider metadata writers remain under their current
owners. Classification reads metadata already delivered with task overviews;
it does not establish provenance or permission to act on a provider.

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| AC-UI-PROVIDER-WATCHER-TYPES-001.1, .2, .3 | Closed key sets; Consumer flow |
| AC-UI-PROVIDER-WATCHER-TYPES-001.4 | Value semantics and compatibility |
| AC-UI-PROVIDER-WATCHER-TYPES-001.5, .6 | Consumer flow; Persistence and failure behavior |
| AC-UI-PROVIDER-WATCHER-TYPES-001.7 | Responsive outcome; Verification boundaries |

All abbreviated acceptance references in this design belong to
`REQ-UI-PROVIDER-WATCHER-TYPES-001`.

## Closed key sets

Use two small fixed key lists, with existential OR semantics within each list.
Do not infer a category from arbitrary names, provider labels, URLs, issue
numbers, task origin, or linked pull-request summaries.

| Provider | Issue-watch key | Review-watch key | Existing writer in backend internal/orchestrator |
| --- | --- | --- | --- |
| GitHub | `issue_watch_id` | `review_watch_id` | `event_handlers_github.go` |
| GitLab | `gitlab_issue_watch_id` | `gitlab_review_watch_id` | `source_gitlab.go` |
| Jira | `jira_issue_watch_id` | None | `source_jira.go` |
| Linear | `linear_issue_watch_id` | None | `source_linear.go` |
| Sentry | `sentry_issue_watch_id` | None | `source_sentry.go` |
| Azure DevOps | `azure_devops_work_item_watch_id` | `azure_devops_pr_watch_id` | `source_azuredevops.go` |

Keep frontend lists private to the existing neutral `lib/metadata-utils.ts`
helpers. The local Sidebar projector consumes those helpers rather than adding
another list or predicate. Backend fixed lists belong beside the Sidebar SQL
projection in `internal/task/repository/sqlite`; do not import orchestrator
source implementations into repository code. Nine explicit keys do not need a
registry, code generation, configuration, or a new framework.

## Value semantics and compatibility

For each exact top-level key, accept only a JSON string whose length exceeds
zero and whose value is not exactly `undefined`. Missing/null/empty strings,
numbers (including zero), booleans, arrays, and objects do not match. The literal
string `null` and nonempty whitespace remain opaque nonempty identifiers;
there is no trimming or UUID validation. Both category booleans are independent.

An invalid first key must not hide a valid later key: evaluate each key's
predicate and OR the resulting booleans, rather than coalescing raw values.
Threads keeps its existing review-first `taskType` precedence when both flags
are true. Do not add provider-specific task-type enum values.

Current malformed-value behavior differs: SQL's `sidebarWatchField` excludes
the `undefined` sentinel but accepts non-string extracted values;
`metadata-utils.ts` accepts only nonempty strings but includes that sentinel;
the local Sidebar uses truthiness and excludes the sentinel. The correction
deliberately reconciles only recognized watcher values: non-string values stop
matching SQL/local Sidebar; literal `undefined` stops matching shared mappers.
Valid GitHub IDs and absent/null/empty values retain their outcomes. Record
these narrow compatibility changes in regression cases; do not preserve
consumer disagreement through a legacy mode or broaden other metadata policy.

## Consumer flow

1. SQL `sidebarWatchFields` in `sidebar_task_query_base.go` expands only the
   category requested by `sidebarBaseNeedsFor`. `sidebarWatchField` produces
   the same `is_issue_watch`/`is_pr_review` aliases from ORed key predicates.
   Reuse `dialect.JSONTypeIsString` and `dialect.JSONExtract` for SQLite and
   PostgreSQL; retain the existing string booleans and parameterized query.
   Apply watcher filters in the existing candidate phase before matching totals
   and page slicing. Do not alter the query stages or projection when neither
   watcher filter is requested.
2. `lib/metadata-utils.ts` keeps `isIssueWatchFromMetadata` and
   `isPRReviewFromMetadata` as the shared frontend classifiers. Extend the
   existing value helper and use the closed lists. Leave `issueFieldsFromMetadata`
   and provider link fields unchanged.
3. `lib/kanban/map-task.ts::toKanbanTask` already calls both helpers for HTTP
   and WebSocket-compatible task shapes. `lib/ssr/mapper.ts::snapshotToState`
   calls them for initial snapshots. These callers need regression coverage,
   not a second implementation of recognition.
4. `lib/sidebar/sidebar-local-projection.ts::sidebarBooleanDimensions` calls
   the same helpers over `TaskOverview.metadata`; remove its independent
   truthiness helper. `matchesLocalSidebarTask` and `localSidebarPage` retain
   the existing boolean operators, totals, and paging.
5. `lib/sidebar/apply-view.ts` continues to use mapped booleans. Threads'
   `lib/threads/thread-view-query.ts` continues to consume those booleans in
   `taskType` and `queryThreadView`. The active `app/threads/threads-page-client.tsx`
   consumes that query. Do not rewrite its filters or admission rules.

## Persistence and failure behavior

No schema, migration, API payload, saved-view rewrite, metadata write, or
provider call is needed. Existing stored tasks gain recognition when evaluated.
Unexpected frontend metadata shapes fail to confer watcher flags. Repository
writes already serialize valid JSON; corrupt serialized database JSON retains
the existing query-error behavior rather than gaining a new repair policy.
Read errors, auth, retention, invalidation, cache coverage, and task lifecycle
remain owned by the existing contracts. No new logging or metrics are needed.

## Responsive outcome

Desktop and phone share the corrected task projections and filter results.
Existing Sidebar navigation and Threads saved-view editor surfaces are reused.
No markup, geometry, navigation, scrolling, touch controls, or translated copy
changes. The mobile-parity skill's data-normalization exception applies:
targeted consumer tests cover the shared result; no new browser scenario or
rendered preview is required for this bounded correction.

## Verification boundaries

Independent permanent tests must call actual consumers: persisted
`CreateTask` then `QuerySidebarTaskPage`; HTTP/WS-shaped `toKanbanTask` then
`queryThreadView`; initial `snapshotToState`; local projection then
`localSidebarPage`; mapped booleans then `applyFilters`. Assert task IDs and
matching totals where available, including positive and negative filters.
Do not derive expected keys/types from production lists or merely reimplement
the predicate in a test.

Cover all nine keys with valid IDs, GitHub controls, ordinary/manual links,
unrelated suffix lookalikes, each unusable value, mixed unusable/usable IDs,
both categories, and no watcher-filter requests. PostgreSQL behavior tests may
reuse the established environment-gated repository fixture, remaining skipped
without a supplied DSN. Add focused SQLite/PostgreSQL generated-expression
checks through the actual projection function for key/type/alias and lazy
projection contracts. Local validation must unset the external DSN; no real
external database or live PostgreSQL coverage claim is authorized.

## Related implementation record

- [One sequential work order](../../../plans/provider-watcher-task-types/plan.md)
