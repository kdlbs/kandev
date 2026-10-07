---
id: "01-sort-preset"
title: "Deliver configurable sort chains and the shared editor"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-RUNNING-ACTIVITY-001
  - REQ-UI-SIDEBAR-GROUP-INDENT-001
acceptance_criteria:
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.1
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.2
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.3
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.4
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.5
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.6
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.8
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.9
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.11
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.12
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.13
  - AC-UI-SIDEBAR-GROUP-INDENT-001.1
  - AC-UI-SIDEBAR-GROUP-INDENT-001.2
  - AC-UI-SIDEBAR-GROUP-INDENT-001.3
  - AC-UI-SIDEBAR-GROUP-INDENT-001.4
  - AC-UI-SIDEBAR-GROUP-INDENT-001.5
  - AC-UI-SIDEBAR-GROUP-INDENT-001.6
system_design:
  - ../../specs/ui/system-design/sidebar-running-first-activity-sort.md
---

# Task 01: Deliver configurable sort chains and the shared editor

## Summary

Implement the ordered chain contract, runtime/activity evaluation, persistence,
and desktop/phone editor. The work order keeps its original ID and filename for
continuation, but its scope replaces the fixed-preset implementation.

## In scope

- Reconcile partial `runningFirstActivity` backend edits. Reuse valid projections
  and provide a legacy input adapter; make Running a separate criterion.
- Add primary color field and flat `then_by` criteria to backend query/user models,
  frontend state/API types, conversions, normalization, and settings round trips.
- Add per-criterion SQL/local/generic comparisons and all-rule projection analysis.
  Complete this slice for existing ordinary fields and Running.
- Implement ordered rule editor, add/remove/reorder, directions, localized copy,
  standalone Custom mode, and activity row time for any activity criterion.
- Do not expose Color in the selector until Task 02 supplies complete evaluation.
  Ordinary chains must remain usable and independently verifiable.
- Test limits, duplicate keys, malformed criteria, old single sorts, legacy preset,
  source coverage, trees, overrides, and page/global conformance using TDD.

- Add per-view `groupIndent`/`group_indent`, default true including legacy views.
  Preserve explicit false in drafts, duplication, wire conversion, and settings.
- Render the labelled toggle below GroupPicker only in expanded Group by.
  Pass the preference to GroupSection and gate its group-body inset. Keep child
  depth, group controls, sorting, page selection, and query identity unchanged.

## Out of scope

Effective-color ranking and color UI exposure belong to Task 02. Browser proof
and public docs belong to Task 03. No lifecycle or schema changes.

## Acceptance

- Existing single-sort views retain behavior, and configurable runtime/activity
  chains work in SQL, covered local data, and generic complete-data consumers.
- Chain order/directions survive saved views and drafts; invalid queries report
  a safe rule index and incomplete coverage falls back to server evaluation.
- Desktop and phone edit/reorder rules with localized accessible controls;
  secondary activity activates row-local activity time. Color stays unavailable
  until its global evaluation exists. Group indentation defaults to enabled,
  with expanded-only control and persisted false; subtasks retain their nesting.

## ASCII UI preview

UI-01/UI-02 basic chain excerpt, before Color is enabled by Task 02:

```text
Desktop: 1 [Running v] [Running first v] [Up] [Down] [Remove]
         2 [Last activity v] [Newest first v] [Up] [Down] [Remove]
         [+ Add sort]

Phone card:
  1 [Running v]
    [Running first v]
    [Move up] [Move down] [Remove]
```

