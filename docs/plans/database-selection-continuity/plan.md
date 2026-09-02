---
created: 2026-08-27
status: implemented
requirements:
  - REQ-PLATFORM-STARTUP-CONFIGURATION-PARITY-002
system_design:
  - ../../specs/platform/system-design/startup-database-selection-continuity.md
legacy_specs: []
---

# Implementation Plan: Database Selection Continuity

## Overview

Preserve default SQLite database continuity across the launcher-to-backend boundary, then replace destructive legacy migration with validated staged adoption and conflict detection. Provenance must be corrected first because persistence cannot distinguish an operator override from a launcher-derived default while the launcher synthesizes `KANDEV_DATABASE_PATH`.

## Scope

### In scope

- Preserve the typed source of the default SQLite path across managed child startup.
- Adopt a sole legacy default database without deleting the source.
- Stop startup when an empty-history current default conflicts with legacy task history.
- Preserve explicit environment, YAML, and dev-isolated database selection.
- Add bounded startup diagnostics and public recovery guidance.

### Out of scope

- PostgreSQL changes.
- Automatic switching or merging when both databases contain task history.
- A new storage-doctor command or recovery UI.
- Self-update snapshots, backup retention changes, or restore redesign.
- Scanning arbitrary filesystem locations for custom databases.

## Technical approach

### Launcher provenance

Update `internal/launcher.backendEnvForConfig` so a default-derived database path is not emitted as `KANDEV_DATABASE_PATH`. Keep ambient explicit environment values, typed YAML handoff, and dev-mode managed overrides. Add focused source-preservation tests and retain startup display of the resolved path.

### Persistence selection and adoption

Extract default SQLite selection from `provideSQLite` into a pre-open helper that uses `Config.SourceFor("database.path")`. Inspect only the current and known legacy default candidates. Adopt a sole legacy candidate through `SnapshotSQLite` into a unique staged file, validate it, atomically install it, and retain the source and sidecars. Fail before repository initialization on inspection, validation, installation, or empty-history conflict errors.

Add a cross-boundary regression test that constructs the launcher's child environment, reloads typed child configuration, invokes persistence, and proves a seeded legacy task remains visible. Keep lower-level persistence tests for explicit selection, WAL-backed adoption, source retention, conflict immutability, and same-database replay.

### Diagnostics and documentation

Log database source and selection outcome without content. Update the public configuration reference and operations recovery guide to describe default legacy adoption, conflict errors, and the rule that explicit paths bypass discovery.

## Tests

- `AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-002.1`: launcher-to-persistence regression plus WAL-backed legacy adoption test.
- `AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-002.2`: two-candidate conflict test that proves both files remain byte-identical.
- `AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-002.3`: environment, YAML, and dev-managed explicit-path tests.
- `AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-002.4`: staged-adoption success and injected-failure tests that prove source retention.

## Work orders

- [x] [Task 01: Preserve database path provenance](task-01-preserve-database-path-provenance.md)
- [x] [Task 02: Adopt legacy databases safely](task-02-adopt-legacy-databases-safely.md)

## Verification results

Task 01 and Task 02 complete. Launcher provenance, staged WAL-safe adoption,
conflict handling, explicit-path isolation, diagnostics, tests, and public
recovery guidance are implemented and verified. Review follow-up hardening
also covers no-replace installation, lightweight warm-start inspection, and
deployment documentation.

## Risks

- Existing service environments can carry an intentional `KANDEV_DATABASE_PATH`; tests must distinguish inherited operator values from launcher-generated defaults.
- SQLite WAL contents must be included without moving or deleting sidecars.
- A deliberately reset current database can conflict with retained legacy task history and require explicit operator cleanup.
- Candidate inspection must not create files or read user content beyond bounded row counts.
- Adoption needs free disk space for the staged copy and must leave the source recoverable on every error path.
