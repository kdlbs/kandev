---
id: "01-task-wide-running-rank"
title: "Deliver task-wide running rank"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-RUNNING-ACTIVITY-001
acceptance_criteria:
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.2
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.3
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.4
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.6
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.7
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.9
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.11
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.12
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.14
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.15
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.16
system_design:
  - ../../specs/ui/system-design/sidebar-running-first-activity-sort.md
---

# Task 01: Deliver task-wide running rank

## Summary

Deliver the runtime aggregate and every sort consumer as one functional slice.
Server and covered local evaluation must agree before paging and after live updates.
The repair recognizes secondary sessions without changing primary ownership.

## In scope

- Add optional `HasRunningSession *bool` to `TaskStatusSummary` and its semantic
  payload. Preserve explicit false in JSON, copies, DTOs, HTTP, boot, and WS delivery.
- Compute the aggregate in authoritative rebuilds and complete live session maps.
  Keep observation completeness independent of foreground activity availability.
- Recompute on creation, state transitions, and removal. Preserve true while
  another session remains running. Primary changes alone leave the value unchanged.
- Subscribe the existing projector to `events.SessionRemoved`, emitted by
  `DeleteSessionAndPublishRemoval` after commit. Remove observations idempotently.
- Cover restart plus unrelated events. Do not clear a stored flag from partial
  observations. Retain existing source-load error/retry behavior.
- Repair requested legacy summaries in `ReconcileTaskStatusSummaries` using
  complete session inputs, existing revision CAS, and winner reload on conflict.
  Projection repair alone does not advance semantic activity.
- Carry the running scalar through `sidebarStateFields` and ancestor aggregation.
  Keep primary state for Status. Legacy rows use scoped session `EXISTS` before
  paging. Support both SQLite and PostgreSQL through the bounded query path.
- Map optional `TaskSwitcherItem.hasRunningSession` through local, desktop, and
  phone projections. Preserve true, false, and absence distinctly.
- Require the new aggregate for covered local Running rules in any chain position.
  Missing aggregate evidence selects server evaluation.
- Retain generic legacy primary runtime fallback, tree filtering/collapse,
  cycles, pins, manual child order, color/activity precedence, and both directions.
- Extend conformance fixtures and accepted-revision/cache tests. Include a change
  outside the displayed page and stale HTTP after a newer summary event.

## Out of scope

Browser tests and public wording belong to Task 02. This work order does not
elect a primary, alter lifecycle, add controls, or change icon/Status precedence.
No startup scan, schema migration, feature flag, or per-session subscription.

## Acceptance

1. Mixed-session and no-primary tasks receive the correct aggregate through
   initial reads, rebuild, live transitions, removal, and restart.
2. SQL and local/generic ranking satisfy the fixture matrix before paging.
   Legacy rows rank correctly on first read. Incomplete local evidence uses the server.
3. Newer summary revisions update order without changing the selected conversation.
   Existing tree, chain, memory, and source-error constraints remain covered.

## TDD sequence

Start with proposed `TestSidebarTaskWideRunningRank/no_primary_secondary_precedes_idle_orange`
in `sidebar_task_query_running_test.go`. Seed durable session rows and legacy
summaries. The expected order fails under the current primary-only query.
Record this behavioral RED before the production correction.

Add projector/rebuild tests in proposed `statussummary/running_session_test.go`.
Add service repair tests in proposed `service_status_summary_running_test.go`.
Keep new tests separate from files near the backend file-size limit.

The matrix covers absent or waiting primary, failed/cancelled siblings, multiple
running sessions, no sessions, `STARTING` only, and `RUNNING` without activity.
It also covers false/missing/null aggregate and preserved primary Status behavior.
Add both directions, secondary Running, filtered/collapsed children, pins, manual
children, small-page boundaries, restart, source errors, and stale revisions.

## ASCII UI preview

