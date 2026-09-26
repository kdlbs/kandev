---
id: "03-system-api-wiring"
title: "System API wiring"
status: done
wave: 3
depends_on: ["01-lifecycle-service", "02-native-platform-inhibitors"]
plan: "plan.md"
spec: "../../specs/platform/requirements/task-sleep-inhibition.md"
---

# Task 03: System API wiring

## Acceptance

- GET returns persisted settings plus reconciled platform status; PATCH validates a boolean, requires admin, persists, and returns post-reconcile status.
- System composition starts sleep inhibition after orchestrator recovery, passes the authoritative task-session repository, and releases/joins it during cleanup.
- Route, startup-default, malformed-body, and read-versus-admin permission tests pass without changing existing System endpoints.

## Verification

```bash
cd apps/backend && go test ./internal/system/sleepinhibition ./internal/system ./internal/backendapp -run 'Test.*(SleepInhibition|SystemRoutes)'
```

## Files likely touched

- `apps/backend/internal/system/sleepinhibition/handler.go`
- `apps/backend/internal/system/sleepinhibition/handler_test.go`
- `apps/backend/internal/system/system.go`
- `apps/backend/internal/system/system_routes_test.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/system_sleep_inhibition_test.go`

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential; this task consumes both the core service and platform factory and changes shared application composition.

## Inputs

- Spec sections: API surface, Permissions, Persistence guarantees.
- Plan section: System settings API and application wiring.
- Existing patterns: `internal/system/queuesettings/`, `internal/system/system.go`, and `backendapp/main.go` System-pages construction.

## Risks

- The System service is constructed after orchestrator startup; preserve that ordering so stale recovered session states are reconciled before the first lease decision.
- Admin middleware belongs only on PATCH; authenticated members must retain read access to status.

## Output contract

Report route shapes, startup/shutdown wiring, files changed, exact tests, blockers/risks, and synchronized task/plan status.

## Results

- Added authenticated GET and admin-only PATCH routes, strict boolean request
  validation, shared-store composition, startup reconciliation, and shutdown
  lease release.
- Focused route/system checks passed:
  `cd apps/backend && go test ./internal/system/... ./internal/backendapp -run 'Test.*(SleepInhibition|SystemRoutes)' -count=1`.
