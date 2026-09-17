---
id: "01-task-observations"
title: "Scoped task observations"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-001
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-002
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-004
  - REQ-ORCHESTRATION-COORDINATOR-VIEW-006
acceptance_criteria:
  - AC-ORCHESTRATION-COORDINATOR-VIEW-001.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-001.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-001.3
  - AC-ORCHESTRATION-COORDINATOR-VIEW-001.4
  - AC-ORCHESTRATION-COORDINATOR-VIEW-002.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-002.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-002.3
  - AC-ORCHESTRATION-COORDINATOR-VIEW-004.1
  - AC-ORCHESTRATION-COORDINATOR-VIEW-004.2
  - AC-ORCHESTRATION-COORDINATOR-VIEW-004.3
  - AC-ORCHESTRATION-COORDINATOR-VIEW-006.1
system_design:
  - ../../specs/orchestration/system-design/coordinator-view.md
---

# Task 01: Scoped task observations

## Summary

Provide canonical workspace task observations for the Coordinator page, including
deterministic groups, coverage-aware paging and fresh status. Preserve the
backend's scope and hidden-task boundaries.

## In scope

- Extend the typed task-list client with existing `exclude_config` support;
  verify canonical hidden/private task exclusion and foreign-workspace rejection.
- Add `use-coordinator-tasks.ts` and `coordinator-task-groups.ts` with tests.
  Read task DTOs/status summaries; expose server filters, local coordinated/group
  filters, loaded coverage and additional pages without new durable storage.
- Integrate workspace generations, aborts, canonical cache/WS invalidation,
  monotonic summary merging and reconnect reconciliation.

## Out of scope

Page layout/chat, task mutations, automatic adoption, request resolution, new
attention persistence, typed reports and new scheduling or plugin contracts.

## Acceptance

- Unlinked tasks and records beyond page one remain available, while hidden
  conversation/configuration tasks and unauthorized workspace data never appear.
- Groups follow the design precedence; unknown, partial and stale data cannot
  produce an invented question, stall, completed task or workspace-wide total.
- Status races, workspace switches, reconnect and read failures converge without
  dispatching work or exposing the previous workspace's content.

## Verification

Run from repository root, with the repository's Go/Node/pnpm toolchain on PATH:

```bash
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web exec vitest run hooks/domains/orchestration/use-coordinator-tasks.test.ts lib/orchestration/coordinator-task-groups.test.ts lib/task-status-summary.test.ts lib/api/domains/kanban-api.test.ts
(cd apps/backend && go test -tags fts5 -count=1 ./internal/task/handlers ./internal/task/service ./internal/task/statussummary)
pnpm --dir apps/web run typecheck
git diff --check
```

## Files likely touched

- `apps/web/lib/api/domains/kanban-api.ts` and its tests.
- `apps/web/hooks/domains/orchestration/use-coordinator-tasks.ts` and its tests (new).
- `apps/web/lib/orchestration/coordinator-task-groups.ts` and its tests (new).
- `apps/web/lib/ws/handlers/task-status-summary.ts` and task lifecycle handlers,
  or the existing cache subscription/invalidation seam selected during implementation.
- `apps/backend/internal/task/handlers/task_http_lifecycle_test.go` or
  `coordinator_listing_test.go` (new); canonical task-list filtering only if the
  exclusion tests identify a gap.

## Dependencies

None beyond the audited v0.94.0 coordinator baseline.

## Risks

Per-task timeline reads would create unnecessary load and expose excess content.
Offset pages may shift during concurrent updates; dedupe and refresh loaded
windows without claiming a transactional snapshot. Do not bypass native task
authorization to calculate counts.

## Parallelism

`sequential`

## Inputs

- System design: Task data and paging; Group projection; Freshness and recovery;
  Authority and privacy.
- `listTasksByWorkspace`, `TaskStatusSummary`, `pickFreshestStatusSummary` and
  `use-sidebar-archived-tasks.ts` for existing paged loading/workspace guards.

## Results

Pending. No production or permanent test changes in the design turn.
