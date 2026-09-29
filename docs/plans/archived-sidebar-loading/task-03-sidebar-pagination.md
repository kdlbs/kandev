---
id: "03-sidebar-pagination"
title: "Connect desktop and phone pagination"
status: done
wave: 3
depends_on:
  - "02-bounded-query-api"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-001
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.5
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.7
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.8
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.9
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.13
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-001.14
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.1
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.2
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.3
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.4
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.5
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.6
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.7
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.8
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.9
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.11
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.12
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.13
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
---

# Task 03: Connect desktop and phone pagination

## Summary

Replace eager archive loading with a shared current-page query and page controls.
Active and archived views consume the same server order on desktop and phone.

## In scope

- Add the API client and typed page response. Replace the all-pages loader and its old test expectation.
- Keep one active page and one pending replacement, with store-owned request deduplication and cancellation.
- Update archive WS handlers to mutate visible rows or invalidate the page, never accumulate off-page tasks.
- Integrate ordered entries and continuation strips into desktop, task picker, and app-navigation surfaces.
- Add localized Previous/Next/status/continuation/error treatment in all locales.
- Reset page on effective query changes; preserve displayed page identity on failed replacement.
- Update archive browsing instructions in `docs/public/tasks-and-workflows.md` after behavior passes.

## Out of scope

Kanban-column pagination, global virtualizer work, and conversation-selection repair.

## Acceptance

1. Opening a 10,000-task archive makes one page request and constructs at most 100 task rows; Next replaces rather than appends.
2. Deferred reads, StrictMode, workspace/view changes, live events, and foreground refresh cannot leak or accumulate rows from old pages.
3. Desktop and phone E2E prove paging, global filters/tree order, retry, touch geometry, and both phone entry points.

## TDD evidence

Replace `loads every archived-only page and stops at the reported total` with
`loads only the requested sidebar page`. Assert request count while responses remain deferred.
Cover requested-page versus displayed-page state, 250ms coalescing with a two-second bound,
one trailing invalidation, metadata clamping, stale responses, and zero-result filters.
Use a fixture where only a task beyond source row 100 matches a title clause.
Use a parent whose newest included descendant crosses the first page boundary.
Assert no automatic page-two request after idle or foreground refresh.

## Risks

Existing consumers expect full `allTasks` and can accidentally re-filter a page.
Audit count, color, current-task, hierarchy, and WS consumers of `sidebarArchivedTasks`.
Keep active snapshots and detail data independent from the bounded sidebar page cache.

## ASCII UI preview

