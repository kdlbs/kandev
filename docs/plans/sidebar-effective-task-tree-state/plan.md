---
created: 2026-09-09
status: implemented
requirements:
  - REQ-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001
system_design:
  - ../../specs/ui/system-design/sidebar-effective-task-tree-state.md
legacy_specs: []
---

# Implementation Plan: Sidebar Effective Task Tree State

## Overview

Correct the shared sidebar state resolver so active descendant work keeps the whole visible task
tree in an active state group. One vertical work order updates the pure derivation, its unit tests,
and the existing rendered sidebar regression.

## Scope

### In scope

- Derive one effective state for every included root task tree.
- Let running and scheduling descendants override parent review, waiting, failure, cancellation,
  and completion for state grouping and state sorting.
- Prevent the Completed group from containing a tree with an included non-completed member.
- Apply the same iterative, memoized derivation on desktop and mobile through the shared `applyView`
  path.
- Add targeted unit and desktop Playwright regression coverage.

### Out of scope

- Backend task-state transitions or session lifecycle changes.
- Row icons, labels, nesting, drag-and-drop, filters, or group-header presentation.
- Saved-view schema, group-collapse keys, localization, or public documentation.
- A separate mobile Playwright scenario because the change is shared data normalization with no
  mobile interaction or layout change.

## Technical approach

Extract `apps/web/lib/sidebar/effective-task-tree-state.ts` as the focused state resolver. It builds
the filtered `subTasksByParentId` graph once and uses an iterative, cycle-safe postorder walk with
memoized aggregates, so each task and edge is processed once. It returns the exact state-group key
and action bucket. Active work takes precedence over parent review and terminal buckets; Completed
remains valid only when every included tree member is completed. `applySort` and `applyGroup` reuse
this result instead of each minimizing `STATE_BUCKET_ORDER` independently.

Keep original `TaskSwitcherItem` objects in the grouped output so row-level status presentation is
unchanged. Preserve the existing behavior that filters run before descendant maps are built.

## Tests

- `apps/web/lib/sidebar/apply-view-effective-state.test.ts` maps
  `AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.1` through `.7` to pure grouping, sorting,
  deep traversal, filtering, completion, and row-object-preservation cases. Its deep-chain case
  guards the linear traversal contract.
- The regression test must fail before the correction for completed, review, and
  waiting-for-input parents with a running child.

## E2E tests

- Extend `apps/web/e2e/tests/task/sidebar-subtask-state-sort.spec.ts` for
  `AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.1`, `.3`, and `.5`. Seed a completed parent and an
  in-progress child, then assert the rendered parent tree is inside the `IN_PROGRESS` group.
- No new mobile test is required under `AC-UI-SIDEBAR-EFFECTIVE-TASK-TREE-STATE-001.6`: desktop and
  mobile share `applyView`, the change has no viewport-specific presentation, and existing mobile
  subtask tests prove the mobile tree consumes that grouped model.

## Work orders

- [x] [Task 01: Correct effective task-tree state](task-01-correct-effective-task-tree-state.md)

## Verification results

- `cd apps/web && pnpm exec vitest run lib/sidebar/apply-view-effective-state.test.ts` passed
  (26 tests).
- `cd apps/web && pnpm e2e:run tests/task/sidebar-subtask-state-sort.spec.ts` passed (1 test).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-subtasks.spec.ts`
  passed (1 test), confirming the shared mobile rendering path.
- Related sidebar unit tests passed (142 tests), TypeScript typecheck passed, targeted ESLint and
  Prettier checks passed, and the E2E sleep ratchet passed.

## Review follow-up

- Replaced the recursive per-task walk with the iterative memoized resolver in
  `apps/web/lib/sidebar/effective-task-tree-state.ts`.
- The deep-chain regression measures child-map lookups and fails the prior implementation's
  quadratic traversal; the focused effective-state suite passes with 27 tests.

## Risks

- Reusing the action bucket without an explicit active override would preserve the current defect
  because terminal and waiting states belong to the higher-priority `review` bucket.
- Direct-child-only traversal would leave nested active descendants incorrectly grouped.
- Divergent sorting and grouping resolvers could place a tree in one state section while ordering
  it as another.