See [combined previews](plan.md#ascii-ui-preview), AC .1, .8, .11, and .13.
Phone cards live inside the existing drawer with one scroll owner.

UI-03 excerpt from [the plan](plan.md#ui-03-group-by-disclosure-desktop-and-phone):

```text
Collapsed: GROUP BY >  State
Expanded:  GROUP BY v  [State v]
           [x] Indent grouped tasks
```

The checkbox is absent from collapsed content. Applies to group-indent AC .1-.6.

## Verification

Run from repository root. Install workspace dependencies once if absent.
Set `KANDEV_TEST_POSTGRES_DSN` for the isolated PostgreSQL cases in the same Go command.
Proposed new test files below are implementation deliverables.

```bash
(cd apps/backend && go test -trimpath ./internal/task/models ./internal/task/handlers ./internal/task/repository/sqlite ./internal/user/models ./internal/user/dto ./internal/user/service ./internal/user/store -run 'Sidebar|QuerySidebar|WorkspaceSidebar' -count=1)
(cd apps/web && pnpm exec vitest run lib/sidebar/sidebar-sort-chain.test.ts lib/sidebar/task-tree-running.test.ts lib/sidebar/apply-view.test.ts lib/sidebar/task-tree-activity.test.ts lib/sidebar/sidebar-task-source.test.ts lib/sidebar/sidebar-local-view.test.ts lib/sidebar/sidebar-local-conformance.test.ts lib/sidebar/sidebar-view-conformance.test.ts lib/state/slices/task-overview-coverage.test.ts lib/state/slices/ui/ui-slice-migration.test.ts lib/state/slices/ui/sidebar-view-wire.test.ts lib/state/slices/ui/sidebar-workspace-views.test.ts components/task/sidebar-filter/sort-picker.test.tsx components/task/sidebar-filter/sidebar-filter-popover.test.tsx components/task/sidebar-filter/task-row-settings.test.tsx components/task/task-switcher.test.tsx components/task/task-switcher-group.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/sidebar/sidebar-sort-chain.ts lib/sidebar/task-tree-running.ts lib/sidebar/sidebar-local-order.ts lib/sidebar/sidebar-local-view.ts lib/sidebar/apply-view.ts lib/sidebar/sidebar-task-source.ts lib/state/slices/task-overview-coverage.ts lib/state/slices/ui/sidebar-view-types.ts lib/state/slices/ui/ui-slice.ts lib/state/slices/ui/sidebar-view-wire.ts lib/types/http-user-settings.ts components/task/sidebar-filter/sort-picker.tsx components/task/sidebar-filter/sidebar-view-editor.tsx components/task/sidebar-filter/task-row-settings.tsx components/task/task-session-sidebar-switcher-props.ts components/task/mobile/mobile-task-list.tsx components/task/task-switcher.tsx components/task/task-switcher-tree.tsx)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
git diff --check
```

If implementation creates extra helpers/components, add every changed source/test
file to the relevant lint/test command. Keep frontend function/file size limits.

## Files likely touched

- Existing partial backend edits under task models and `repository/sqlite/sidebar_task_query*.go`.
- `apps/backend/internal/task/handlers/sidebar_task_http.go` and query error tests.
- `apps/backend/internal/user/models/models.go`, DTO/settings mappings, and round-trip tests.
- `apps/web/lib/state/slices/ui/sidebar-view-types.ts`, `ui-slice.ts`,
  `sidebar-view-wire.ts`, and workspace/draft migrations and tests.
- `apps/web/lib/types/http-user-settings.ts` and sidebar query wire types.
- `apps/web/lib/sidebar/{apply-view,sidebar-local-order,sidebar-local-view,sidebar-task-source}.ts` and tests.
- New `sidebar-sort-chain.ts`/test and `task-tree-running.ts`/test beside existing sidebar helpers.
- `apps/web/lib/state/slices/task-overview-coverage.ts` and tests.
- Shared `sort-picker.tsx`, `sidebar-view-editor.tsx`, and focused rule subcomponents/tests.
- Desktop switcher props, `mobile/mobile-task-list.tsx`, and `task-row-settings.tsx`.
- Shared group renderer `task-switcher-tree.tsx`, `task-switcher.tsx`, and
  existing switcher/group tests for group inset and preserved child depth.
- All task locale catalogs and shared sidebar conformance fixtures.

## Dependencies

None. Read the revised requirement/design and current partial diff before editing.

## Risks

Old single-sort comparator tiebreaks must not suppress secondary criteria.
Partial fixed-preset SQL joins must work when Running is secondary.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/sidebar-running-first-activity-sort.md)
- [Design](../../specs/ui/system-design/sidebar-running-first-activity-sort.md)
- [Proposed decision](../../decisions/2026-10-06-composable-sidebar-sort-rules.md)

## Results

Completed. Focused Go tests passed for task models, handlers, SQLite, and user
settings. PostgreSQL-only tests were skipped because `KANDEV_TEST_POSTGRES_DSN`
was unset. The original combined focused frontend batch passed (23 files, 342
tests). After review remediation, the final focused regression batch passed (7
files, 235 tests), covering page-2 retention during a real status-summary update,
equal full-settings broadcasts, `groupIndent` writes and acknowledgements, and
authoritative missing-primary-summary behavior shared by local and SQL ranking.
`pnpm run typecheck`, targeted ESLint, translation synchronization, pseudo-locale,
i18n completeness, and changed-line ratchet checks passed. Group indentation
defaults, expanded-only editing, persistence, and shared rendering are covered by
state, component, and desktop/phone browser tests.
Follow-up PR verification (2026-10-07) used an isolated PostgreSQL 16 database;
the focused sidebar and user-settings Go command passed with PostgreSQL enabled.
The shared provider-color conformance fixture passed on PostgreSQL and SQLite.
CI exposed excess RSS in activity sorting with state grouping. Restricting forced
materialization to query shapes that share the recursive ancestor projection
brings the single-projection activity/state reproducer under the configured
memory budget while preserving the combined running/activity cases. Targeted
Linux pool-memory cases for activity/state, running/activity/state, and state
sorting in both directions passed under the configured native and RSS limits.