UI-01 and UI-02, excerpt from [the full preview](plan.md#ascii-ui-preview).

```text
Desktop sidebar                 Phone picker body
[View v] [Filters]           [View v] [Filters]
Group / Parent (continued)      Group / Parent (continued)
Task A              Running    Task A              Running
[Previous] Page 2 [Next]         [Previous] Page 2 [Next]
```

Desktop keeps its sidebar scroll owner. Phone keeps its drawer/menu scroll owner,
safe-area padding, and 44px touch controls. All entries fit the current page limit.
Initial pending reads show Loading; failed replacement reads retain rows plus Retry.
Empty successful results show the archive empty state with both page directions disabled.
Maps to requirement 002's criteria and requirement 001.9.

## Verification

Run from the repository root. Install workspace dependencies first in a fresh worktree.

```bash
(cd apps/web && pnpm exec vitest run components/task/task-switcher.test.tsx components/task/task-session-sidebar-grouped-view.test.ts components/task/task-select-helpers.test.ts components/task/task-select-helpers-archived.test.ts components/task/mobile/session-task-switcher-sheet-hooks.test.ts components/task/mobile/session-task-switcher-sheet-archived-selection.test.ts components/task/sidebar-task-pagination.test.tsx hooks/domains/kanban/use-sidebar-task-page.test.tsx hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts hooks/domains/session/use-ensure-task-session.test.ts hooks/domains/sidebar/use-sidebar-task-prefs.test.tsx hooks/use-sidebar-multi-select.test.ts hooks/use-task-removal.test.ts lib/api/domains/kanban-api.test.ts lib/sidebar/sidebar-view-conformance.test.ts lib/sidebar/apply-view.test.ts lib/ssr/session-page-state.test.ts lib/ws/handlers/tasks-archive.test.ts src/task-detail-route.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --project chromium -- e2e/tests/task/sidebar-task-pagination.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- e2e/tests/task/mobile-sidebar-task-pagination.spec.ts)
git diff --check
```

## Files likely touched

- `apps/web/lib/api/domains/kanban-api.ts`
- `apps/web/lib/api/domains/kanban-api.test.ts`
- `apps/web/hooks/domains/kanban/use-sidebar-archived-tasks.ts`
- `apps/web/hooks/domains/kanban/use-sidebar-archived-tasks.test.ts`
- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts`
- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/lib/state/slices/kanban/kanban-slice.ts`
- `apps/web/lib/ws/handlers/tasks.ts`
- `apps/web/lib/ws/handlers/tasks-archive.test.ts`
- `apps/web/lib/ws/handlers/tasks-unarchive.test.ts`
- `apps/web/components/task/sidebar-task-page.tsx (new)`
- `apps/web/components/task/sidebar-task-page.test.tsx (new)`
- `apps/web/components/task/task-session-sidebar.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`
- `apps/web/components/navigation/mobile-task-navigation-provider.tsx`
- `apps/web/src/locales/*/sidebar.json`
- `apps/web/e2e/tests/task/sidebar-task-pagination.spec.ts (new)`
- `apps/web/e2e/tests/task/mobile-sidebar-task-pagination.spec.ts (new)`
- `docs/public/tasks-and-workflows.md`

New files named in this work order are planned; existing parent directories are present.

## Dependencies

02-bounded-query-api.

## Parallelism

`sequential`

## Inputs

- [Shared size-based pagination](../../specs/ui/requirements/sidebar-task-pagination.md).

- [Requirements](../../specs/ui/requirements/sidebar-archived-filter.md).
- [System design](../../specs/ui/system-design/sidebar-archived-filter.md).
- [Plan and evidence](plan.md).

## Results

Desktop and phone sidebars use the same bounded query for built-in, saved, draft, active, and
archived views. Coverage includes 99/100/101 thresholds, filter shrinkage, active conversation
preservation, live archive/unarchive changes, phone task-picker and app-navigation entry points,
and desktop multi-selection plus bulk archive across pages. The shared fixture also checks that
full-view filtering can find a task beyond the first 100 source rows.

Passed: 263 focused web tests across 19 files; `pnpm run typecheck`; changed-file ESLint with
`--max-warnings 0`; `pnpm run i18n:check && pnpm run i18n:ratchet`; `pnpm run build:vite`;
desktop pagination E2E (2 tests); phone pagination E2E (2 tests); public-doc validation (47 pages,
62 validator tests); spec/catalog validation; and `git diff --check`. One run of the mobile
pagination E2E failed during concurrent local checks, then the isolated case and full two-test
mobile file both passed without competing checks.

## View-independent pagination contract

Use one query/controller for built-in, saved, and draft views, active or archived.
Count displayable task rows after filters/collapse, excluding headings and context.
For 0 through 100 rows, hide paging controls. At 101 rows, show two pages.
Do not download all tasks to discover the threshold. Return the count with page one.
Successful Next/Previous replaces rows and scrolls only the task list to its top.
Failure preserves rows, page number, scroll position, and the selected conversation.
Live refresh can cross the threshold without selecting another task.

## Active-view integration and regression matrix

Own the removal of sidebar-only full workflow snapshot loading, including workspace boot
and hydration dependencies. Board-owned snapshots remain separate and retain their contract.
Inspect `useWorkspaceSidebarTasks`, workspace-context consumers, row action eligibility,
selection, removal fallback, ordering, and hierarchy consumers before replacing their data source.
Do not pass one page to an API that assumes a complete workspace inventory.

Desktop and phone E2E cover 99/100/101 boundaries in default active, saved active, saved archived,
and draft filtered views. A workspace with thousands of tasks filtered to 20 has no paginator.
One hundred tasks plus group headings still have no paginator. Filter changes reset page one.
Live create/archive/unarchive/delete cross the threshold and clamp a vanished last page.
Assert active task ID, session ID, transcript, and transcript scroll stay unchanged during paging.

Preserve selected IDs across pages; assert the selected-count indicator and clear action.
Exercise bulk actions spanning two pages, visible sibling reorder preserving off-page order,
parent actions with an off-page child, and delete-current-task fallback beyond this page.
Keep pending prompts, permission badges, WIP queue positions, and active row actions accurate.
Fetch authoritative bounded action data when it is absent from the page.
Add these scenarios to the named desktop/mobile sidebar-task-pagination E2E files and run them
with the existing verification commands. Add every changed action unit suite to that command
before recording Results; the implementation owns the concrete affected-file inventory.
