---
id: "03-shared-overviews"
title: "Normalize shared task overviews and coverage"
status: done
wave: 2
depends_on: ["01-bound-preparation"]
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.7
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.8
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.16
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.20
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.21
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.22
  - AC-UI-SIDEBAR-ARCHIVED-FILTER-002.24
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
  - ../../specs/ui/system-design/sidebar-shared-task-state.md
---

# Task 03: Normalize shared task overviews and coverage

## Summary

Make homepage and sidebar overview consumers resolve the same canonical Zustand task records.
Add explicit coverage metadata without yet enabling the local sidebar evaluator.
The existing server-backed sidebar remains functional throughout this work order.

## In scope

- Add a scoped `taskOverview` slice for lightweight entities, ownership, field availability, and accepted freshness.
- Convert workflow and sidebar memberships to IDs and expose stable compatibility selectors for existing consumers.
- Route snapshot/boot hydration, page responses, overview mutations, and relevant WebSocket updates through shared merge actions.
- Preserve board optimistic actions and existing Office behavior. Keep rich task/session data separate.
- Add optional snapshot `task_coverage` metadata for active workflow scope, total, completeness, and ordering profile.
- Propagate coverage through boot, HTTP types, hydration, and workflow fetch settlement.
- Track stale/incomplete coverage and bounded in-flight tombstones without trusting cached record counts.
- Release unowned entities and preserve the existing five-page/2 MiB/five-minute reusable-page limits, including entity bytes.

## Out of scope

Local view evaluation, source selection, UI layout, Redux, new persistent caches, and changing task lifecycle semantics.

## Acceptance

1. Homepage and sidebar selectors resolve one accepted overview object per task, with stable references for unchanged entities.
2. Truncated, failed, missing-workflow, old-server, and archived-ineligible data cannot mark a view complete.
3. Newer live updates and deletions survive late reads; page eviction and context disposal release unowned records within the stated budgets.

## TDD evidence

Add `task-overview.test.ts` and `task-overview-coverage.test.ts`.
First reproduce divergent homepage/sidebar records after one accepted update, then require shared identity and equivalent displayed fields.
Exercise boot hydration, HTTP settlement, create/update/move/archive/unarchive/delete, summary revisions, and optimistic rollback.
Cover explicit null versus missing projection fields and atomic placement/step metadata.
Do not let an older summary overwrite task fields or a task timestamp order unrelated summary revisions.

Add `TestWorkflowSnapshotTaskCoverage` to the existing handler tests.
Cover zero tasks, full active scope, explicit `task_limit`, hidden/missing workflows, and archive exclusion.
The completeness total must describe the same task collection before truncation.
Add `TestBootWorkflowSnapshotTaskCoverage` for homepage and task-detail boot projections.
Boot currently removes tasks without workflow steps; coverage must describe that actual collection and cannot imply omitted tasks are present.
Include boot metadata and older-server omission fixtures in frontend coverage tests.
A response spanning live membership changes remains incomplete unless those changes are reconciled.
A reconnect gap clears completeness until authoritative recovery succeeds.

Repeated archived page replacement must release unowned records rather than growing an all-task map.
Cover shared ownership between board, displayed page, reusable page, and active detail.
Assert that eviction cannot remove a record still owned by another active consumer.
Use bounded event-journal overflow tests to prove old requests cannot outlive their deletion protections.

## Verification

Run from repository root. Install workspace dependencies once if absent.

```bash
(cd apps/backend && go test -tags sqlite_fts5 -p 1 ./internal/task/handlers -run 'WorkflowSnapshot|WorkspaceSnapshot|TaskCoverage' -count=1)
(cd apps/backend && go test -tags sqlite_fts5 -p 1 ./internal/backendapp -run 'BootWorkflowSnapshotTaskCoverage|MapKanban' -count=1)
(cd apps/web && pnpm exec vitest run lib/state/slices/task-overview.test.ts lib/state/slices/task-overview-coverage.test.ts lib/state/slices/kanban/kanban-slice.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts lib/sidebar/sidebar-task-page-cache.test.ts lib/ws/handlers/tasks-created-snapshot.test.ts lib/ws/handlers/tasks-archive.test.ts lib/ws/handlers/tasks-unarchive.test.ts lib/ws/handlers/tasks-status-summary.test.ts)
(cd apps/web && pnpm run typecheck)
git diff --check
```

