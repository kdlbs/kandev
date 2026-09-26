---
created: 2026-09-26
status: done
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-001
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
legacy_specs: []
---

# Implementation Plan: Sidebar Loading and Archived Navigation

## Overview

Bound every sidebar task view and restore conversation loading on archived-task selection.
The four sequential work orders are complete, including the query, API, desktop/phone paging,
and existing-session navigation paths.

UI owns this package because saved-view evaluation and task navigation are UI contracts.
The tasks system continues to own archive state, relationships, activity, and session lifecycle.

## Evidence and requirement conformance

The loader loops through every 100-task page and stores the whole archive.
Its test, `loads every archived-only page and stops at the reported total`, encodes that behavior.
This is a missing boundedness requirement, covered by the new requirement 002.

Archived selection clears `activeSessionId` and changes history without SPA navigation.
`task-select-helpers.test.ts` asserts that the archived path skips session loading.
The phone selector has the same early return. This violates requirement 001.6's detail-opening
outcome; criteria 001.10 through .12 make conversation, lifecycle, and race outcomes explicit.
The smallest reproduction is an archived row selected from another open task, with distinct
saved conversations. Assert the new task's message, without refreshing the browser.

No isolated browser reproduction was completed during diagnosis. The route and selection
regressions now have unit and browser coverage. The original screenshot remains diagnostic
context only. Browser Dockview exceptions remain uncorrelated.

## Scope

In scope: bounded sidebar pages, global saved-view semantics, current-page refresh,
read-only existing-session navigation, localized desktop/phone states, and regression evidence.

Out of scope: active-board pagination, task retention, task deletion semantics, new archive
row actions, general Dockview deserialization repairs, and database maintenance tools.

## Technical approach

Use the [system design](../../specs/ui/system-design/sidebar-archived-filter.md) and
[proposed ADR](../../decisions/2026-09-26-bounded-archived-sidebar-queries.md).

The existing task-list GET endpoint cannot represent the complete sidebar view.
Add a dedicated read-only query with relational filtering, complete tree ranking, and bounded
row hydration. Do not port the eager archive loop into a backend service.
SQLite and PostgreSQL use existing dialect conventions. Global view output precedes paging.

All sidebar views consume ordered page entries
without re-filtering or regrouping a partial dataset. Keep one active page and one pending
replacement. Current-task detail remains independent of page membership.

Use real SPA navigation for archived row selection. Validate route/task/session identity,
load an existing session, and block all agent preparation while archive state is unknown
or archived. This is a navigation repair, not an unarchive operation.

## Related delivery records

- [Original archived sidebar package](../sidebar-archived-filter/plan.md): implemented history;
  this package supersedes eager accumulation and URL-only archived selection.
- [Last activity](../sidebar-last-activity-sort/plan.md): preserve task activity publication.
- [Tree activity](../sidebar-task-tree-activity-sort/plan.md): preserve complete included-tree ranking.
- [Workspace views](../workspace-sidebar-task-views/plan.md): preserve user/workspace settings ownership.

No old work-order status or verification result is changed into evidence for this repair.

## ASCII UI preview

UI-01: Desktop task view. Entry: Tasks > any built-in, saved, or draft view.

```text
BEFORE                         AFTER
Tasks [View v]             Tasks [View v] [Filters]
All archived rows             Group / Parent (continued)
...                           Task A          Running   2m
                              Task B          Waiting   4m
                              [Previous] Page 2 [Next]
```

UI-02: Phone task picker. Entry: task-title picker or app navigation > Tasks.

```text
+--------------------------------+
| Tasks                  [Close] |
| [View v]         [Filters] |
|--------------------------------|
| Group / Parent (continued)     |
| Task A              Running  |
| Task B              Waiting  |
|                                |
| [Previous]  Page 2  [Next]      |
+--------------------------------+
```

