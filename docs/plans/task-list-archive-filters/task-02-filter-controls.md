---
id: "02-filter-controls"
title: "Organize desktop and phone list filters"
status: done
wave: 2
depends_on:
  - "01-archive-navigation"
plan: "plan.md"
requirements:
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-001
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-002
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-003
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-004
acceptance_criteria:
  - AC-TASKS-LIST-ARCHIVE-FILTERS-001.3
  - AC-TASKS-LIST-ARCHIVE-FILTERS-001.5
  - AC-TASKS-LIST-ARCHIVE-FILTERS-002.2
  - AC-TASKS-LIST-ARCHIVE-FILTERS-002.3
  - AC-TASKS-LIST-ARCHIVE-FILTERS-002.4
  - AC-TASKS-LIST-ARCHIVE-FILTERS-002.5
  - AC-TASKS-LIST-ARCHIVE-FILTERS-003.1
  - AC-TASKS-LIST-ARCHIVE-FILTERS-003.2
  - AC-TASKS-LIST-ARCHIVE-FILTERS-003.3
  - AC-TASKS-LIST-ARCHIVE-FILTERS-003.4
  - AC-TASKS-LIST-ARCHIVE-FILTERS-003.5
  - AC-TASKS-LIST-ARCHIVE-FILTERS-004.2
  - AC-TASKS-LIST-ARCHIVE-FILTERS-004.3
  - AC-TASKS-LIST-ARCHIVE-FILTERS-004.4
  - AC-TASKS-LIST-ARCHIVE-FILTERS-004.5
system_design:
  - ../../specs/tasks/system-design/task-list-archive-filters.md
---

# Task 02: Organize desktop and phone list filters

## Summary

Make the existing query controls visible together on desktop and usable through
a phone drawer. Complete applied indicators, clear actions, query feedback,
last-page recovery, and user documentation without adding new filter dimensions.

## In scope

- Desktop toolbar and permanent List search. Keep the archive row from Task 01.
- Phone Filters drawer, immediate selections, applied indicators, count, and focus return.
- Clear filters, individual clear actions, loading/error/unavailable/empty
  presentation, current-query Retry, pull-to-refresh, and last-page recovery.
- Tasks-only removal of duplicate scope/search controls. Keep sort, group,
  details, plugin contributions, and other listing routes intact.
- Complete translations, focused desktop/phone tests, and public documentation.

## Out of scope

New filter criteria, saved views, global presentation changes, backend/schema
changes, plugin API changes, archive lifecycle changes, and broad QA or review passes.

## Acceptance

1. Desktop and phone render UI-02/UI-03 with shared effective query values.
   Clear actions preserve archive and display settings. No conflicting duplicate
   workflow/repository or hidden List search control remains.
2. UI-04 distinguishes every specified result state. Refresh and row actions
   preserve current filters, and removal from the final page recovers correctly.
3. Targeted tests prove keyboard/touch use, focus return, viewport containment,
   control dimensions, history/persistence, and unchanged adjacent listing flows.
   Public how-to guidance describes the implemented controls.

## UI preview

