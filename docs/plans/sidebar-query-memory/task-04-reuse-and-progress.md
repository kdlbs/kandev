---
id: "04-reuse-and-progress"
title: "Reuse complete views and preserve initial-load progress"
status: in_progress
wave: 3
depends_on: ["03-shared-overviews"]
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
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.16
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.20
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.21
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.22
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.23
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.24
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
  - ../../specs/ui/system-design/sidebar-shared-task-state.md
---

# Task 04: Reuse complete views and preserve initial-load progress

## Summary

Render covered views immediately from shared Zustand records and fetch uncovered views lazily.
Prevent ordinary live invalidation from repeatedly discarding the first safe response.

## In scope

- Add local view evaluation against verified SQLite semantics and the complete-coverage contract.
- Share one page projection/controller between desktop, phone, and app-navigation task lists.
- Select complete resident data before retained pages or a server request; never refetch covered tasks solely on sidebar mount.
- Paginate already complete larger resident collections locally, without a new all-task download.
- Fetch cold archived and incomplete views by requested page and merge entities into the shared store.
- Separate hard stale-response barriers from soft invalidation, with safe provisional rows and one trailing refresh.
- Preserve bounded memberships, count/refresh status, conversation identity, and existing page scroll behavior.
- Update public task/API documentation for implemented source selection and optional coverage metadata.

## Out of scope

Redux, unbounded archive prefetch, new visual controls, removing hard context guards, persistent browser storage, and renderer-crash claims.
PostgreSQL local ordering remains disabled for unverified collation profiles; its server pages still reuse canonical entities.

## Acceptance

1. Complete current-workspace coverage produces the correct first page synchronously and starts no redundant sidebar query.
2. Uncovered archives fetch one requested page, preserve global view semantics, and release old unowned records after navigation.
3. Three soft invalidations during a deferred first read cannot keep safe rows blank after success; hard barriers and known removals still win.

## TDD evidence

Add `sidebar-local-view.test.ts` and `sidebar-task-source.test.ts`.
Run shared conformance fixtures against local output and actual SQLite server pages.
Cover all filters, supported sorts/groups, Unicode/ASCII collation, canonical stable ties, nanosecond activity, filtered/deep/cyclic trees,
repository combinations, WIP metadata, pins/manual order, collapse, and 0/100/101-task boundaries.
An unsupported profile or missing required input must select the server path, never silently change ordering.

Extend `use-workspace-sidebar-tasks.test.ts`, `use-sidebar-task-page.test.tsx`, and `sidebar-task-page-cache.test.ts`.
Cover complete homepage data with a deferred sidebar endpoint and assert zero requests, not merely eventual rows.
Cover a complete resident set above 100 tasks and assert local paging without network traversal.
Cover stale/truncated/missing-workflow coverage, cold task-route entry, and positive workflow restriction eligibility.
Cover archived saved views after active homepage hydration: active coverage cannot satisfy the archive.

Hold the first server response while three ordinary task updates arrive, then resolve it.
Assert safe rows appear, newer fields remain, deleted/ineligible IDs stay absent, and at most one trailing read starts.
Assert provisional membership cannot become reusable authoritative cache data.
Repeat with workspace/account change, access denial, superseded view/page, reconnect gap, and journal overflow.
Those cases must reject unsafe data rather than show it for responsiveness.
When every returned row was removed, retain eligible prior rows or show recovery, never an authoritative empty result.

## ASCII UI preview

