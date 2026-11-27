---
id: "07-shared-sidebar-state"
title: "Restore small sidebar views from shared task state"
status: in_progress
wave: 7
depends_on:
  - "06-firefox-task-paint"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.3
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.9
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.19
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.20
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
---

# Task 07: Restore shared-store sidebar rows

The user explicitly requests restoring Zustand reuse in PR #4062, cancelling the
seeded preview and using CI for all further tests. The existing implementation
always waits for a separate sidebar query, even with complete homepage snapshots.
Its bounded page cache duplicates task projections; its revision guard can discard
a completed read during live activity. These facts do not prove an unbounded heap
leak, and this change must not claim to repair an unmeasured leak.

## Implementation

1. Add a shared inventory selector/hook beside `use-workspace-sidebar-tasks.ts`.
   Require every workflow snapshot in the current workspace, reject failed and
   placeholder snapshots, preserve authorization/context boundaries and stop
   collecting above 100 tasks. Reuse the old aggregation and WIP queue projection.
2. Disable `use-sidebar-task-page.ts` for that store-backed active view. Release
   requests and response ownership, evict the redundant retained view page, and
   stop refresh timers. Keep server paging for larger, missing and archived data.
3. Route desktop and phone grouping through the existing `applyView` engine for
   store-backed lists. Restore its previous small-list ordering; server paging
   retains canonical ordering, so text/tie order can differ at the threshold.
4. Add unit regressions for complete/partial/failed/foreign inventories, 100/101
   tasks, live store changes, request cancellation and page eviction. Add desktop
   and phone browser regressions that hold sidebar requests while complete
   homepage data remains usable, including a first visit to a saved view.

## UI and mobile contract

Keep the desktop sidebar and native phone picker. No additional controls, copy,
loading status or scroll owners. Preserve filtering, grouping, pin order, task
selection, keyboard dismissal and conversation ownership.

```text
Desktop: | views | existing task rows | selected conversation |
Phone:   | task picker: views, existing rows | -> conversation
Before:  complete homepage tasks -> separate query -> sidebar rows
After:   complete homepage tasks -> shared Zustand projection -> sidebar rows
```

## Verification and results

No local test execution, per user instruction. CI must run affected Vitest suites,
typecheck, lint, specs and desktop/mobile Playwright coverage. Keep normal static
commit hooks. Do not start another seeded instance or claim memory measurements.
Implementation and CI results will be recorded before delivery.

## Expanded approved scope

The user subsequently selected [the full shared-state and query-memory package](../sidebar-query-memory/plan.md).
Its explicit coverage, normalized ownership, local/server conformance, and native
SQLite resource requirements supersede this work order's <=100 inventory shortcut.
Task 07 remains in progress until that package is implemented and CI is green.