The existing phone picker header stays fixed. Its body owns vertical scrolling.
In app navigation, the existing menu scroller owns this same content.
Page controls remain in the list flow, with safe-area padding and 44px touch targets.
Desktop controls keep 28px fine-pointer height. Spacing and example titles are illustrative.
Page boundaries, context, action order, and scroll ownership are required.
These views map to AC-UI-SIDEBAR-ARCHIVED-FILTER-002.1 through .9.

UI-03: Archived task selection, shared outcome in desktop chat and phone detail.

```text
Click/tap task -> Loading conversation... -> Existing messages
                                         -> Could not load. [Retry]
                                         -> No saved conversation.
                                            Archived, read-only.
```

Only a successful zero-session or zero-message read permits the empty state.
Desktop keeps its task workbench; phone closes the picker and opens one detail surface.
The existing Unarchive action remains available. No agent starts.
This maps to AC-UI-SIDEBAR-ARCHIVED-FILTER-001.6 and .9 through .13.

## Tests and acceptance mapping

| Criteria | Regression evidence |
| --- | --- |
| 001.2-.5, .8, .14; 002.3-.5 | Task 01 conformance fixtures: every clause, sort, group, missing fields, cross-page descendants, pins and manual child order |
| 001.8-.9; 002.1-.2, .6-.8 | Task 02 repository/handler tests: authorized bounded pages, totals, clamping, invalid inputs, stable reads and query plans |
| 001.5, .7-.9, .13-.14; 002.1-.9 | Task 03 hook/store/component tests and desktop/phone sidebar-page E2E |
| 001.6, .9-.13 | Task 04 route/selection/ensure tests and desktop/phone archived-conversation E2E |

Full acceptance IDs appear in each work order. Cross-page fixtures place a matching task
and an active descendant beyond the first 100 source rows. Concatenated stable pages must
match complete expected order, not merely each page's local sort.

## E2E tests

- `e2e/tests/task/sidebar-task-pagination.spec.ts`, chromium: active and archived views,
  99/100/101 boundaries, default/saved/draft filters, live archive/unarchive changes, preserved
  conversation, and cross-page multi-select plus bulk archive. Query and hook tests cover
  complete tree ordering, replacement failures, and stale responses.
- `e2e/tests/task/mobile-sidebar-task-pagination.spec.ts`, mobile-chrome: threshold and filter
  transitions through the task picker and app-navigation outlet, list-only scroll reset,
  44px controls, and no document navigation or horizontal overflow.
- `e2e/tests/task/archived-task-navigation.spec.ts`, chromium: active-to-archived and
  archived-to-archived navigation, distinct saved messages, cold cache, multiple sessions,
  no agent launch, delayed reads, and rapid selection.
- `e2e/tests/task/mobile-archived-task-navigation.spec.ts`, mobile-chrome: matching tap flow,
  picker close, sessionless and retry states, no document overflow.

Use real seeded task/session/message data on isolated test backends. Intercept transport
only to control delay/error timing. Do not use the user's database.

## Work orders

Execute sequentially. No delegation is authorized.

- [x] [Task 01: Define complete sidebar view queries](task-01-view-query-semantics.md)
- [x] [Task 02: Expose bounded sidebar pages](task-02-bounded-query-api.md)
- [x] [Task 03: Connect desktop and phone pagination](task-03-sidebar-pagination.md)
- [x] [Task 04: Restore archived conversation navigation](task-04-conversation-navigation.md)

## Verification results

Implementation and product tests are complete.
Design-package validation passed on 2026-09-26:

