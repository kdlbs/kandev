---
id: "01-sort-visible-task-trees"
title: "Sort visible task trees by activity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-LAST-ACTIVITY-SORT-002
acceptance_criteria:
  - AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.1
  - AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.2
  - AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.3
  - AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.4
  - AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.5
  - AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.6
system_design:
  - ../../specs/ui/system-design/sidebar-task-tree-activity-sort.md
---

# Task 01: Sort Visible Task Trees by Activity

## Summary

Derive each visible task subtree's latest activity and use it for Last activity sorting. Prove that a recently active child moves its parent tree in both desktop and phone task lists while every row retains its own time.

## In scope

- Add a cycle-safe, iterative, memoized tree-activity resolver over the included parent-child map.
- Use the derived key only in the Last activity sort path, preserving direction, ties, filtered roots, and existing later ordering overrides.
- Preserve the phone task drawer's task-local activity field and fallback through its row projection.
- Add focused unit and desktop/phone Playwright coverage.

## Out of scope

- Backend generation of activity timestamps, Updated and other sort keys, group-heading sort, settings format, and visual layout changes.

## Acceptance

1. A direct or nested child's newer activity ranks its included parent tree ahead of an older tree in descending order, with the inverse order for ascending views; siblings follow the same subtree rule.
2. Filtered-out descendants have no effect, collapsed descendants still contribute, and ties, missing timestamps, pinning, manual order, and other sort keys retain their established behavior.
3. Desktop and phone rendered tests show the same parent-tree order, and the parent's visible time remains its own.

## ASCII UI preview

`UI-01: Desktop Tasks sidebar`, Last activity descending, before and after. The complete preview is in [plan.md](plan.md#ascii-ui-preview).

```text
CURRENT                       EXPECTED
Standalone task      2d         v Parent task       4d
v Parent task        4d           Child task       10s
  Child task        10s         Standalone task    2d
```

`UI-02: Phone task-switcher drawer`, same saved view and ordering.

```text
Tasks | Last activity view
v Parent task          4d
  Child task          10s
Standalone task        2d
```

The times illustrate each row's own activity; the tree's rank follows the child. These views cover `AC-UI-SIDEBAR-LAST-ACTIVITY-SORT-002.1`, `.2`, `.5`, and `.6`.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run lib/sidebar/task-tree-activity.test.ts lib/sidebar/apply-view.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-task-tree-activity-sort.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-task-tree-activity-sort.spec.ts)
```

Run the new focused unit case red before production edits, then green after the resolver and sort integration. The managed E2E runner builds the current web bundle before browser assertions.

## Files likely touched

- `apps/web/lib/sidebar/task-tree-activity.ts`
- `apps/web/lib/sidebar/task-tree-activity.test.ts`
- `apps/web/lib/sidebar/apply-view.ts`
- `apps/web/lib/sidebar/apply-view.test.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-item.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-item.test.ts`
- `apps/web/e2e/tests/task/sidebar-task-tree-activity-sort.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-tree-activity-sort.spec.ts`
- `apps/web/e2e/tests/task/sidebar-task-tree-activity-sort-helpers.ts`

## Dependencies

None. Read [the requirement](../../specs/ui/requirements/sidebar-last-activity-sort.md) and [the system design](../../specs/ui/system-design/sidebar-task-tree-activity-sort.md) before implementation.

## Risks

- The tree resolver must use filtered descendants and remain bounded for deep or malformed parent graphs.
- Existing mobile task views use the same `applyView` result but a separate drawer; prove the rendered order rather than assuming shared code is sufficient.

## Parallelism

`sequential`

## Inputs

- `applyView`, `applySort`, `separateSubtasks`, and existing `lastActivitySortValue` in `apps/web/lib/sidebar/apply-view.ts`.
- The existing state-tree resolver and its tests as a traversal pattern.
- Existing desktop Last activity and phone view E2E fixtures.

## Results

Implemented a cycle-safe iterative resolver that aggregates the newest activity across each included subtree. Filters run before aggregation, collapse does not affect it, and each row keeps its own activity timestamp. The phone row projection now carries `lastActivityAt` with task update and creation fallbacks while preserving summary freshness in `updatedAt`.

Activity keys compare chronological RFC3339 instants with full fractional precision and timezone offsets through the shared strict parser. Malformed or semantically invalid timestamps retain the existing lexical fallback instead of being normalized by `Date.parse`. Equal instants retain stable order, and missing timestamps keep their existing fallback order.

Validation passed:

- Focused Vitest suite after review: 3 files, 83 tests, including malformed calendar dates and shorthand timestamp fallback.
- Frontend typecheck.
- ESLint on all changed TypeScript files with `--max-warnings 0`.
- Desktop Playwright regression in `chromium`.
- Phone Playwright regression in `mobile-chrome`.
- Specification validation and lint.
- `git diff --check`.