Use [the full previews](plan.md#ui-preview). These structured rows retain the
same labels and replace ASCII under the active task instructions.

**UI-02: Desktop List, Archived selected**

| Order | Region |
| --- | --- |
| 1 | Active / **Archived** / All, Search tasks, accepted result total |
| 2 | Workflow, Repository, Clear filters. separate Sort and Group controls |
| 3 | Removable workflow/repository indicators, wrapping within width |
| 4 | Task rows and pagination in the existing list scroller |

**UI-03: Phone List and Filters drawer**

| Surface | Controls in order |
| --- | --- |
| List | Existing header. archive row. search and Filters (count). indicators. rows |
| Filters drawer | Fixed heading. Workflow, Repository, Clear filters in one scrolling body. Done footer |
| View options | Existing Sort, Group, and row details |

Use at least 44px phone targets, nominal 48px Done, safe-area clearance,
dynamic viewport limits, and contained overscroll. The desktop base remains 28px.

**UI-04: Result feedback with controls retained**

Loading hides previous-scope results. Failure offers Retry. An unavailable scope
offers clear or replacement. An empty archive has its own message. An empty
filtered result offers Clear filters. Coverage: `002.4`, `003.1` through `.5`,
`004.2`, `004.4`, `004.5`.

## Verification

Run from the repository root. Use TDD for clear/refresh/feedback logic. Managed
E2E runs rebuild current production assets. Run desktop and mobile sequentially.

```bash
(cd apps/web && pnpm exec vitest run hooks/use-tasks-list-query-state.test.tsx app/tasks/tasks-page-client.query.test.tsx app/tasks/tasks-list-view.test.tsx hooks/use-task-listing-view.test.tsx components/kanban/kanban-header-mobile.test.tsx components/kanban-display-dropdown.test.tsx app/tasks/tasks-page-client.mr-hydration.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=12288 pnpm run typecheck)
(cd apps/web && pnpm exec eslint app/tasks/tasks-list-filters.tsx app/tasks/tasks-list-filters-drawer.tsx app/tasks/tasks-list-controls.tsx app/tasks/tasks-page-client.tsx app/tasks/tasks-page-content.tsx app/tasks/tasks-list-view.tsx app/tasks/tasks-list-read-feedback.tsx hooks/use-tasks-list-data.ts hooks/use-tasks-list-query-state.ts components/kanban/kanban-header.tsx components/kanban/kanban-header-mobile.tsx components/kanban/mobile-menu-sheet.tsx components/kanban/mobile-menu-task-list-options.tsx components/kanban-display-dropdown.tsx hooks/use-mobile-menu-sheet-state.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/task/task-list-filters.spec.ts tests/task/task-list.spec.ts tests/task/task-listing-view-preferences.spec.ts tests/task/task-list-archive-navigation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-task-list-filters.spec.ts tests/task/mobile-task-list-search.spec.ts tests/task/mobile-task-listing-display.spec.ts tests/task/mobile-task-list-archive-navigation.spec.ts tests/plugins/mobile-plugin-topbar.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Extend `task-list-filters.spec.ts` with fine-pointer checks at 767px and 768px.
The mobile project covers Pixel 5 taps and a wider coarse-pointer viewport.
Measure the actual trigger, fields, and Done target, not their utility classes.
Check drawer viewport containment, one internal scroller, focus return, and
zero document horizontal overflow. Inspect rendered screenshots.

Migrate every affected removed search/scope selector before running its spec.
At minimum, `mobile-task-list-search.spec.ts` must use the permanent search
field. Existing archive grouping and rich-row assertions must remain.
Update exact commands if an additional existing test file changes.

## Files likely touched

- New `apps/web/app/tasks/tasks-list-filters.tsx` and
  `tasks-list-filters-drawer.tsx`. Browser tests cover their presentation.
- Existing Tasks page/client/content/list-controls/list-view files and query tests.
- `apps/web/hooks/use-tasks-list-query-state.ts`, `use-tasks-list-data.ts`, and tests.
- New `apps/web/app/tasks/tasks-list-read-feedback.tsx` for current-read feedback.
- Shared Tasks branches in `components/kanban/kanban-header.tsx`,
  `kanban-header-mobile.tsx`, `mobile-menu-sheet.tsx`,
  `mobile-menu-task-list-options.tsx`, `components/kanban-display-dropdown.tsx`,
  and `hooks/use-mobile-menu-sheet-state.ts`.
- Desktop task-list, filter, and phone search browser specs.
  New `apps/web/e2e/tests/task/mobile-task-list-filters.spec.ts` and the
  existing `e2e/tests/plugins/mobile-plugin-topbar.spec.ts`.
- Compatibility coverage in `app/tasks/tasks-page-client.facet-selection.test.tsx`,
  `app/tasks/tasks-page-search-debounce.test.tsx`, `app/tasks/tasks-list-controls.test.tsx`,
  `components/kanban/mobile-menu-task-list-options.test.tsx`,
  `e2e/tests/task/task-search-debounce.spec.ts`, and `e2e/tests/plugins/task-list-facet.spec.ts`.
  These paths are relative to `apps/web/`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/{tasks,kanban,common}.json`
  as needed. Generate Traditional Chinese and pseudo through repository scripts.
- `docs/public/tasks-and-workflows.md`, sections **Find and organize tasks**
  and **Archive, unarchive, and delete**. Keep the page's how-to purpose and links.
- Package status and results files after all required checks pass.

## Dependencies

Task `01-archive-navigation` must pass. Reuse its query model and response
ownership. Do not create separate phone filtering or request state.

## Risks

Shared display surfaces also serve Board and Threads. Remove duplicate controls
only for Tasks and keep plugin contributions. A desktop-only visibility class
must not leave 640-767px without the intended phone controls. Empty results,
failed reads, and metadata failures must remain distinct.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-list-archive-filters.md).
- [System design](../../specs/tasks/system-design/task-list-archive-filters.md),
  especially Query transitions through Verification and documentation.
