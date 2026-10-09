---
id: "02-view-page-reuse"
title: "Reuse bounded sidebar first pages"
status: done
wave: 2
depends_on:
  - "01-membership-validation"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.2
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.6
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.7
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.8
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.12
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.15
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.16
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.18
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
---

# Task 02: Reuse bounded sidebar first pages

## Summary

Display eligible recently visited first pages immediately and revalidate in the
background. Keep server ordering and existing current-page replacement semantics.

## In scope

Store-scoped controller and full query identity; five first pages, 2 MiB serialized
budget, five-minute fetch-age TTL, LRU eviction. Shared consumers deduplicate
requests. Clear on context/authorization changes and invalidate on query revision.
Integrate display-only updates so snapshots cannot restore stale fields.
Record query cost separately from cache-hit behavior using the existing benchmark.

## Out of scope

Persistence, prefetch, keeping later pages, local re-filtering, speculative SQL
rewrites, and the status component owned by Task 03.

## Acceptance

1. Red first: hook test `shows cached first page before background refresh resolves`
   holds the response during A-B-A and asserts A's rows immediately. Cover shared
   desktop/phone consumers, StrictMode, error retention, and page-two-to-view reset.
2. Cache tests prove limits, TTL without access extension, eviction, complete key
   identity, same-ID changed drafts, generations, revision changes mid-read,
   permission denial, unmount/remount, store separation, and late aborted writes.
   No previously removed or foreign-workspace row can reappear.
3. Existing paging and live-event tests pass. Record SQLite/PostgreSQL benchmark
   durations and plans, retaining the existing warm one-second fixture target on
   the documented four-vCPU host. Do not infer live-instance latency from fixtures;
   investigate a measured regression before changing query/index code.

## ASCII UI preview

UI-01 / UI-03, cache-hit transition; full compositions in [plan](plan.md#ascii-ui-preview).

```text
Desktop sidebar                  Phone's existing Tasks drawer
[View A v] [Filters]             [View A v] [Filters]
Updating tasks...                Updating tasks...
Task A1                          Task A1
Task A2                          Task A2
```

Rows appear before the held response; no View B flash. Task 03 adds the status
label. Share data behavior across viewports; retain each surface's scroll owner.
Maps to 002.15-.16 and .18. No layout or touch-target change in this work order.

## Verification

Install workspace dependencies once from `apps/` if absent. From repository root:

```bash
(cd apps/web && pnpm exec vitest run lib/sidebar/sidebar-task-page-cache.test.ts hooks/domains/kanban/use-sidebar-task-page.test.tsx hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts lib/ws/handlers/tasks-archive.test.ts lib/ws/handlers/tasks-unarchive.test.ts lib/ws/handlers/tasks.deleted.test.ts lib/ws/handlers/tasks-status-summary.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/sidebar/sidebar-task-page-cache.ts hooks/domains/kanban/use-sidebar-task-page.ts)
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite -run '^$' -bench BenchmarkSidebarTaskPage100K -benchtime=3x -count=1 -benchmem)
git diff --check
```

Add any changed event-handler files to targeted ESLint. Task 03 owns the held-response
desktop/phone rendered proof; this work order does not claim browser completion.

## Files likely touched

- `apps/web/lib/sidebar/sidebar-task-page-cache.ts` and `.test.ts` (new)
- `apps/web/hooks/domains/kanban/use-sidebar-task-page.ts` and `.test.tsx`
- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts` and `.test.ts`
- `apps/web/lib/ws/handlers/task-archive-cache.ts`
- `apps/web/lib/ws/handlers/task-status-summary.ts`
- Existing archive, unarchive, deletion, and status-summary tests listed above
- `apps/web/AGENTS.md` if the shared cache ownership needs a concise local invariant

## Dependencies

Task 01; preserve its error shape for access/error classification.

## Risks

High event churn reduces hits. A controller must not outlive its store or expose
cross-account results. Current-page preservation is allowed only in its own context.

## Parallelism

`sequential`

## Inputs

- [Design: Cache and live updates](../../specs/ui/system-design/sidebar-archived-filter.md#cache-and-live-updates).
- [Reuse ADR](../../decisions/2026-09-28-sidebar-view-page-reuse.md).
- Existing hook tests and task-query revision/update paths.

## Results

Implemented store-scoped first-page reuse, shared pending requests, fetch-age TTL,
LRU/byte limits, and workspace/account/revision/summary fencing. A-B-A regression
failed before implementation and passes with a held refresh. Added contract
coverage for initiating-consumer unmount, late workspace responses, access denial,
expiry, aggregate bytes, logout, and display-only summary invalidation.

The targeted 11-file frontend/event run passed 91 tests, followed by the added
summary invalidation case (9 cache tests total). Existing phone consumer tests
passed (3 files, 8 tests). Typecheck and zero-warning targeted ESLint passed;
Task 03 records the final shared frontend checks.

Cold-query measurements on this host (Intel i7-11700T, Go GOMAXPROCS 2), using the
listed benchmark with `-p 2` and disposable PostgreSQL 17: SQLite first/middle/final
5.009/6.636/6.510 s per query; PostgreSQL first page 119.509 s. Responses were
60,354–60,854 bytes. Remaining PostgreSQL cases were stopped after the first-page
measurement because each query took about two minutes; the full benchmark
command is therefore incomplete. These are cost observations, not a passing
latency budget. Query SQL is unchanged. Cached-return behavior is proved separately
with held responses, without claiming an improvement to cold-query latency.

PR review follow-up: reproduced and fixed stale display commits after query revision
changes and access-denied rows retained by idle sibling consumers. Shared denial
notifications now reset all mounted consumers and fence their pending responses.
The original page-navigation invalidation test now requires the last accepted
page to remain displayed until the fresh requested page arrives, then scroll once
after its committed render. A separate
regression proves transient errors still expose Retry after a corrected filter;
the reported missing-Retry scenario did not reproduce on the original head.

Additional review follow-up: reproduced render-time cache lookup aborting a live
request when observing a workspace generation that is subsequently discarded.
Cache reads now validate scope, revision, summary references, and age without
mutating retained pages or pending requests; request acquisition and settlement
own synchronization. The regression first failed on the aborted signal and now
passes, including preservation of the original page and pending response.
Focused cache, hook, query-status, and pagination suites passed 32 tests across
four files; targeted ESLint and frontend typecheck passed.
