---
id: "01-portable-schema-helpers"
title: "Add portable schema helpers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.2
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 01: Add Portable Schema Helpers

## Summary

Add shared helpers for portable timestamp types and schema introspection. These helpers remove provider-owned database detection.

## In scope

- Add the timestamp-type helper in `internal/db/dialect`.
- Add table and column introspection helpers in `internal/db`.
- Cover SQLite and PostgreSQL behavior.

## Out of scope

- Change a provider store.
- Add a migration framework.

## Acceptance

- The timestamp helper returns `DATETIME` for SQLite and `TIMESTAMPTZ` for PostgreSQL.
- Table and column probes use the active database catalog.
- The helpers report absent objects without an error.

## Verification

```bash
KANDEV_TEST_POSTGRES_DSN=<dsn> go test -race ./internal/db/... -run 'Test.*(TimestampType|TableExists|TableColumns)' -v
```

## Files likely touched

- `apps/backend/internal/db/dialect/types.go`
- `apps/backend/internal/db/dialect/dialect_test.go`
- `apps/backend/internal/db/schema.go`
- `apps/backend/internal/db/schema_test.go`
- `apps/backend/internal/db/schema_postgres_test.go`

## Dependencies

None.

## Risks

- PostgreSQL probes must use `current_schema()` to preserve isolated test schemas.

## Parallelism

`sequential`

## Inputs

- `docs/decisions/0027-replayable-schema-migrations.md`
- `apps/backend/internal/delivery/store.go`
- `apps/backend/internal/task/repository/sqlite/worktree_ownership_targets.go`

## Results

- Added `dialect.TimestampType` for SQLite `DATETIME` and PostgreSQL
  `TIMESTAMPTZ` schemas.
- Added shared `db.TableExists`, `db.TableColumns`, and `db.ColumnExists`
  probes using the active database catalog and current PostgreSQL schema.
- Added SQLite and environment-gated PostgreSQL coverage for present and
  absent tables and columns.
- Verification: `KANDEV_TEST_POSTGRES_DSN=<local test DSN> go test -race
  ./internal/db/... -run 'Test.*(TimestampType|TableExists|TableColumns)' -v`
  passed.
