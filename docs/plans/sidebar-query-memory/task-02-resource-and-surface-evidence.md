---
id: "02-resource-and-surface-evidence"
title: "Prove pooled resource bounds and surface compatibility"
status: in_progress
wave: 4
depends_on: ["04-reuse-and-progress"]
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.1
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.2
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.3
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.4
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.5
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.6
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.7
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.8
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.9
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.10
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.11
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.12
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.13
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.19
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.20
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.21
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.22
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.23
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.24
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
  - ../../specs/ui/system-design/sidebar-shared-task-state.md
---

# Task 02: Prove pooled resource bounds and surface compatibility

## Summary

Establish bounded native memory through repeated, concurrent reads on the real reader pool.
Prove the corrected query retains existing desktop and phone navigation and view results.

## In scope

- Add the pooled subprocess regression and extend the existing 100,000-task benchmark matrix.
- Exercise all supported sort/group pairs, both directions, full query cleanup, and fixed-fixture repeated reads.
- Extend existing desktop and phone activity/paging tests with State-grouped Last activity refreshes.
- Record final resource and conformance results in this package.

## Out of scope

General QA, a full E2E suite, new UI controls, browser heap-leak fixes, cache changes, and mutation of the live installation.

## Acceptance

1. `TestSidebarQueryPoolMemoryPlateau` meets the design's 256 MiB concurrent native peak, 512 MiB RSS delta, and 8 MiB retained-native delta budgets.
2. The 100,000-task benchmark covers first, middle, and final pages for Last activity with State and Repository grouping, preserving existing result bounds.
3. Desktop and phone refresh/paging preserve complete-tree order, the selected conversation, and touch/scroll behavior without eager page traversal.

## Resource evidence

Use the child-process measurement helper from Task 01, without parallel unrelated tests.
Initialize the schema and four-connection reader pool before the baseline.
Use fresh subprocesses per matrix case to prevent one case's allocator history from hiding another.
Warm the fixture once, then run 100 reads in batches of four against fixed 101-task data.
Sample RSS and native current/peak counters per batch, including after all rows and transactions close.
Alternate workspace IDs and view shapes in an additional run to detect stale scratch state.
Include empty pages, collapsed groups, and late pages so secondary statements execute.

Do not force GC or call allocator trimming to satisfy the budget.
Native counters are process-global: peak reset and sampling must not overlap unrelated database work.
Emit host, build, SQLite version, query mode, concurrency, count, peak bytes, retained bytes, RSS, and duration.
Do not emit filter contents, task titles, or user records.

Extend `BenchmarkSidebarTaskPage100K` to include State/Repository Last activity cases in both dialects.
Preserve existing first/middle/final page coverage and the existing warm timing criterion in the system design.
Report cold preparation separately from populated execution, Go allocation, native allocation, and RSS.
A schema-only pass does not prove acceptable populated execution.

## E2E scenario matrix

| Project | Existing file | Extension |
| --- | --- | --- |
| chromium | `sidebar-task-tree-activity-sort.spec.ts` | State grouping; repeated child activity refresh; full-tree rank and row-owned time |
| chromium | `sidebar-task-pagination.spec.ts` | More than 100 tasks; next/back/refresh; conversation identity unchanged |
| mobile-chrome | `mobile-sidebar-task-tree-activity-sort.spec.ts` | Same query through the task-picker drawer; touch access and correct order |
| mobile-chrome | `mobile-sidebar-task-pagination.spec.ts` | Bounded pages and retained conversation; viewport-contained picker scrolling |

