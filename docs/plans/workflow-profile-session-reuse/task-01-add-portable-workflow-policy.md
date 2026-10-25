---
id: "01-add-portable-workflow-policy"
title: "Split the step lifecycle contract"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.7
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.12
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 01: Split the Step Lifecycle Contract

## Summary

Replace the unshipped combined policy with independent start and end settings on
each workflow step.

## In scope

- Add normalized start and end enums to step models and templates.
- Replace the step database column and repository mappings.
- Update step create, update, read, list, boot, WebSocket, MCP, and adapter
  contracts.
- Update portable export, import, duplication, templates, and sync.
- Remove the old enum, field, mappings, and tests.

## Acceptance

- Missing values normalize to `reuse` and `complete`.
- Both fields round-trip independently on each step.
- No combined policy remains.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/workflow/... ./internal/workflowsync ./internal/backendapp -run 'ProfileSession|WorkflowStep' -count=1
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/workflow/models/models.go`
- `apps/backend/internal/workflow/models/export.go`
- `apps/backend/internal/workflow/repository/sqlite.go`
- `apps/backend/internal/workflow/controller/controller.go`
- `apps/backend/internal/workflow/service/`
- `apps/backend/internal/workflowsync/`
- `apps/web/lib/types/http.ts`
- `apps/web/lib/types/backend.ts`

## Dependencies

None.

## Risks

A stale mapping can preserve the old combined field or reset one new setting.

## Parallelism

`sequential`

## Results

Pending.
