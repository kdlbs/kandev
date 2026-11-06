---
id: "01-startup-reader-override"
title: "Startup SQLite reader pool override"
status: in_progress
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLATFORM-SQLITE-READER-POOL-001
acceptance_criteria:
  - AC-PLATFORM-SQLITE-READER-POOL-001.1
  - AC-PLATFORM-SQLITE-READER-POOL-001.2
  - AC-PLATFORM-SQLITE-READER-POOL-001.3
  - AC-PLATFORM-SQLITE-READER-POOL-001.4
  - AC-PLATFORM-SQLITE-READER-POOL-001.5
system_design:
  - ../../specs/platform/system-design/sqlite-reader-pool.md
---

# Startup SQLite reader pool override

## Scope

Add a startup-only SQLite reader pool size to the typed configuration catalog,
apply it to both maximum open and idle reader connections, and document it. Keep
the shipped default at four.

## Exclusions

No probe timeout, probe hysteresis, reader-contention classification, runtime
setting, PostgreSQL sizing, schema, or operational change. Do not adopt the
fork's default of twelve.

## Acceptance

- Default and override sizing both use the configured maximum open and idle
  counts.
- Invalid and out-of-range values fail startup from YAML or the environment.
- Writer sizing, transaction admission, and required-store fail-closed behavior
  are unchanged.

## Files touched

`apps/backend/internal/db/sqlite.go`,
`apps/backend/internal/persistence/provider.go`,
`apps/backend/internal/common/config/{config,catalog,source,validation}.go`,
their existing tests, the linked specification and plan documents, and
`docs/public/configuration.md`.

## Verification

Use the commands in [plan](plan.md#verification).

## Dependencies

None.

## Parallelism

Sequential. Single writer.

## Risks

See [plan](plan.md#risks).

## Results

Implemented `database.sqliteReaderConns` (`KANDEV_SQLITE_READER_CONNS`) with a
default of four, strict range one through 64, and environment-over-YAML
precedence. Added `OpenSQLiteReaderWithConns`, wired persistence to the typed
value, and documented the setting. Strict parsing and range validation apply
only when SQLite is selected. After driver selection, a present environment
value is parsed first and a lower-priority YAML value is not parsed; an invalid
environment value fails without falling back to YAML. When the environment
value is absent, the raw YAML value is parsed strictly. An explicitly present
empty environment value is invalid, not absent. An invalid or nonnumeric
SQLite-only value therefore cannot reject a PostgreSQL startup.

The upstream review follow-up added the required permanent coverage in existing
files: configuration resolution, provenance, and file-aware YAML errors in
`source_test.go`; reader factory sizing in `sqlite_test.go`; and persistence
wiring in `provider_legacy_test.go`. Existing package tests continue to cover
immediate fail-closed behavior. The catalog audited-inventory line remains the
minimum consistency update for the existing catalog contract.
