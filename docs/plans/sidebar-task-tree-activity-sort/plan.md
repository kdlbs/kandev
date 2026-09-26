---
created: 2026-09-25
status: complete
requirements:
  - REQ-UI-SIDEBAR-LAST-ACTIVITY-SORT-002
system_design:
  - ../../specs/ui/system-design/sidebar-task-tree-activity-sort.md
legacy_specs: []
---

# Implementation Plan: Sidebar Task Tree Activity Sort

## Overview

Make Last activity sort rank each visible task tree by its newest included member. The shared sort path serves both desktop and phone, so one focused work order can add the derived key and prove it through unit and rendered scenarios.

## Scope

### In scope

- Apply the tree's latest activity to root and nested sibling placement for ascending and descending Last activity views.
- Respect active filters, collapse state, stable ties, and existing pin and manual-order precedence.
- Prove the shared result in the desktop sidebar and phone task-switcher drawer.

### Out of scope

- Changes to how a task's own activity timestamp is published or displayed.
- Changes to Updated, state, custom, or other sort keys; group-heading order; task-row presentation; saved-view storage.

## Technical approach

`applyView` in `apps/web/lib/sidebar/apply-view.ts` already filters before building `subTasksByParentId`, then calls `applySort`. Add a pure iterative resolver in `apps/web/lib/sidebar/task-tree-activity.ts` that uses each task's Last activity fallback and memoizes the maximum for each included subtree. The phone task-switcher projection must pass through the task's own activity timestamp instead of summary freshness. Use the derived value only in the `lastActivityAt` comparator branch of `applySort`. Keep the stable index tiebreak, sort-direction sign, `applyGroup`, pin floating, and `applySubtaskOrder` in their current order. Neither the backend nor `TaskSwitcherItem` changes.

## ASCII UI preview

`UI-01: Desktop Tasks sidebar`, entry point: a saved view with Last activity descending. The example rows share one existing group. Times are illustrative; each label is the task's own time.

```text
CURRENT                             EXPECTED
Tasks | Last activity v             Tasks | Last activity v
  Standalone task          2d          v Parent task           4d
  v Parent task           4d            Child task           10s
    Child task           10s          Standalone task        2d
```

`UI-02: Phone task-switcher drawer`, entry point: the existing Tasks picker and shared saved view.

```text
Tasks                         [New]
[Last activity view v] [Filters]
---------------------------------
v Parent task              4d
  Child task              10s
Standalone task            2d
```

Only tree order is required to change. The existing desktop sidebar and phone drawer keep their respective headers and navigation. Their task lists remain the scroll owners. A collapsed parent keeps the same rank, with its child row hidden. No new controls or copy are required. These previews cover `AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.1`, `.2`, `.4`, `.5`, and `.6`.

## Tests

`apps/web/lib/sidebar/task-tree-activity.test.ts` and `apply-view.test.ts` will cover `AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.1` through `.5`: both directions, deep descendants, sibling subtrees, equal values, fallbacks, filtered-out descendants, orphan promotion, collapsed trees, and precedence of pin, manual order, and other sort modes. Include a deep chain to catch recursion limits.

## E2E tests

- `apps/web/e2e/tests/task/sidebar-task-tree-activity-sort.spec.ts` (`chromium`) covers the desktop parent-versus-standalone order and individual row time after a child activity update (`AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.1`, `.5`, `.6`).
- `apps/web/e2e/tests/task/mobile-sidebar-task-tree-activity-sort.spec.ts` (`mobile-chrome`) covers the same saved-view order in the phone drawer and touch navigation to the child (`AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.1`, `.6`).

## Work orders

- [x] [Task 01: Sort visible task trees by activity](task-01-sort-visible-task-trees.md)

## Verification results

Task 01 is complete. The focused unit suite passed (3 files, 83 tests), frontend typecheck and changed-file ESLint passed, including mixed fractional precision and malformed RFC3339 coverage, and focused desktop (`chromium`) and phone (`mobile-chrome`) Playwright tests passed. Specification validation and lint passed.

## Risks

- A recursive per-comparison tree walk would grow with list size and can overflow on deep trees; compute the map once per sort.
- A child excluded by filtering must not influence its former parent's key. The resolver must consume the filtered parent-child map, not the original task list.
- The parent row's own time may appear older than its position. Keep that established row meaning and prove the child's activity explains the tree's placement.
- The phone task drawer must use each task's activity time for both Last activity sorting and row display, with task-local fallbacks when the summary field is absent.