- [Combined preview and coverage matrix](plan.md#ui-preview).
- Root and web `AGENTS.md`, `/tdd`, `/e2e`, `/mobile-parity`, `better-ui`,
  and `/docs-maintainer`.
- `MobileMenuSheet` drawer geometry, existing shared controls, filter E2E,
  phone search E2E, list row tests, and grouping tests.

## Results

Complete on 2026-10-20.

- Behavioral RED covered retained refresh callbacks and recovery after the final
  page loses its last row. Initial unit validation passed 99 tests in 13 suites,
  including the display-dropdown and MR-hydration regressions.
- All 25 unique desktop and 9 unique phone browser cases passed on the final
  production assets. The [plan results](plan.md#verification-results) distinguish
  the initial final-build runs from the focused assertion/interaction reruns.
- Desktop filtering combines archive membership, workflow, repository, and text.
  The pagination case finds task 26 from the second page and resets page size.
  Phone filtering covers immediate drawer choices, indicators, clear actions,
  failed-read Retry, focus return, and one contained scroll body.
- Screenshots match UI-02/UI-03. Measured phone/coarse-pointer targets are at
  least 44px, with 28px ordinary desktop controls and no horizontal overflow.
- Typecheck, changed-file lint, `i18n:check`, and `i18n:ratchet` passed. All
  supported locale catalogs contain the new copy. Traditional Chinese and
  pseudo were generated with repository scripts.
- Public documentation validator tests passed all 63 cases; 48 published pages
  validated. Public task guidance describes the archive command, explicit modes,
  desktop selectors, phone drawer, and clearing behavior.

Presentation is covered in Playwright. No markup-only filter unit test was
added. Existing archive and grouping coverage was retained while selectors
were migrated. The user approved manual testing and requested publication.

PR integration retained the current plugin facet preference contract and updated
inherited control/search tests for the visible List filters. A regression proves
that facet tokens use a built-in API sort. Search tests prove one debounce window,
immediate clear, retained scope, cancellation, and stale-response rejection.
After the first base integration, validation passed 153 unit tests in 20 suites, typecheck, ESLint for
40 changed TypeScript files, and all 32 desktop and 11 phone browser cases.
See [PR integration validation](plan.md#pr-integration-validation) for the evidence.

The final base update extends task deletion. The List layers preserve the shared
`DeleteTaskParams` and cleanup feedback. The final focused unit run passed 158
tests in 21 suites; typecheck and affected-file lint passed. The fresh-build
desktop and phone deletion smoke checks passed.

PR review remediation uses translated interpolated messages for both applied
filter labels and corrects the phone search guide. All locale checks passed.
See [PR review remediation](plan.md#pr-review-remediation).

CI remediation updates inherited phone menu and empty-state assertions to the
shipped List controls. Search-toggle coverage remains on Kanban. Filter label,
touch target, and focus-return coverage uses List's Filters drawer. Startup and
Home coverage retains its route, workspace, reload, and preference assertions.
See [CI remediation](plan.md#ci-remediation) for current validation results.
