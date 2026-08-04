---
spec: docs/specs/ui/sidebar-archived-filter.md
created: 2026-08-04
status: done
---

# Implementation Plan: Sidebar Archived Filter Retirement

## Overview

Retire the filter dimension that cannot be satisfied by the sidebar's active
workflow snapshots, and use the existing saved-view migration boundary to
remove stale `archived` clauses safely. Then prove the shared editor behavior
on desktop and mobile without changing task fetching, archive lifecycle, or
the synthetic current-archived row.

## Root cause

PR #644 added `archived` to the filter registry and pure `applyView` engine,
but the sidebar continued to aggregate `ListTasks` workflow snapshots. That
repository query deliberately requires `archived_at IS NULL`, and
`task.updated` removes archived tasks from both Kanban caches. The only
archived sidebar item is a synthetic placeholder for a directly opened
archived task, so the normal **Archived: Show** path always filters an
archived-free collection to an empty list.

## Backend

No backend changes. `ListTasks` remains the active workflow-task contract, and
`ListTasksByWorkspace(..., includeArchived=true)` remains the full Tasks page
and command-panel contract.

## Frontend

### Filter contract and saved-view migration

- `apps/web/components/task/sidebar-filter/filter-dimension-registry.ts`:
  remove the `archived` dimension metadata so neither responsive surface can
  select it.
- `apps/web/lib/state/slices/ui/sidebar-view-types.ts`: remove `archived` from
  the supported `FilterDimension` union while leaving archived task display
  state untouched.
- `apps/web/lib/state/slices/ui/ui-slice.ts`: remove `archived` from
  `KNOWN_DIMENSIONS`; the existing `migrateView` behavior then drops legacy
  clauses while preserving the rest of each view.
- `apps/web/lib/sidebar/apply-view.ts`: remove the unreachable archived
  extractor from the supported view engine.
- Update focused unit tests to replace the synthetic archived-filter assertion
  with migration and registry regressions.

## Mobile design contract

This is a shared option-retirement change, not a composition or touch change.
Desktop keeps the existing sidebar filter popover; mobile keeps the existing
`session-task-switcher-sheet.tsx` entry point and portaled filter popover. The
shared dimension registry and migration logic remain the single source of
truth. The nearest mobile exemplar is
`apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`; the rendered mobile
check opens the existing sheet and filter selector and verifies the obsolete
choice is absent. Scroll ownership, safe-area handling, touch targets, and
primary task navigation remain unchanged.

## Tests

- **Legacy view migration:** an `archived` clause fails the new
  `migrateView` regression before the code change and is removed afterward,
  while a neighboring valid clause and all other view fields survive.
  **File:** `apps/web/lib/state/slices/ui/ui-slice-migration.test.ts`.
- **Supported dimension registry:** the registry does not expose `archived`.
  **File:**
  `apps/web/components/task/sidebar-filter/filter-dimension-registry.test.ts`.
- **View-engine cleanup:** existing filtering tests continue to cover every
  supported dimension after removing the unreachable synthetic archived case.
  **File:** `apps/web/lib/sidebar/apply-view.test.ts`.

## E2E Tests

- **Desktop scenario:** open the sidebar filter editor, add a clause, open the
  dimension selector, and verify **Archived** is absent while a supported
  dimension remains available.
  **File:** `apps/web/e2e/tests/task/sidebar-filter.spec.ts`.
- **Mobile scenario:** open the mobile task-switcher sheet and the same filter
  selector, then verify **Archived** is absent without horizontal overflow.
  **File:** `apps/web/e2e/tests/task/mobile-sidebar-views.spec.ts`.

## Verification Results

- Task 01 unit suite: 4 files, 98 tests passed.
- Task 01 typecheck passed.
- Task 01 focused ESLint passed with no warnings or errors.
- Task 02 desktop E2E: 1 Chromium test passed via the managed production runner.
- Task 02 mobile E2E: 1 Pixel 5/mobile-chrome test passed via the managed production runner.
- `git diff --check` passed.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01 — Retire archived filter contract](task-01-retire-archived-filter.md)

Wave 2:

- [x] [Task 02 — Prove desktop and mobile behavior](task-02-sidebar-filter-e2e.md)

The tasks are sequential because the E2E assertions depend on the shared
registry and migration change. No parallel-safe task is identified.

## Risks

- Persisted views are backend-owned opaque data, so old `archived` clauses may
  continue arriving until a later user mutation rewrites the view. Frontend
  migration must remain idempotent on every hydration.
- Removing the clause intentionally makes archived-only saved views unfiltered;
  silently deleting or renaming the whole view would discard unrelated user
  preferences and diverge from existing removed-dimension migration behavior.
- The synthetic archived row still needs `TaskSwitcherItem.isArchived` for
  display and action guards; the repair must remove only filter support.

## Out of scope

- Fetching or paginating archived tasks in the sidebar.
- Backend, WebSocket, archive lifecycle, and full Tasks page changes.
- Sidebar layout or mobile interaction redesign.
