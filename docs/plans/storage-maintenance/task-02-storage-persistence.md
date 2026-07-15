---
id: "02-storage-persistence"
title: "Storage persistence contracts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 02: Storage persistence contracts

Create normalized install settings, persistent maintenance-run history, and quarantine storage.

## Acceptance

- Missing settings produce the exact disabled defaults; invalid ranges or unsafe Docker combinations
  fail without overwriting saved values.
- Maintenance runs and quarantine entries implement every spec state transition and survive store
  recreation on SQLite and Postgres-supported paths.
- Retention keeps the newest 20 terminal runs plus every non-terminal run.

## Verification

```bash
cd apps/backend && go test ./internal/system/settings ./internal/system/storage
```

## Files likely touched

- `apps/backend/internal/system/storage/types.go`
- `apps/backend/internal/system/storage/settings.go`
- `apps/backend/internal/system/storage/store.go`
- `apps/backend/internal/system/storage/store_migrations.go`
- `apps/backend/internal/system/storage/settings_test.go`
- `apps/backend/internal/system/storage/store_test.go`

## Dependencies

None.

## Inputs

- Spec Data model and State machine sections
- `apps/backend/internal/system/settings/store.go`
- `apps/backend/internal/system/metrics/store.go`
- ADR 0027 replayable schema migrations

## Output contract

Report persisted shapes, migration/replay coverage, validation behavior, tests run, blockers/risks,
and update this task plus `plan.md` to done.