Also run every existing test suite changed while migrating additional overview writers; record each exact path and command in Results.
This is required migration coverage, not a broad verification audit.

## Files likely touched

- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/handlers/workflow_handlers.go`
- `apps/backend/internal/task/handlers/workflow_handlers_test.go`
- `apps/backend/internal/backendapp/boot_state.go` (`workflowSnapshotState` and homepage/task-detail projections)
- `apps/backend/internal/backendapp/boot_state_task_coverage_test.go` (new)
- `apps/web/lib/types/http.ts`
- `apps/web/lib/state/store.ts`
- `apps/web/lib/state/hydration/hydrator.ts`
- `apps/web/lib/state/slices/task-overview.ts` (new)
- `apps/web/lib/state/slices/task-overview.test.ts` (new)
- `apps/web/lib/state/slices/task-overview-coverage.ts` (new)
- `apps/web/lib/state/slices/task-overview-coverage.test.ts` (new)
- `apps/web/lib/state/slices/kanban/types.ts`
- `apps/web/lib/state/slices/kanban/kanban-slice.ts`
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`
- `apps/web/hooks/domains/kanban/use-workspace-sidebar-tasks.ts`
- `apps/web/lib/sidebar/sidebar-task-page-cache.ts`
- `apps/web/lib/ws/handlers/tasks.ts` and its split task handlers
- `apps/web/lib/ws/handlers/kanban.ts`
- `apps/web/AGENTS.md` (document the resulting overview slice and ownership)

## Dependencies

Task 01. The server path remains available while normalized consumers are introduced.

## Risks

An independent legacy write path can undo normalization. Inventory all overview producers and consumers before changing the slice.
A normalized map without owner-based eviction would recreate browser memory growth.
Do not invent a shared revision across different field owners.

## Parallelism

`sequential`

## Inputs

- [Shared-state design](../../specs/ui/system-design/sidebar-shared-task-state.md), records, coverage, reconciliation, retention.
- [Requirements](../../specs/ui/requirements/sidebar-task-pagination.md), .7–.8, .16, .20–.22, .24.
- Existing snapshot merge and WS removal/freshness tests.

## Results

The implementation routes all board writes through `withTaskOverviewNormalization` before publication. Canonical objects and ID memberships cover:

- Initial Go boot payload and frontend hydration (`boot_state.go`, `hydrator.ts`, `store.ts`).
- Single/all-workflow HTTP snapshots (`use-workflow-snapshot.ts`, `use-all-workflow-snapshots.ts`), with read journals and actual mapped membership counts.
- Phone workspace switching (`session-task-switcher-sheet-hooks.ts` and its helpers), including a journal spanning workflow metadata and snapshot reads.
- Live task create/update/move/archive/unarchive/delete and summary revisions (`lib/ws/handlers/tasks.ts`, its split handlers, and `task-status-summary.ts`).
- Existing board, Office, nesting, optimistic move/removal, and rollback writes through the same store transaction boundary; compatibility arrays retain canonical references.
- Bounded server-page membership (`sidebar-task-page-cache.ts`, `sidebar-page-overviews.ts`) with separate display, reusable-page, board, and active-detail ownership.

Coverage propagates through backend snapshot DTOs, homepage/task boot mapping, HTTP types, store hydration, and both workflow fetch paths. Workflow scope inventory includes hidden and unassigned active membership while excluding archived, ephemeral, automation-run, and config-mode records. Reconnect gaps invalidate both snapshot coverage and scope inventory; reads begun during the gap cannot survive reconnection. Workflow metadata refreshes with transport recovery.

Regression suites cover canonical identity, explicit clears, independent nanosecond task/summary freshness, optimistic rollback and atomic placement, live membership changes, scope isolation, access denial, bounded journals, and owner eviction. Additional mobile helper, workflow metadata, and SQLite/PostgreSQL scope cases are included. Tests execute only in CI at the user's request; the confirmed results are recorded below.

Before the final browser-fixture correction, frontend CI run `36707126982`
passed at `a0bf2671f` (attempt 1): all 2,428 test files passed, with 20,887 tests
passed and four skipped out of 20,891 discovered, followed by a successful build.
The backend snapshot/scope regressions also passed in run `36707127038`, including
both PostgreSQL variants. The final fixture correction does not change normalization
or any overview producer. No product tests, typechecks, or benchmarks ran locally.
