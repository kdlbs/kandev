---
id: "02-attachment-service"
title: "Attachment service"
status: done
wave: 2
depends_on: ["01-durable-source-contracts"]
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 02: Attachment Service

## Acceptance

- `AttachWorkspaceSources` validates and resolves complete mixed batches under a per-task lock before
  persistence, including task/workspace ownership, idle state, locator/path/branch validity,
  duplicates, and runtime-name collisions.
- Materialization success publishes one truthful task update; failure or caller cancellation removes
  only records/repository entities created by the operation and returns a typed error.
- Existing `AddBranchToTask` behavior and tests pass through a one-item compatibility adapter.

## Verification

```bash
cd apps/backend
rtk go test ./internal/task/service/... -run 'Test(AttachWorkspaceSources|AddBranchToTask)'
```

## Files likely touched

- `apps/backend/internal/task/service/service.go`
- `apps/backend/internal/task/service/service_workspace_sources.go` (new)
- `apps/backend/internal/task/service/service_workspace_sources_test.go` (new)
- `apps/backend/internal/task/service/service_branches.go`
- `apps/backend/internal/task/service/service_branches_test.go`
- `apps/backend/internal/task/service/service_events.go`

## Dependencies

Task 01.

## Inputs

- Spec: What, Failure modes, Permissions.
- Existing repository resolution in `service_tasks.go` and `repository_discovery.go`.
- Existing add-branch materialization/rollback behavior in `service_branches.go`.

## Output contract

Summary, files changed, tests run, blockers, risks, divergence, and task/plan status updates.
