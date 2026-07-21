---
id: "07-system-storage-api"
title: "System storage API and composition"
status: done
wave: 3
depends_on:
  [
    "03-activity-gate-scheduler",
    "04-workspace-quarantine",
    "05-managed-go-cache",
    "06-docker-storage",
  ]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 07: System storage API and composition

Expose the complete Storage contract and replace the old Office GC wiring with the composed service.

## Acceptance

- Every spec endpoint returns the documented status/shape, validates Go-cache/Docker confirmation
  and busy states, and publishes/persists matching System jobs.
- Backend composition supplies all providers, activity coordinator, cleanup worker, health checks,
  and startup reconciliation with correct start/stop ownership.
- The unconditional `office/infra` sweep no longer runs; optional scheduling is available with
  Office disabled and waits one interval before its first run.

## Verification

```bash
cd apps/backend && go test ./internal/system/storage ./internal/system ./internal/backendapp ./internal/office/infra
```

## Files likely touched

- `apps/backend/internal/system/storage/handler.go`
- `apps/backend/internal/system/storage/handler_test.go`
- `apps/backend/internal/system/system.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/office/infra/gc.go` and tests (remove/migrate)
- health provider wiring and tests

## Dependencies

Tasks 03–06.

## Inputs

- Spec API surface, permissions, failures, and persistence guarantees
- Existing System route/service/job patterns
- Existing backend provider cleanup ownership conventions

## Output contract

Report API matrix, composition lifecycle, old-GC migration, health behavior, tests run,
blockers/risks, and update this task plus `plan.md` to done.