UI-01/UI-02 use the [combined preview](plan.md#ascii-ui-preview).

```text
Desktop sidebar                 Phone task-picker drawer
Tasks [View v] [Filters]         +--------------------------+
Parent task                     | Tasks [View v] [Filters]  |
  Child task                    | Parent task              |
Other task                      |   Child task             |
                                | Other task               |
[Previous] 1/2 [Next]            | [Previous] 1/2 [Next]     |
                                +--------------------------+
```

Covered views show rows immediately. Uncovered cold views use existing loading; refreshing keeps safe rows visible.
Pagination appears only above 100 displayable tasks, regardless of data source or archive state.
The existing list/picker body remains the scroll owner. Phone retains safe areas, focus return, and 44px touch targets.
These structural rules cover .6–.9 and .20–.24; spacing and labels are illustrative.
Reuse existing localization keys; any new copy requires all six locales and the i18n checks below.

## Verification

Run from repository root. Task 03 must be complete.

```bash
(cd apps/web && pnpm exec vitest run lib/sidebar/sidebar-local-view.test.ts lib/sidebar/sidebar-task-source.test.ts lib/sidebar/sidebar-view-conformance.test.ts lib/sidebar/sidebar-task-page-cache.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts hooks/domains/kanban/use-sidebar-task-page.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Task 02 owns the combined desktop/phone E2E runs after this work order.
If implementation changes localization, also run `pnpm run i18n:ratchet` from `apps/web` and record its result.

## Files likely touched

- `apps/web/lib/sidebar/sidebar-local-view.ts` and `.test.ts` (new)
- `apps/web/lib/sidebar/sidebar-task-source.ts` and `.test.ts` (new)
- `apps/web/lib/sidebar/apply-view.ts` and existing tree utilities
- `apps/web/lib/sidebar/sidebar-view-conformance.test.ts`
- `apps/web/lib/sidebar/sidebar-task-page-cache.ts` and `.test.ts`
- `apps/web/hooks/domains/kanban/use-sidebar-task-page.ts` and `.test.tsx`
- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts` and `.test.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts`
- Existing sidebar query status presenter, only if the current localized refresh state cannot represent provisional membership.
- `docs/public/tasks-and-workflows.md`
- `docs/public/websocket-api.md`

## Dependencies

Task 03. It supplies canonical records, coverage metadata, and bounded reconciliation ownership.

## Risks

The previous browser evaluator and server differ in collation and stable input order; do not restore the old path without parity tests.
Soft invalidation must not weaken account/workspace or deletion safeguards.
In-progress response reconciliation cannot reconstruct unseen rows or exact fresh totals from a partial page.

## Parallelism

`sequential`

## Inputs

- [Shared-state design](../../specs/ui/system-design/sidebar-shared-task-state.md), source selection, ordering, initial display, retention.
- [Requirements](../../specs/ui/requirements/sidebar-task-pagination.md), referenced acceptance criteria.
- [Plan UI-01/UI-02](plan.md#ascii-ui-preview).
- Existing SQLite and browser view-conformance fixtures.

## Results

Complete eligible active scopes now render and paginate from canonical Zustand records. This includes resident collections above 100 tasks. The covered path has no sidebar request, loading state, or updating announcement. Uncovered archive/incomplete/PostgreSQL scopes keep bounded server paging and share accepted canonical records.

The local evaluator and actual SQLite queries consume `repository/testdata/sidebar-local-conformance.json`: all 72 sort/group/direction pairs plus filter operators/dimensions, Unicode and ASCII ordering, nanosecond activity, stable ties, tree promotion/cycles/collapse, continuations, repository combinations, and WIP ordering. Existing local boundary/deep-tree tests cover 0/100/101 tasks and a 2,000-node chain.

The page controller accepts safe first responses despite soft invalidations, merges live journal changes, excludes known deletions/ineligible archives, and schedules one trailing refresh. Provisional responses cannot enter reusable cache. Workspace/account/access/view/page/reconnect/overflow guards remain hard barriers. Existing translated server-refresh announcements remain limited to server-backed views and do not move the list.

Desktop, phone, and app-navigation lists use the same controller. CI regression cases cover zero-request complete views, local pagination, bounded cold archive ownership, first-response progress after three live invalidations, repeated workspace changes, and reconnect recovery. No tests ran locally. Exact CI conformance and surface results remain pending.

CI followups debounce queued invalidations after a slow response and cancel redundant
scheduled reads when that trailing read starts. Workspace-list failures retain manual
recovery; a denied task-page read clears its data and offers no unchanged-scope retry.
Changing workspace while a task route remains open cannot restart that old route and
restore its previous workspace. The 101-task fixture uses each surface's native
search control before opening its virtualized anchor. It preserves complete shared
coverage across client navigation and still asserts zero sidebar queries.

Old-head CI found 101 display memberships after the desktop reached archived page
two: the task command host independently retained page one. Commands and the
CSS-hidden phone desktop sidebar now read the active canonical record without a
list page. The controller regression covers an archived active record and active-task
changes; desktop/phone ownership assertions remain unchanged. The current-main
Changes timeline fixture now supplies a ResizeObserver entry when asserting a new
measurement, matching the deferred initial geometry contract. CI verification is pending.

Before the final fixture correction, frontend CI confirmed 2,428 test files passed
(20,887 tests passed, four skipped), and full backend CI was green. Browser CI
then exposed a stale causal order in the cold-archive phone recovery test: it awaited
the sidebar request before opening the drawer. The hidden desktop/command consumers
now correctly own no server page, so the request begins when the phone drawer mounts.
The fixture now arms its response wait before tapping the opener and awaits it after
the drawer is visible. The 503 alert, empty-state exclusion, 44px Retry target, and
successful archived-row recovery assertions remain intact. Final exact-head E2E
counts and merge confirmation remain pending in the external delivery record.
