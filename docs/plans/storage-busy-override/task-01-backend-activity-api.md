---
id: "01-backend-activity-api"
title: "Backend activity and API contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 01: Backend Activity and API Contract

## Acceptance

- A blocked manual Storage run returns stable busy kinds with user-facing labels and declares
  whether a force request is available.
- `force: true` starts a cleanup beside current task activity, but never beside another maintenance
  run; a later task acquisition still cancels the forced maintenance lease.
- Existing scheduled and non-forced manual admission behavior, run persistence, and 202 job shape
  remain unchanged.

## Verification

```bash
cd apps/backend && go test ./internal/agent/runtime/activity ./internal/system/storage ./internal/system
```

## Files likely touched

- `apps/backend/internal/agent/runtime/activity/coordinator.go`
- `apps/backend/internal/agent/runtime/activity/coordinator_test.go`
- `apps/backend/internal/system/storage/runner.go`
- `apps/backend/internal/system/storage/runner_test.go`
- `apps/backend/internal/system/storage/operations.go`
- `apps/backend/internal/system/storage/operations_test.go`
- `apps/backend/internal/system/storage/handler.go`
- `apps/backend/internal/system/storage/handler_test.go`
- `apps/backend/internal/system/storage_routes_test.go`

## Dependencies

None.

## Inputs

- Spec: Manual Run now, API surface, and busy-override scenarios
- Plan: Backend and Tests
- Existing activity lease behavior in `internal/agent/runtime/activity/coordinator.go`

## Output contract

Report acceptance status, changed request/response shapes, focused test results, risks, and update
this task plus `plan.md` to `done` in the same conversation.

## Validation Results

- `go test ./internal/agent/runtime/activity ./internal/system/storage ./internal/system -count=1 -race`: passed.
- `go test ./internal/backendapp ./internal/system/storage ./internal/system -count=1`: passed.
