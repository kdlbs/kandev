---
id: integrated-control-office-aggregate-task-01
title: Office workspace aggregate API
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-OFFICE-WORKSPACE-OVERVIEW-001
acceptance_criteria:
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.1
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.2
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.3
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.4
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.5
  - AC-OFFICE-WORKSPACE-OVERVIEW-001.6
system_design:
  - ../../specs/office/system-design/workspace-overview.md
legacy_specs: []
---

# Task 01: Office workspace aggregate API

## Scope

- `GET /api/v1/office/workspaces/aggregate` in `office/dashboard/aggregate.go`, registered beside the per-workspace dashboard routes.
- Batched cross-workspace reads in `office/repository/sqlite/aggregate.go`; shared task-state bucketing with `BucketTaskBreakdown`.
- The owner-aggregate route table and guard in `backendapp/office_scope.go`, ahead of the auth-disabled bypass, and its completeness-test classification.
- Workspace lister wiring in `backendapp/main.go`.
- `getWorkspaceAggregate` and wire types in `apps/web/lib/api/domains/office-extended-api.ts`.
- Run links in the shared Office `ActivityRow` keep the activity's `workspaceId`.

Out of scope: the standalone `/office/overview` page, its Office store slice, its navigation entry, and its browser tests.

## Acceptance

- Agent tokens are refused in every auth mode; enforced auth requires a real, non-synthetic identity.
- Only the caller's Office workspaces are returned, sorted by name, with zero counts for empty workspaces.
- At most 20 activity entries, newest first, labels enriched.
- 503 without a lister; 500 on any read failure; batches stay under the SQLite parameter limit.

## Verification

- `cd apps/backend && go test ./internal/office/dashboard ./internal/office/repository/sqlite`
- `cd apps/backend && go test ./internal/backendapp -run 'TestOfficeScope|TestOfficeRoute|TestOfficeWorkspaceless'`
- `cd apps/web && pnpm test -- app/office/workspace/activity/activity-row.test.tsx`

## Current-main reconciliation (2026-10-31)

The normal merge preserves the published aggregate and recorded-control source
and current main. The one service-struct conflict keeps both dependencies:
`workspaceLister` supplies identity-scoped aggregate reads; `taskStateWriter`
keeps main's task-owned transactional writes and completion guards. Both setters
and composition-root bindings remain. No read, mutation or authorization policy
is replaced, and no schema, API, flag or additional control feature is introduced.
Existing aggregate and scope tests verify this composition; current results and
normal hooks are recorded at handoff separately from the historical checks.