Nearest phone exemplar: `SessionTaskSwitcherSheet` with `MobileTaskList`.
Retain its current drawer, single scroll owner, safe areas, focus return, and touch targets.
Use real isolated backend responses. Do not mock successful query data or test against the user's instance.
Preserve UI-01/UI-02 in the [plan](plan.md#ascii-ui-preview). Task 04 owns the changed loading/refresh states.

Add `sidebar-shared-task-state.spec.ts` and `mobile-sidebar-shared-task-state.spec.ts`.
Use UI-01/UI-02 with complete homepage hydration and assert immediate rows plus zero redundant sidebar queries.
Then select cold archives, assert one bounded read, page repeatedly, and inspect the test store's bounded entity ownership.
Use controlled deferred HTTP responses and real seeded changes for the three-invalidation case.
Prove first-response display, newer-field preservation, known-deletion removal, and bounded trailing requests.
Repeat context switches and reconnect recovery on both surfaces. Do not mock successful data to simulate coverage.

## Verification

Run from the repository root. Complete Tasks 01, 03, and 04 first.
Install dependencies once if this worktree has no `apps/node_modules`.
The PostgreSQL DSN must name a disposable test database; skipped cases remain incomplete.
Run the two E2E commands sequentially with the managed runner's worker limits.

```bash
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test -tags sqlite_fts5 -p 1 ./internal/task/repository/sqlite -run '^TestSidebarQueryPoolMemoryPlateau$' -count=1 -v)
(cd apps/backend && go test -tags sqlite_fts5 -p 1 ./internal/task/repository/sqlite -run '^TestSidebarTaskViewConformance$|^TestQuerySidebarTaskPagePostgres$' -count=1 -v)
(cd apps/backend && go test -tags sqlite_fts5 -p 1 ./internal/task/repository/sqlite -run '^$' -bench '^BenchmarkSidebarTaskPage100K$' -benchtime=1x -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-task-tree-activity-sort.spec.ts tests/task/sidebar-task-pagination.spec.ts tests/task/sidebar-shared-task-state.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-task-tree-activity-sort.spec.ts tests/task/mobile-sidebar-task-pagination.spec.ts tests/task/mobile-sidebar-shared-task-state.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record measurements and exact discovered/passed E2E counts before marking the work order done.
A browser-process crash that reproduces after this correction needs separate renderer evidence; do not mark it resolved by association.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_memory_test.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_memory_linux_test.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_test.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_memory_benchmark_test.go` (new, if required by file limits)
- `apps/web/e2e/tests/task/sidebar-task-tree-activity-sort.spec.ts`
- `apps/web/e2e/tests/task/sidebar-task-tree-activity-sort-helpers.ts`
- `apps/web/e2e/tests/task/sidebar-task-pagination.spec.ts`
- `apps/web/e2e/tests/task/sidebar-task-pagination-fixtures.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-tree-activity-sort.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-pagination.spec.ts`
- `apps/web/e2e/tests/task/sidebar-shared-task-state.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-sidebar-shared-task-state.spec.ts` (new)
- `docs/plans/sidebar-query-memory/plan.md`

## Dependencies

Task 04, transitively Tasks 03 and 01: complete shared-state/controller implementation and the native measurement helper.

## Risks

Timing varies with host load; report hardware and separate timing evidence from deterministic memory gates.
Temporary storage can increase I/O on large fixtures, so the benchmark cannot be replaced with a warm empty-query loop.
Browser tests prove view compatibility, not the cause or absence of a renderer memory leak.

## Parallelism

`sequential`

## Inputs

- [Pagination requirements](../../specs/ui/requirements/sidebar-task-pagination.md), .1–.13 and .19.
- [Browsing design](../../specs/ui/system-design/sidebar-archived-filter.md), Memory and compatibility budgets.
- Task 01 results and existing sidebar E2E fixtures.
- `apps/web/e2e/README.md` and the local E2E skill.

## Results

The CI-only resource job now runs fresh subprocesses for every sort/group/direction case, 100 reads per case at concurrency four, plus an alternating-workspace run. It also covers maximum legal filters, collapse lists, and pin/manual-order preferences. Populated benchmarks cover first, middle, and final pages for Last activity with None, State, and Repository grouping on SQLite and PostgreSQL. The populated fixture includes backlog/running/completed trees and single/combined/unassigned repository memberships. Stage timings separate candidate preparation from page execution, headers, and hydration. Native peak, retained allocation, RSS, elapsed time, cold execution, and Go allocation evidence remain pending CI.

Desktop and phone regression cases now cover State-grouped repeated child activity, complete-store local pagination without sidebar requests, bounded archive ownership, safe first-response progress during three live invalidations, and repeated workspace/reconnect recovery. They execute through the existing E2E CI projects; no local tests or seeded instance were run. Final discovered/passed counts and measurements must be recorded before completion.

The initial CI resource job (run `36652869598`, job `109692202303`, artifact `11072014218`) passed all 144 preparation cases, five maximum-input cases, and 73 pooled cases (72 shapes plus alternating workspaces). Native peaks were 18,930,240 bytes for preparation, 36,382,240 bytes for maximum inputs, and 74,777,464 bytes for pooled reads. The pooled retained-native maximum was 96,192 bytes and RSS delta maximum 156,868,608 bytes. These satisfy the resource budgets.

The same run exposed a remaining timing failure on Linux/amd64, AMD EPYC 7763, Go 1.26.0, SQLite 3.51.1, and GOMAXPROCS=4: 100,000-task warm queries took 10.76–15.74 seconds on SQLite and 4.37–12.71 seconds on PostgreSQL. A successful benchmark process does not meet the one-second target. Query-plan remediation now uses parent adjacency for page descendant counts, narrower intermediate rows, existing SQLite scratch indexes, and a planner-visible ephemeral predicate. Repeat CI measurements and browser counts remain required before completion.

At `b8c343d8b`, timing job `109815402966` in run `36692999006` passed the
two-engine semantic matrix. Populated 100K warm pages on AMD EPYC 7763 with
GOMAXPROCS=4 took 1.20–1.22s (None), 1.98–2.00s (State), and 1.50–1.55s
(Repository) on SQLite; PostgreSQL took 2.95–3.20s, 3.73–4.18s, and 3.05–3.25s.
These remain above the criterion. Execution plans exposed inflated recursive/page
cardinality and repeated ordering. Followups retain parent-relationship statistics,
bound planner-visible page rows, share display-root memberships for grouping/counts,
and add scratch statistics cleanup checks. Final exact-head evidence remains pending.

The user waived the one-second 100K timing gate on 2026-09-30. Keep the populated
benchmark informational and preserve its measured results; native memory, CI,
review disposition, and the zero-query covered-state contract remain acceptance
criteria. Run `36701226446`, timing job `109841594666`, measured `e5ea04469` on
AMD EPYC 9V74 with GOMAXPROCS=4: SQLite 0.909–1.502s and PostgreSQL
2.246–3.227s across all first/middle/final None/State/Repository pages.
