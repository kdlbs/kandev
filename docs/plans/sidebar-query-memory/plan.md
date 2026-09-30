---
created: 2026-09-29
status: in_progress
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
  - ../../specs/ui/system-design/sidebar-shared-task-state.md
legacy_specs: []
---

# Implementation Plan: Shared Sidebar State and Bounded Queries

## Overview

Reuse normalized Zustand task overviews from the homepage and fetch uncovered sidebar views on demand.
Bound native SQLite preparation for the server requests that remain necessary.
Implement query cleanup, shared records/coverage, and source selection in that order, then run the combined resource and browser checks.
All four work orders run sequentially in the primary session. Implementation is in progress.
The user requested this package in PR #4062 and explicitly requires CI-only test execution.
The seeded preview is cancelled; the running installation remains untouched.
Covered local views have no query refresh state or announcement. Server refresh
announcements remain screen-reader-only and never move the visible list.

UI owns this reusable saved-view query contract, including its backend evaluation.
Task lifecycle, workspace authorization, and activity publication keep their existing owners.
The user requested a fix package for backend memory, normalized homepage/sidebar reuse, lazy archive reads, and initial-load progress during live updates.
No product choice requires another interview: existing sort, grouping, paging, and saved preferences must remain valid.

## Evidence and conformance

The investigated build was `320f050e12071342522a7ba0730af03e6ce1b386`.
The live backend reached 17.5 GiB RSS with about 23 MB of live Go allocations.
CPU pprof attributed roughly 82% of sampled CPU to the sidebar handler and SQLite preparation.
Existing sidebar requests took 3–8 seconds and returned roughly 346 KB.
The saved view used `lastActivityAt` descending with `state` grouping.

An isolated process used the same SQLite dependency and a schema-only database, without user records.
Preparing that generated query peaked at 1,320,988,256 bytes inside SQLite and about 1.6 GB RSS.
Preparation took about 4.08 seconds. Statement close returned SQLite usage to about 3 MB but left RSS high.
Allocator trimming in that isolated process reduced RSS to about 12 MB.
No allocator calls were injected into the live backend.

The proposed temporary-relation prototype preserved the timestamp expression.
It executed candidate population, then prepared the recursive statement against stored columns.
Native peak allocation was 21,908,448 bytes, RSS about 36 MB, and elapsed time about 64 ms.
This is design evidence only: the prototype used no tasks and did not exercise the pooled repository lifecycle.
A separate disposable SQLite check showed temporary creation and rollback working with a read-only main database.

The failure exposes a missing resource acceptance criterion. Add AC-UI-SIDEBAR-ARCHIVED-FILTER-002.19 to the existing requirement.
Reuse .1–.14 for pagination and view compatibility. Add .20–.24 for shared records, coverage, lazy reads, progress, and retention.
Snapshot responses gain optional coverage metadata. Existing request shapes and saved preferences remain valid.
The [proposed ADR](../../decisions/2026-09-29-sidebar-query-scratch-relation.md) records the new scratch ownership rule.

Session-local evidence is under `.kandev/diagnostics/memory-20260929/` and `/tmp/kandev-memory-investigation/`.
These ignored artifacts are optional supporting evidence, not prerequisites for implementation or tests.
Rebuild permanent regressions from synthetic schema and existing fixtures; do not commit the diagnostic bundle or user data.

## Scope

### In scope

- SQLite candidate/filter staging within the existing snapshot and reader pool.
- Deterministic cleanup, connection discard on uncertain cleanup, and request isolation.
- Native allocation, pooled RSS, cold preparation, and populated execution evidence.
- Existing SQLite/PostgreSQL conformance and desktop/phone sidebar behavior.
- Canonical task overview records in Zustand, with separate homepage and sidebar membership.
- Explicit complete/current coverage and verified local ordering before local evaluation.
- Immediate rendering from eligible homepage data, including local paging of complete resident collections.
- Lazy bounded reads for archives and other incomplete views, plus reconciled initial display during soft invalidation.

### Out of scope

- Browser renderer crash remediation: no browser heap or renderer crash dump established its cause.
- IndexedDB clearing, Redux adoption, unbounded archive prefetch, or allocator tuning.
- Saved-view migration, replacing Last activity with Updated, or changing grouping defaults.
- Persistent schema changes, custom SQLite functions, global connection defaults, and new feature flags.
- Unverified PostgreSQL browser collation. Its records are shared, but unsupported ordering profiles use server evaluation.
- Restarting or changing the user's live installation.

## Technical approach

