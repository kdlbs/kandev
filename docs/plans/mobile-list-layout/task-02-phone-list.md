---
id: "02-phone-list"
title: "Phone List uses shared saved views"
status: done
wave: 2
depends_on:
  - "01-query-adapter"
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-LIST-VIEWS-001
  - REQ-UI-MOBILE-LIST-VIEWS-004
acceptance_criteria:
  - AC-UI-MOBILE-LIST-VIEWS-001.1
  - AC-UI-MOBILE-LIST-VIEWS-001.2
  - AC-UI-MOBILE-LIST-VIEWS-001.3
  - AC-UI-MOBILE-LIST-VIEWS-001.4
  - AC-UI-MOBILE-LIST-VIEWS-001.5
  - AC-UI-MOBILE-LIST-VIEWS-001.6
  - AC-UI-MOBILE-LIST-VIEWS-001.7
  - AC-UI-MOBILE-LIST-VIEWS-004.2
  - AC-UI-MOBILE-LIST-VIEWS-004.6
system_design:
  - ../../specs/ui/system-design/mobile-list-saved-views.md
---

# Task 02: Phone List uses shared saved views

## Summary

Render the actual workspace sidebar views in primary phone List. Reuse shared
rows, mutations, query state, and pagination while keeping wider List independent.

## In scope

- Before UI edits, capture pre-change phone screenshots and computed heights at 393x852.
- Keep the phone action/dialog host mounted across breakpoint changes while
  disabling its result-page query when the phone List is inactive.
- Mount only one active result-query client at a time.
- Add a phone consumer of the shared query adapter, tabs, row presentation, and pagination.
- Replace fixed archive tabs with actual saved-view names and order.
- Keep task navigation, hierarchy, groups, pins, actions, and live updates functional.
- Preserve default/missing view fallback and workspace isolation.
- Implement task-body loading, empty, read-error, and Retry states.
- Start `mobile-list-saved-views.spec.ts` with shared-view and ordering scenarios.

## Out of scope

The new search toggle/editor entry belongs to Task 03. Header blur and menu
spacing belong to Task 04. Do not rewrite desktop List preferences or Threads views.

## Acceptance

1. Phone List and desktop sidebar use the same saved identity and complete
   ordered result, including the four-rule chain and a dataset larger than 100 rows.
2. Empty/default, grouping, live update, task actions, and workspace changes use
   existing shared semantics with one active result-query consumer. Rename,
   edit, create, link, and subtask dialogs retain their drafts through
   orientation changes.
3. Rendered UI follows UI-01/UI-08 and preserves measured control heights.
   Desktop standalone List and sidebar still pass their relevant flows.

## ASCII UI preview

The final control placement is [UI-01 in the plan](plan.md#ui-01-phone-list-search-closed).
Task 03 supplies the header search control. Do not add a permanent input row to finish this work order.

### UI-01: Phone List, search closed

Entry: ordinary `/tasks`, selected saved view, no result refinements.
AC: 001.1-.7, 002.1, 004.1-.2, 004.6.

```text
+--------------------------------------+
| Default                              |
| List v                       [S] [=] |
+--------------------------------------+
| [All tasks] [Archived] [My view]      |
+--------------------------------------+
|                                      |
| | Running task title       2m  [...] |
| | Another task title      12m  [...] |
| | Task title               1h  [...] |
|                                      |
| [Previous]             [Next]        |
|                                [+]   |
+--------------------------------------+
```

Header and tab row do not scroll vertically. Tabs own contained horizontal
scrolling when needed. The task body owns vertical scrolling. Pagination appears
only when required; the FAB clears the last row and bottom safe area.

The task-body states follow [UI-08](plan.md#ui-08-task-body-feedback).
The wider consumers follow [UI-07](plan.md#ui-07-wider-consumers-compatibility).

## Verification

```bash
pnpm --dir apps/web exec vitest run app/tasks/mobile-tasks-page-client.test.tsx app/tasks/tasks-list-view.test.tsx hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts
pnpm --dir apps/web e2e:run --project mobile-chrome tests/task/mobile-list-saved-views.spec.ts -- --grep 'saved views:|sort chain:|pagination:'
pnpm --dir apps/web e2e:run --project chromium tests/task/workspace-sidebar-views.spec.ts tests/task/sidebar-running-first-activity-sort.spec.ts tests/task/task-list-filters.spec.ts
git diff --check
```

Use behavioral RED tests for the independent List enum/order mismatch. Record
baseline heights before changing markup. Open the resulting UI-01 screenshots;
record task order, density, containment, row navigation, and differences.
Required E2E test-name prefixes are `saved views:`, `sort chain:`, and `pagination:`.

## Files likely touched

- `apps/web/app/tasks/tasks-page-client.tsx` and `tasks-page-content.tsx`.
- New `apps/web/app/tasks/mobile-tasks-page-client.tsx` and its test.
- New phone list/view-tab presentation components under `apps/web/app/tasks/`.
- `apps/web/components/task/mobile/mobile-task-list.tsx` and shared row builders.
- `apps/web/components/task/task-switcher.tsx` only for a shared presentation seam.
- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts` if integration needs a narrow adjustment.
- New `apps/web/e2e/tests/task/mobile-list-saved-views.spec.ts`.

## Dependencies

Task 01 supplies request identity and refinement composition.

## Risks

Re-reading the global view inside a renderer can disagree with the effective
query. Generic List boot rows do not prove sidebar coverage. Do not mount two
consumer trees or lose controller-owned task dialogs on orientation changes.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/mobile-list-saved-views.md), 001 and 004.
- [Design](../../specs/ui/system-design/mobile-list-saved-views.md), Shared query and Phone composition.
- Existing `MobileTaskList`, `useSheetData`, row action hooks, and workspace-view E2E fixtures.

## Results

Implemented the responsive phone List with workspace-scoped saved-view tabs,
shared query, row actions, and pagination. Kept the standalone List client on the
desktop branch. Added workspace-isolation, sort-chain-across-pages, bounded
browsing, and row-navigation E2E coverage. Updated the responsive archive-filter
regression to expect saved-view tabs below 768px and retain the desktop archive
control-height check at 768px.

Verification passed:

- Focused unit tests: 4 files, 35 tests.
- `mobile-list-saved-views.spec.ts`: 3 tests passed.
- Desktop sidebar/List regression group: 11 tests passed.
- `git diff --check` passed at Task 02 completion.

The pre-change image and computed heights remain recorded in
[`verification.md`](verification.md). Final rendered comparison is owned by
Task 05.

### Review remediation (2026-10-27)

- Added a phone E2E that opens Rename, types a draft, crosses 768px in both
  directions, and confirms the same dialog and text remain present.
- The mobile task page hook disables the bounded result request while its
  stable dialog host is inactive. A focused hook test verifies that an
  incomplete inventory does not start the hidden phone page request.
- The focused phone archive-navigation and link-override E2E cases pass as
  recorded in [`verification.md`](verification.md).
