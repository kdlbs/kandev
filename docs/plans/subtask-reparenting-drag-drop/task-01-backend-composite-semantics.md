---
id: "01-backend-composite-semantics"
title: "Backend composite re-parent semantics"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/subtask-reparenting-drag-drop.md"
---

# Task 01: Backend composite re-parent semantics

## Acceptance

- `Service.UpdateTask` normalizes `metadata.workspace.mode` from `inherit_parent` to `shared_group` whenever the effective parent changes (set or cleared); other modes and root tasks are untouched. The returned task, the persisted row, and the `task.updated` payload all reflect the change.
- `DashboardService.UpdateTaskParentID` (Office dashboard PATCH) applies the same normalization on its non-empty parent path and includes `metadata` in the published `fields` when the mode changed.
- All existing reparent/detach service and handler tests pass unchanged.

## Verification

```bash
cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/office/dashboard
```

## Files likely touched

- `apps/backend/internal/task/service/service_tasks.go` (parent block in `Service.UpdateTask`, new `normalizeWorkspaceModeAfterReparent` helper)
- `apps/backend/internal/task/service/service_reparent_test.go` (new cases)
- `apps/backend/internal/office/dashboard/service_tasks.go` (`UpdateTaskParentID`)
- `apps/backend/internal/office/dashboard/service_detachment_test.go` (parity test)
- `apps/backend/internal/office/repository/sqlite/tasks.go` (new `UpdateTaskWorkspaceMode`)
- `apps/backend/internal/office/dashboard/service.go` (repo interface member, if `UpdateTaskWorkspaceMode` is not already surfaced)

## Dependencies

None.

## Inputs

- Spec sections: What (composite semantics), Data model, API surface, Failure modes.
- Existing pattern: `Service.DetachTask` + `detachTaskQuery` (dialect-aware JSON mode normalization in `apps/backend/internal/task/repository/sqlite/task.go`), `resolveParentID` / `validateReparentDepth` in `service_tasks.go`, `publishTaskUpdated` in the office dashboard service.
- Do NOT change `resolveParentID` / `validateReparentDepth`; validation behavior is already shipped and tested.

## Output contract

Report the service/repo changes, exact commands and results, files changed, blockers, residual risks; update this task and `plan.md` when acceptance passes.

## Results

- `cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/office/dashboard` — service and office/dashboard `ok`; handlers `ok` (run with `umask 022`; this sandbox's default umask 002 makes 3 pre-existing local-repository handler tests fail — they pass under standard umask and fail identically on the stash-clean base).
- Added `normalizeWorkspaceModeAfterReparent` to `Service.UpdateTask` (parent-change block): `inherit_parent` → `shared_group` on any effective parent change. TDD red→green: `TestService_UpdateTask_ReparentNormalizesInheritedWorkspaceMode`, `TestService_UpdateTask_UnnestNormalizesInheritedWorkspaceMode`, `TestService_UpdateTask_ReparentPreservesNonInheritedWorkspaceModes`.
- Office parity: repo `UpdateTaskParentID` now sets `parent_id` AND normalizes mode in one dialect-aware UPDATE (SQLite JSON, mirroring `detachTaskQuery`); dashboard service publishes `["parent_id", "metadata"]`; parity test `TestUpdateTaskParentIDNormalizesInheritedWorkspaceMode`; added the missing `metadata` column to the office test-harness tasks DDL (matches production schema).
- Files changed: `internal/task/service/service_tasks.go`, `service_reparent_test.go`, `internal/office/dashboard/service_tasks.go`, `service_detachment_test.go`, `handler_test.go`, `internal/office/repository/sqlite/tasks.go`.