Follow [Bounded SQLite query preparation](../../specs/ui/system-design/sidebar-archived-filter.md#bounded-sqlite-query-preparation).
Split candidate/filter SQL from visibility SQL in `sidebar_task_query.go` and `sidebar_task_query_base.go`.
Use `CREATE TEMP TABLE ... AS` once per request, then use its scalar columns in `sidebar_task_query_page_sql.go`.
Keep the original timestamp expression, recursive semantics, and parameter binding.
Introduce a focused `sidebar_task_query_snapshot.go` for connection, transaction, and scratch cleanup ownership.
Close rows before cleanup. Never return a connection with uncertain scratch state to the pool.

| Boundary | Intended behavior | Evidence |
| --- | --- | --- |
| SQLite writer | Existing writes and WAL behavior | Reader/writer overlap regression |
| SQLite reader, `mode=ro` | Main database read-only; temporary schema owned by one request | Cleanup, cancellation, pool-reuse tests |
| PostgreSQL | Existing statement path and snapshot | Disposable-PostgreSQL conformance |
| Desktop sidebar | Same selected view, tree order, and bounded pages | Existing desktop flows plus repeated refresh |
| Phone picker | Same query through `SessionTaskSwitcherSheet` | Existing mobile flows plus repeated refresh |

### Shared overview and source ownership

Follow [shared task state](../../specs/ui/system-design/sidebar-shared-task-state.md).
Introduce canonical overview entities and ID-based workflow/page membership in the existing Zustand store.
Migrate homepage/sidebar consumers and relevant writes together; do not complete the work with two independently mutable task stores.
Keep rich task/session detail separate. Preserve board optimistic behavior and existing Office consumers.

Add optional `task_coverage` to workflow snapshots and boot projection.
Treat truncated, missing-workflow, failed, older-server, disconnected, or field-incomplete datasets as incomplete.
Do not guess coverage from record count. Active snapshot coverage never includes archives.
Use verified SQLite local ordering; unknown or unsupported profiles use the optimized server path.

A covered view renders locally without another sidebar query. The 100-row limit applies to displayed pages, not source selection.
An already complete larger resident collection can paginate locally. Never fetch the whole workspace to enable local rendering.
An uncovered archived saved view requests only its selected page and merges task records into the shared store.
Retain bounded query memberships and evict overview records after their last owner releases them.

### Initial-load progress

Distinguish hard identity/access barriers from ordinary same-view invalidation.
Do not cancel a running read for every task update. Merge its successful response with newer records and deletion tombstones.
Display remaining safe rows as refreshing, and schedule one trailing refresh.
Keep provisional memberships out of reusable cache storage. Preserve cross-workspace/account guards and known removals.
The first successful safe response must not require an event-free interval.

## ASCII UI preview

UI-01: Desktop Tasks sidebar, active saved view with complete homepage coverage.
UI-02: Phone task-picker drawer, same view and coverage. Geometry remains unchanged.

```text
Before: homepage has tasks        After UI-01: complete shared data
Tasks [View v] [Filters]          Tasks [View v] [Filters]
Loading tasks...                 Parent task
(wait for sidebar query)           Child task
                                 Other task
                                 [Previous] Page 1 of 2 [Next]
                                 (paging only above 100 rows)

UI-02: Phone picker               Uncovered archive, either surface
+---------------------------+    Tasks [Archived v] [Filters]
| Tasks [View v] [Filters]   |    Loading tasks...       (first read)
| Parent task               |    -> returned page rows
|   Child task              |    -> safe rows retained (soft refresh)
| Other task                |    rows remain visible during refresh
| [Previous] 1/2 [Next]      |
+---------------------------+
```

The list body retains its existing scroll owner. Phone retains safe areas, focus return, and at least 44px touch targets.
No new control is proposed. Labels are illustrative; reuse existing localized copy and translate any additions in all six catalogs.
The required change is data availability and refresh behavior, not spacing or visual redesign.
UI-01/UI-02 cover .6–.9 and .20–.24. The no-overflow and conversation-preservation checks remain required.

## Tests

| Acceptance | Required evidence |
| --- | --- |
| .19 | New `TestSidebarQueryPreparationMemory` and `TestSidebarQueryPoolMemoryPlateau` subprocess regressions |
| .19, .7 | New `TestSidebarQueryScratchLifecycle` and `TestSidebarQueryScratchWorkspaceIsolation` |
| .1–.5, .10, .14 | Existing `TestSidebarTaskViewConformance`, `TestQuerySidebarTaskPage*`, and large-membership fixtures |
| .3–.4 | Chronological-instant fixtures, deep descendants, malformed values, equal instants, filtered children |
| .6–.9, .11–.13 | Existing desktop/phone activity and pagination E2E, extended with repeated refresh |
| .20, .24 | Shared-entity identity, HTTP/WS freshness, board compatibility, and ownership/eviction tests |
| .21–.22 | Snapshot completeness metadata, truncation, missing workflows, old servers, local/server conformance |
| .23, .7, .16 | Three-invalidation initial read, deletion, context change, access denial, and bounded journal tests |
| .20–.24 | New desktop/phone shared-state E2E: no duplicate covered read, cold archive query, bounded browsing |

All acceptance references use the prefix `AC-UI-SIDEBAR-ARCHIVED-FILTER-002`.
Memory regressions must fail on the original code for native allocation, not timeout or missing setup.
Measure the production repository path. Do not pass tests by trimming the allocator or reducing supported query limits.

## E2E tests

Task 02 owns `sidebar-task-tree-activity-sort.spec.ts` and `sidebar-task-pagination.spec.ts` in project `chromium`.
It also owns their `mobile-` counterparts in project `mobile-chrome`.
Seed state-grouped Last activity views and repeat refresh after task changes.
Assert correct complete-tree order, retained conversation identity, bounded pages, and usable navigation.
Add `sidebar-shared-task-state.spec.ts` and `mobile-sidebar-shared-task-state.spec.ts` for UI-01/UI-02.
Open the homepage first, then the sidebar. Assert immediate rows and zero redundant sidebar queries while coverage remains valid.
Select an uncovered archive and assert one initial page read, no eager traversal, and bounded retention after paging.
Defer a cold response across three ordinary updates; safe rows must appear after its first successful settlement.
Use isolated fixture backends, never the user's running database.

## Work orders

- [ ] [Task 01: Bound SQLite preparation and scratch lifetime](task-01-bound-preparation.md)
- [ ] [Task 03: Normalize shared task overviews and coverage](task-03-shared-overviews.md)
- [ ] [Task 04: Reuse complete views and preserve initial-load progress](task-04-reuse-and-progress.md)
- [ ] [Task 02: Prove pooled resource bounds and surface compatibility](task-02-resource-and-surface-evidence.md)

Execution order is 01 → 03 → 04 → 02. Existing work-order identifiers remain stable.
No delegation is authorized.

## Companion packages

[Archived sidebar loading](../archived-sidebar-loading/plan.md) and
[sidebar view loading repair](../sidebar-view-loading-repair/plan.md) remain completed historical packages.
Their behavioral acceptance and test matrices still apply.
Their prior Go-allocation and warm-timing results do not establish the new native-memory budget.
Do not reopen or replace their historical results with planned evidence.
This package owns the correction, shared-state reuse, and expanded resource/browser matrix.
The shared-state ADR explicitly revises unconditional server evaluation and blanket soft-invalidation rejection.
Historical companion results remain unchanged; their earlier statements no longer define the proposed source-selection policy.

## Verification results

Design checkpoint: no production or permanent test changes were made during the design turn.
Documentation catalog validation passed: 332 decisions and 1249 specifications.
Full specification lint and its 36 tests passed.
The documentation-coverage preflight accepted all four work orders against the planned runtime change.
Relative package links and whitespace checks passed.
No product suites ran in this design turn. The isolated prototype is recorded separately above.
Kandev task-plan and completion tools were unavailable in this turn's tool catalog.
The repository files hold the handoff; no platform completion signal is claimed.
Exact product commands are in the four work orders.

Implementation is in progress in PR #4062. Complete current-workspace overviews
render and paginate through Zustand without sidebar queries or refresh announcements.
Cold, incomplete, and archived views retain bounded server reads. First successful
safe responses survive ordinary live invalidations; queued updates share one trailing
refresh. Workspace switching prevents the old task route from rehydrating its scope.

CI run `36692999006`, timing job `109815402966`, passed the SQLite/PostgreSQL
semantic matrix at `b8c343d8b`. On AMD EPYC 7763 with GOMAXPROCS=4, populated
100K warm pages took 1.20–2.00s on SQLite and 2.95–4.18s on PostgreSQL.
These measurements still miss the one-second criterion. Subsequent changes reuse
display-root forests, preserve parent statistics, and expose the validated page
bound to the PostgreSQL planner. Final timings and browser counts remain required.

On 2026-09-30 the user waived the one-second 100K timing gate because that dataset
is an unused edge case. Timing remains reported evidence. Required CI, review gates,
native memory budgets, zero-request covered sidebar behavior, and confirmed merge
still gate delivery. At `e5ea04469`, timing job `109841594666` in run `36701226446`
reported AMD EPYC 9V74 and GOMAXPROCS=4: SQLite warm pages took 0.909–0.926s
(None), 1.474–1.502s (State), and 1.098–1.106s (Repository); PostgreSQL took
2.246–2.299s, 3.138–3.227s, and 2.360–2.467s respectively.

## Risks

- Temporary relations introduce cleanup and pool-reuse hazards; failure-path tests are mandatory.
- Candidate storage can move cost to disk; populated benchmarks must measure that cost.
- Cancellation can race rollback; cleanup uses a separate bounded context and discards uncertain connections.
- Native SQLite counters are process-global; resource tests use isolated subprocesses without parallel unrelated work.
- Shared-host timing is noisy; native memory is the CI gate, while timings include host details.
- Local evaluation must not change collation, stable ties, tree semantics, or WIP metadata.
- Snapshot truncation, missing workflows, and reconnect gaps cannot be treated as complete coverage.
- Normalization must preserve optimistic board writes and freshness across all overview producers.
- Soft invalidation may produce temporarily stale counts; known deletions and hard context barriers still take precedence.
- The browser crash may remain after this repair; do not claim it is fixed without renderer evidence.

## Public documentation

Task 04 updates `docs/public/tasks-and-workflows.md` for shared-data reuse and on-demand archived views.
It updates `docs/public/websocket-api.md` for additive snapshot coverage and HTTP/WS reconciliation behavior.
No operator setting or saved-view migration is proposed. Product documentation ships with implementation, not as an implemented claim in this draft.
