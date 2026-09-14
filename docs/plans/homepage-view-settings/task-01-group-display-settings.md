---
id: "01-group-display-settings"
title: "Group homepage display settings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-004
acceptance_criteria:
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-004.1
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-004.2
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-004.3
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-004.4
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-004.5
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-004.6
system_design:
  - ../../specs/ui/system-design/task-listing-display-preferences.md
---

# Task 01: Group homepage display settings

## Summary

Deliver collapsible Home display groups with live summaries on desktop and
inside the existing mobile drawer. Preserve every current setting action.

## In scope

Shared disclosure presentation, Home composition, summary derivation, translations,
focused unit and browser coverage, affected test-helper updates, and public docs.

## Out of scope

New settings, backend changes, persistent expansion, sidebar behavior changes,
Columns redesign, and unrelated cleanup.

## Acceptance

1. All applicable groups match UI-01/UI-02 and pass AC 004.1-004.4.
2. Keyboard, nested controls, phone geometry and dismissal pass AC 004.5-004.6.
3. Existing affected suites pass; localized documentation and exact results are recorded.

## ASCII UI preview

### UI-01: Desktop display menu

Entry: Home display-settings button. Before: flat Workflow, Repository, Board
sort, Priority filter, and Preview panel sections, as shown by current source.
After, with all groups collapsed:

```text
+------------------------------------------+
| FILTERS                                > |
| All workflows, All repositories          |
| All priorities                           |
+------------------------------------------+
| SORT                                   > |
| Newest first                             |
+------------------------------------------+
| PREVIEW PANEL                          > |
| Off                                      |
+------------------------------------------+
```

UI-01 expanded Filters (Sort/Preview can also remain open):

```text
+------------------------------------------+
| FILTERS                                v |
| Workflow                                 |
| [All Workflows                         v]|
| Repository                               |
| [All repositories                      v]|
| Priority                                 |
| [ ] Critical       [ ] High              |
| [ ] Medium         [ ] Low               |
| [Plugin filter controls, when present]   |
+------------------------------------------+
| SORT                                   > |
| Newest first                             |
+------------------------------------------+
| PREVIEW PANEL                          > |
| Off                                      |
+------------------------------------------+
```

Expanded Sort contains the existing Board sort select. Expanded Preview panel
contains Open preview on click and its existing explanation. List adds a List
rows group with Show task details and its explanation. Threads omits inapplicable
groups. Empty/loading repositories keep the disabled selector and current
placeholder. Long summaries wrap; options scroll within existing Select menus.

### UI-02: Phone Home menu drawer

Entry: existing Home menu button, Board active; Filters expanded.

```text
+--------------------------------+
| Menu                           | fixed
+--------------------------------+
| [Existing workspace/navigation]|
| [Existing view/search controls]|
| Display options                |
| FILTERS                      v |
| Repository                     |
| [All repositories            v]|
| Priority                       |
| [ ] Critical    [ ] High       |
| [ ] Medium      [ ] Low        |
| SORT                         > |
| Newest first                   |
| PREVIEW PANEL                > |
| Off                            |
| [Existing Columns controls]    |
| [Other existing menu content]  |
+--------------------------------+
| Safe-area clearance            |
+--------------------------------+
```

The body is one scroll region. Workflow stays in existing phone Board navigation.
Phone List/Threads follow existing visibility flags. Headers and changed touch
controls have >=44px hit areas. Tablet keeps its existing Sheet composition.
Group order, summary hierarchy, independent expansion, and containment are
structural requirements (AC 004.1-004.6); spacing, capitalization, and the
illustrative two-column priority grid use existing tokens and available width.

