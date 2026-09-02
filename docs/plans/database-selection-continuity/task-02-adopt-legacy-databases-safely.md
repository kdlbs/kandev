---
id: "02-adopt-legacy-databases-safely"
title: "Adopt legacy databases safely"
status: done
wave: 2
depends_on:
  - 01-preserve-database-path-provenance
plan: "plan.md"
requirements:
  - REQ-PLATFORM-STARTUP-CONFIGURATION-PARITY-002
acceptance_criteria:
  - AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-002.1
  - AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-002.2
  - AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-002.3
  - AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-002.4
system_design:
  - ../../specs/platform/system-design/startup-database-selection-continuity.md
---

# Task 02: Adopt Legacy Databases Safely

## Summary

Resolve default SQLite candidates before writable open, adopt a sole legacy database through a validated staged snapshot, and fail without mutation on an empty-history conflict. Prove the complete launcher-to-persistence regression and document recovery behavior.

## In scope

- Add source-aware default SQLite target selection and read-only candidate inspection.
- Replace legacy rename with staged snapshot, validation, atomic installation, and source retention.
- Detect the current-zero-task versus legacy-task-history conflict before repository startup.
- Add selection diagnostics, integration tests, failure-path tests, and public recovery guidance.

## Out of scope

- Switching or merging when both databases contain task history.
- Arbitrary custom-path discovery.
- PostgreSQL, backup retention, restore, UI, and self-update changes.

## Acceptance

- A default launch with only a WAL-backed legacy database exposes its task data from the current target and leaves the legacy files intact.
- A current default with no tasks and a legacy default with tasks causes a non-destructive startup error naming both candidates.
- Explicit paths bypass legacy discovery, and every staged-adoption failure leaves the source and prior target state unchanged.

## Verification

```bash
cd apps/backend && go test ./internal/persistence ./internal/launcher -run '^(Test.*(Legacy|DatabaseContinuity|DatabaseConflict|DatabasePath))' -count=1
cd apps/backend && go test ./internal/persistence ./internal/launcher -count=1
cd apps/backend && go build ./cmd/kandev
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/persistence/provider.go`
- `apps/backend/internal/persistence/sqlite_selection.go`
- `apps/backend/internal/persistence/provider_legacy_test.go`
- `apps/backend/internal/launcher/database_handoff_integration_test.go`
- `docs/public/configuration.md`
- `docs/public/operations.md`

## Dependencies

- Task 01 must preserve default source provenance across child startup.

## Risks

- Snapshot validation and atomic installation must work across macOS, Linux, and Windows filesystem semantics.
- Candidate inspection must tolerate historical schemas that have `tasks` but no `kandev_meta` table.
- Tests must prove committed WAL data is present in the adopted snapshot.

## Parallelism

`sequential`

## Inputs

- `REQ-PLATFORM-STARTUP-CONFIGURATION-PARITY-002`
- `docs/specs/platform/system-design/startup-database-selection-continuity.md`
- ADR `2026-08-27-preserve-legacy-sqlite-before-default-initialization`
- Existing `persistence.SnapshotSQLite` and backup/restore validation patterns

## Results

- Added source-aware SQLite selection before writable database open.
- Replaced destructive legacy rename with a WAL-safe `VACUUM INTO` snapshot,
  integrity and task-count validation, owner-only staged permissions, atomic
  installation, and source plus sidecar retention.
- Added launcher-to-persistence, WAL adoption, invalid-candidate,
  empty-current conflict, explicit-path, established-current, and failed
  installation coverage.
- Hardened installation with an atomic no-replace target operation and made
  established-current inspection use a lightweight task-history probe.
- Updated Docker and Kubernetes upgrade guidance and marked the implemented
  system design as current.
- Updated the public configuration and operations guides with continuity and
  recovery behavior.
- Verification passed:
  - `go test ./internal/persistence ./internal/launcher -run '^(Test.*(Legacy|DatabaseContinuity|DatabaseConflict|DatabasePath))' -count=1`
  - sanitized full package tests with `KANDEV_INTERNAL_CONFIG_FILE`,
    `KANDEV_DATABASE_PATH`, and `KANDEV_DATABASE_DRIVER` unset
  - targeted race tests for the persistence and launcher packages
  - `go build ./cmd/kandev`
  - `make -C apps/backend lint`
  - `node --test scripts/validate-public-docs.test.mjs`
  - `node scripts/validate-public-docs.mjs`