- `python3 scripts/list-docs.py validate`: 310 decisions and 1185 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/list-docs.py decisions --text bounded-archived --format paths`: ADR discovered.
- `git diff --check -- docs/plans/archived-sidebar-loading docs/specs docs/decisions`: passed.
- Work-order AC references, design links, existing input paths, and pending dependencies resolved.

Implementation verification passed:

- Backend sidebar query, handler, and service tests passed against SQLite and PostgreSQL.
- `TMPDIR=/root/.cache/kandev-sidebar-pagination-go-tmp make -C apps/backend build`: passed.
- The 100,000-task sidebar benchmark passed in the prior session; warm queries measured about
  0.85-0.95 seconds and responses were about 60 KB.
- Focused web tests passed: 263 tests across 19 files. Web typecheck, changed-file ESLint,
  i18n checks and ratchet, and the Vite build passed.
- Desktop sidebar pagination E2E passed (2 tests); phone pagination E2E passed (2 tests).
  Archived conversation navigation E2E passed on desktop and phone (1 test each).
- Documentation catalog and spec lint passed (310 decisions, 1185 specifications). Public-doc
  validation passed for 47 pages, with all 62 validator tests passing. `git diff --check` passed.
- Follow-up query tests passed on SQLite and PostgreSQL. Both engines covered chronological
  timestamp ordering and global group order with pinned rows; SQLite also covered the separate
  reader-pool snapshot and filtered-graph collapse across archive filters. Nine focused web tests
  passed (six hook, one component, two view-conformance), along with web typecheck, changed-file
  ESLint, the backend build, and the Vite build.
- An optional unfiltered `go test -p 1 ./internal/task/...` run was stopped after more than
  eight minutes on the existing `TestTaskReviewFinding_ConcurrentTransitionsKeepResolvedTimestamp`;
  this is not reported as a passing suite. The scoped sidebar Go tests and backend build passed.

The task-specific work-order results below list exact coverage and commands. E2E checks used
isolated test backends. No live-instance data was changed, and no changes were staged or committed.

## Risks

- Global tree and locale semantics make the query more involved than a task-list LIMIT.
  Shared fixtures passed against SQLite and PostgreSQL before the consumer switched.
- Sidebar text ordering uses the existing database title policy. Case and accented-title
  ties can differ from the old browser locale order; Last activity semantics remain intact.
- Deep OFFSET reads and recursive aggregation can remain expensive. The 100,000-task
  benchmark passed; future query or indexing changes must preserve that evidence.
- Pages can move during concurrent task changes. Current-page invalidation gives live
  convergence; snapshot-stable export is excluded.
- Page controls change continuous archive scrolling. They keep client work bounded even
  after long browsing sessions. Current saved-view settings are retained.
- A route-level remount must not affect active-task navigation or launch a session.
- The separate Dockview errors can persist. Record any remaining reproduction separately.

## Public documentation

Updated `docs/public/tasks-and-workflows.md` with sidebar paging and read-only archived
conversation access, and `docs/public/websocket-api.md` with the additive bounded query route.
Public-doc validation passed for 47 pages.

## View-independent pagination contract

Use one query/controller for built-in, saved, and draft views, active or archived.
Count displayable task rows after filters/collapse, excluding headings and context.
For 0 through 100 rows, hide paging controls. At 101 rows, show two pages.
Do not download all tasks to discover the threshold. Return the count with page one.
Successful Next/Previous replaces rows and scrolls only the task list to its top.
Failure preserves rows, page number, scroll position, and the selected conversation.
Live refresh can cross the threshold without selecting another task.

## Size-based UX for every view

The user explicitly expanded pagination to all sidebar views. Archive state is only a filter.
The package path retains its original name, but all query and paging work orders now cover
active and archived lists. The navigation repair remains specific to archived conversations.

```text
100 matching displayable tasks       101 matching displayable tasks
Tasks [View v] [Filters]              Tasks [View v] [Filters]
Task rows                            First 100 task rows
(no pagination controls)             [Previous disabled] Page 1 of 2 [Next]
```

The same rule applies in the phone picker/menu with its existing scroll owner and touch sizes.
Next keeps current rows while loading, then replaces them and scrolls the list to the top.
Failures retain page and scroll position with Retry. Paging never changes the open task.
Headings do not count toward 100. Hidden descendants contribute to tree rank, not page size.

The shared requirement is [Sidebar task pagination](../../specs/ui/requirements/sidebar-task-pagination.md).
Its original stable ID is retained after the scope expansion.
