---
id: "03-sidebar-cancellation"
title: "Classify canceled sidebar reads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CANCELLED-SIDEBAR-READS-001
acceptance_criteria:
  - AC-TASKS-CANCELLED-SIDEBAR-READS-001.1
  - AC-TASKS-CANCELLED-SIDEBAR-READS-001.2
  - AC-TASKS-CANCELLED-SIDEBAR-READS-001.3
  - AC-TASKS-CANCELLED-SIDEBAR-READS-001.4
system_design:
  - ../../specs/tasks/system-design/cancelled-sidebar-reads.md
---

# Task 03: Classify canceled sidebar reads

## Summary

Return the existing client-cancellation status for abandoned sidebar queries. Stop optional enrichment and preserve active-request failure handling.

## In scope

- HTTP query and enrichment cancellation boundaries.
- Shared task-list optional-read sequencing and request-context checks.
- Pending-count cancellation log classification with its original error chain.

## Out of scope

- Sidebar layout, queue logic, authorization, HTTP payload additions, and global error suppression.

## Acceptance

- Wrapped cancellation during query or enrichment returns 499 before response start. Cancellation generates no warning/error cascade or later optional query.
- Active-context database failures and deadlines retain their severity and response behavior.
- A canceled query does not mutate task state or cancel a subsequent successful query. Successful DTOs remain equivalent.

## Verification

Run this block from the repository root after the implementation result exists.
New test names below are required planned regressions, not claims of existing coverage.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/handlers -run 'Test.*(Sidebar|TaskListCancellation|TaskListActivityCancellation)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator/messagequeue -run 'TestCountPendingByTaskIDs' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/service -run 'Test(BuildDependencyViewsPreservesNonCancellationReadDiagnostics|BuildRunnerMutabilityViewsPreservesNonCancellationReadDiagnostics|ReconcileTaskStatusSummaries)' -count=1)
```

## Files likely touched

- `apps/backend/internal/task/handlers/sidebar_task_http.go`
- `apps/backend/internal/task/handlers/errors.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/sidebar_task_http_cancellation_test.go (new)`
- `apps/backend/internal/task/service/service_status_summary_rebuild.go`
- `apps/backend/internal/task/service/service_dependencies.go`
- `apps/backend/internal/task/service/service_dependencies_test.go`
- `apps/backend/internal/task/service/service_runner_switch.go`
- `apps/backend/internal/task/service/service_runner_mutability_views_test.go`
- `apps/backend/internal/orchestrator/messagequeue/service.go`
- `apps/backend/internal/orchestrator/messagequeue/count_pending_cancellation_test.go (new)`

## Dependencies

None. Follow the plan's sequential priority order.

## Risks

- Optional-read helpers can serve other routes. Preserve genuine failure fallback behavior for active callers.
- Response-start and nested-context cases need separate regressions.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), especially evidence, contract ownership, and completion rules.
- [Design](../../specs/tasks/system-design/cancelled-sidebar-reads.md) and its linked requirements.

- Scoped backend/agentctl instructions for any touched package.
- Existing source and tests listed above. Preserve completed companion-package results.

## Results

Implemented request-context-aware 499 handling at sidebar query, task-list enrichment, and nested status-summary reconciliation boundaries. Summary repair now checks cancellation between activity, PR, per-task environment, Git, queued-count, completion-gate, persistence, reload, and publish operations. Cancellation-only warnings are omitted only when the request context and returned error both identify cancellation; deadlines and unrelated repository failures retain their original diagnostics. Dependency and runner projections stop remaining batched reads under cancellation and use the same narrow warning rule. The pending-count service returns the repository error unchanged and omits its error entry only when both the supplied context and repository error chain identify `context.Canceled`.

Handler integration tests cover canceled query, pending-action enrichment, dependency projection, runner projection, nested cancellation under an active request, a deadline, and cancellation during a wired task-activity read after loading the status-summary repository. The latter verifies HTTP 499, no later launch-queue/environment/Git/queued-count read, no summary write, no cancellation warning/error, and a successful successor request. Service regressions verify wrapped request cancellation stays quiet while deadline failures and unrelated database errors that race with cancellation retain warning/error severity.

Passed:

- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/handlers -run 'Test.*(Sidebar|TaskListCancellation|TaskListActivityCancellation)' -count=1)`
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator/messagequeue -run 'TestCountPendingByTaskIDs' -count=1)`
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/service -run 'Test(BuildDependencyViewsPreservesNonCancellationReadDiagnostics|BuildRunnerMutabilityViewsPreservesNonCancellationReadDiagnostics|ReconcileTaskStatusSummaries)' -count=1)`
- Full affected-package tests passed: `go test -trimpath -tags fts5 ./internal/task/handlers ./internal/task/service ./internal/orchestrator/messagequeue ./internal/agent/runtime/agentctl/launcher -count=1`.
- `make -C apps/backend build` passed.
- Focused non-race handler, service, and messagequeue cancellation/projection regressions passed during implementation.

No route payload, task mutation, or public documentation changed.