See the [combined preview](plan.md#ascii-ui-preview). Keep both copies synchronized.

## Verification

Run from repository root. Install once if this worktree has no dependencies.
Use TDD: add focused failing behavior tests, implement, then run these checks.
The managed E2E runner rebuilds before testing; run desktop and mobile sequentially.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/display-settings-summary.test.ts components/kanban-display-dropdown.test.tsx components/kanban/mobile-menu-sheet.test.tsx components/task/sidebar-filter/sidebar-settings-disclosure.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/kanban/display-settings-groups.spec.ts tests/kanban/board-priority-sort-filter.spec.ts tests/kanban/workflow-filter.spec.ts tests/kanban/pipeline-view.spec.ts tests/task/task-listing-view-preferences.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/kanban/mobile-display-settings-groups.spec.ts tests/kanban/mobile-board-priority-sort-filter.spec.ts tests/kanban/mobile-step-visibility-filter.spec.ts tests/task/mobile-task-listing-display.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Audit `display-button`, `display-workflow-filter`, `display-repository-filter`,
`display-board-sort`, priority checkbox IDs, and preview/List labels in E2E
specs and page helpers. Update affected tests and add exact commands here for
all changed suites before completion. If new helper test files are introduced,
add them to the Vitest command. Compare rendered desktop/phone captures with
UI-01/UI-02 during these focused runs; record deviations or blockers.

## Files likely touched

- `apps/web/components/kanban-display-dropdown.tsx` and its `.test.tsx`.
- New `apps/web/components/kanban/mobile-display-options.tsx` for the phone
  display groups and their existing controls.
- `apps/web/components/kanban/mobile-menu-sheet.tsx` and its `.test.tsx`.
- New `apps/web/components/display-settings-disclosure.tsx`, if extracting.
- `apps/web/components/task/sidebar-filter/sidebar-settings-disclosure.tsx`
  and its `.test.tsx`, only for a compatible shared presentation extraction.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/kanban.json`.
- New `apps/web/e2e/tests/kanban/display-settings-groups.spec.ts` and
  `mobile-display-settings-groups.spec.ts`.
- Existing E2E specs/page helpers identified in the plan and locator audit.
- `docs/public/tasks-and-workflows.md` and this package's result/status sections.

Read `hooks/use-kanban-display-settings.ts`, `hooks/use-mobile-menu-sheet-state.ts`,
`lib/kanban/kanban-sort.ts`, `lib/tasks/task-priority.ts`, and plugin registry
key helpers as integration inputs; retain their business logic.

## Dependencies

None. One complete vertical slice.

## Risks

Nested portal focus, summary overflow, hidden-locator regressions, and accidental
changes to phone visibility or sidebar sizing. Cover these in focused tests.

## Parallelism

`sequential`

## Inputs

Requirement 004, its grouped-settings design section, ADR 0041, the accepted
preview, scoped web guidance, mobile-parity, TDD, E2E and docs-maintainer skills.

## Results

Implemented the grouped display-settings layout across desktop and phone
surfaces. Added the shared disclosure component and summary helpers, preserved
existing setting callbacks and visibility rules, kept Columns outside the new
groups, and added localized host copy for all supported catalogs. Updated the
affected unit and E2E selectors so hidden controls are expanded intentionally.

Focused unit verification passed:

```text
pnpm exec vitest run components/display-settings-summary.test.ts components/kanban-display-dropdown.test.tsx components/kanban/mobile-menu-sheet.test.tsx components/task/sidebar-filter/sidebar-settings-disclosure.test.tsx
4 files, 35 tests passed
```

The new desktop and phone group E2E suites passed through the managed runners:

```text
pnpm e2e:run --project chromium tests/kanban/display-settings-groups.spec.ts
pnpm e2e:run --project mobile-chrome tests/kanban/mobile-display-settings-groups.spec.ts
```

The required desktop regression run passed all 29 tests:

```text
pnpm e2e:run --project chromium tests/kanban/display-settings-groups.spec.ts tests/kanban/board-priority-sort-filter.spec.ts tests/kanban/workflow-filter.spec.ts tests/kanban/pipeline-view.spec.ts tests/task/task-listing-view-preferences.spec.ts
29 passed
```

The desktop surface uses Popover focus handling so the display trigger opens with
the keyboard, normal Tab traversal reaches revealed controls and later groups,
and Escape returns focus to the trigger. The keyboard regression uses the actual
trigger plus Enter, Tab, Space, and Select typeahead; it does not directly focus
group headers. Nested Select dismissal and the existing mobile drawer behavior
remain covered by the desktop and mobile regression commands above.

The required phone regression run passed all 12 tests:

```text
pnpm e2e:run --project mobile-chrome tests/kanban/mobile-display-settings-groups.spec.ts tests/kanban/mobile-board-priority-sort-filter.spec.ts tests/kanban/mobile-step-visibility-filter.spec.ts tests/task/mobile-task-listing-display.spec.ts
12 passed
```

The final checks also passed: `pnpm run typecheck`, `pnpm run lint`,
`pnpm run i18n:check`, `python3 scripts/list-docs.py validate` (267 decisions,
894 specifications), `python3 scripts/lint-spec-files.py --all`,
`node --test scripts/validate-public-docs.test.mjs` (62 tests),
`node scripts/validate-public-docs.mjs` (46 published docs), and `git diff --check`.