UI-01 excerpt from [the complete preview](plan.md#ascii-ui-preview):

```text
Before                     After
[idle] Resume (orange)     [running] Projects
[running] Projects         [idle] Resume (orange)
```

UI-02 uses the same corrected row order inside the existing phone Tasks drawer.
Its fixed header, scroll body, and task navigation action remain.
AC .2, .6, .7, .14-.16 apply. Task 02 supplies rendered desktop and phone evidence.

## Verification

Run from the repository root. Install workspace dependencies once if absent.
The three proposed Go test files are implementation deliverables.

```bash
(cd apps/backend && go test -trimpath ./internal/task/statussummary -count=1)
(cd apps/backend && go test -trimpath ./internal/task/service -run 'RunningSession|ReconcileTaskStatusSummaries' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite ./internal/task/handlers -run 'Sidebar|QuerySidebar' -count=1)
(cd apps/web && pnpm exec vitest run lib/sidebar/task-tree-running.test.ts lib/sidebar/apply-view.test.ts lib/sidebar/sidebar-local-view.test.ts lib/sidebar/sidebar-local-conformance.test.ts lib/sidebar/sidebar-view-conformance.test.ts lib/sidebar/sidebar-task-source.test.ts lib/state/slices/task-overview-coverage.test.ts lib/task-status-summary.test.ts lib/types/task-status-summary.test.ts components/task/task-session-sidebar-item.test.ts components/task/mobile/session-task-switcher-sheet-hooks.test.ts hooks/domains/kanban/use-sidebar-task-page.test.tsx hooks/domains/kanban/use-sidebar-store-tasks.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 lib/types/task-status-summary.ts lib/sidebar/task-tree-running.ts lib/sidebar/sidebar-local-projection.ts lib/state/slices/task-overview-coverage.ts components/task/task-switcher-types.ts components/task/task-session-sidebar-item.ts components/task/mobile/session-task-switcher-sheet-item.ts)
git diff --check
```

The Sidebar selection includes existing native preparation and pooled-memory
tests on Linux. It does not introduce a full-suite gate.
Run the same repository command with `KANDEV_TEST_POSTGRES_DSN` set to an
isolated PostgreSQL database. Record any skip as missing dialect evidence.
Use the dual-dialect pattern in `sidebar_task_query_ancestors_test.go` for the
proposed query regression. Make sure that the PostgreSQL subtests run.
Add each extra changed source/test file to its corresponding targeted command.

## Files likely touched

- `apps/backend/internal/task/statussummary/model.go`
- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/projector_derive.go`
- `apps/backend/internal/task/statussummary/projector_events.go`
- `apps/backend/internal/task/statussummary/projector_helpers.go`
- `apps/backend/internal/task/statussummary/rebuild.go`
- Proposed `apps/backend/internal/task/statussummary/running_session_test.go`
- `apps/backend/internal/task/service/service_status_summary_rebuild.go`
- Proposed `apps/backend/internal/task/service/service_status_summary_running_test.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_base.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_page_sql.go`
- Proposed `apps/backend/internal/task/repository/sqlite/sidebar_task_query_running_test.go`
- `apps/backend/internal/task/repository/sqlite/sidebar_task_query_conformance_test.go`
- `apps/backend/internal/task/repository/testdata/sidebar-local-conformance.json`
- `apps/web/lib/types/task-status-summary.ts`
- `apps/web/components/task/task-switcher-types.ts`
- `apps/web/lib/sidebar/task-tree-running.ts` and its test
- `apps/web/lib/sidebar/sidebar-local-projection.ts`
- `apps/web/lib/state/slices/task-overview-coverage.ts` and its test
- `apps/web/components/task/task-session-sidebar-item.ts` and its test
- `apps/web/components/task/mobile/session-task-switcher-sheet-item.ts`
- `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.test.ts`
- Existing local/view/source tests listed in Verification
- `apps/web/lib/task-status-summary.test.ts`
- `apps/web/lib/types/task-status-summary.test.ts`
- `apps/web/hooks/domains/kanban/use-sidebar-task-page.test.tsx`
- `apps/web/hooks/domains/kanban/use-sidebar-store-tasks.test.tsx`

## Dependencies

None. Read the amended specification and decision before implementation.

## Risks

False from a partial observation can demote a running task.
Legacy first-page rank needs the query fallback before summary repair.
Combined running and activity sorts need a bounded SQLite memory check.
New state must survive complete summary replacement and stale-response rejection.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-running-first-activity-sort.md)
- [Task-wide projection design](../../specs/ui/system-design/sidebar-running-first-activity-sort.md#task-wide-running-projection)
- [Decision](../../decisions/2026-10-07-task-wide-running-sidebar-rank.md)
- `statussummary/projector_load_test.go` and `projector_restart_rehydration_test.go`
- `service/service_status_summary_rebuild_test.go`
- `repository/sqlite/sidebar_task_query_ancestors_test.go`
- Existing shared local/SQL fixture and harnesses

## Results

Completed. The first no-primary/secondary-running repository regression failed
under the primary-only predicate, with the idle preferred-color peer first.
After the task-wide projection and legacy fallback were implemented, targeted
SQLite/backend tests passed. The final focused frontend run passed 15 files and
314 tests; web typecheck and targeted ESLint passed. The repository/sidebar
Go checks passed both with SQLite alone and with PostgreSQL subtests enabled by
an isolated disposable PostgreSQL 16 container. PostgreSQL repository and
handler packages passed in 121.9s and 0.03s respectively.

Review remediation preserves the 64 MiB SQLite native-memory limit. The
combined running/activity query now reads the bounded summary boolean and uses
an indexed task-scoped `EXISTS` only for legacy summaries without a valid
boolean. The measured peak was 64,991,352 bytes for both the empty and
101-task fixtures, with no retained growth. Handler enrichment retains the
initial session and pending-action reads before the optional summary read to
preserve cancellation behavior, then uses a fresh batched observation after
loading the summary revision for repair. Deterministic start and stop
interleavings verify that reconciliation retains the newer running flag. The
summary-repair regression also verifies that filling the missing flag advances
the revision without replacing newer activity. Browser and production-build
evidence is in Task 02.
