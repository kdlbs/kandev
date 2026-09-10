---
created: 2026-09-10
status: completed
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-002
system_design:
  - ../../specs/system-page/system-design/storage-database-footprint.md
legacy_specs: []
---

# Implementation Plan: Database storage visibility

## Overview

Show database and backup space in Storage analysis and include both in the counted total.
Implement the measurement contract first, then its resource rows, then browser evidence.
The system-page system owns this package because it owns the existing storage analysis contract.

[Requirements](../../specs/system-page/requirements/storage-maintenance.md) extend the
existing capability with draft requirement `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-002`.
[System design](../../specs/system-page/system-design/storage-database-footprint.md) defines the measurement and presentation contract.

## Scope

### In scope

- Local SQLite database file lengths, including current journal sidecars.
- The configured database's sibling backup directory, including manual snapshots.
- Two independent progressive sources, explicit availability, and exactly-once total contribution.
- Localized desktop/mobile rows and visible explanation of analysis scope.

### Out of scope

- Database or backup deletion, retention changes, VACUUM, and checkpointing.
- Remote PostgreSQL size or backup discovery.
- Logs, repository caches, attachments, and complete host-volume accounting.
- Changing the existing binary-byte conversion labeled GB.

## Technical approach

### Measurement and composition

Introduce `internal/system/storage/databasestore` with injected driver, path,
shared scanner, and testable filesystem boundaries. Follow the configured database
location used by `internal/system/system.go`; do not use a hardcoded home path.
Extend `storageOverview.summary` and `summaryFromMeasurements` in
`backendapp/storage_maintenance.go`.

### Overview contract

Extend `storage.Summary`, source identities, legacy progress mapping, and
`summaryFromSourceValues`. Distinguish measured, unavailable, and not-applicable
results. Missing compatibility fields remain unknown. Include each new source in
existing completion telemetry, cache snapshots, and progressive-response fixtures.
No schema migration is required.

### Web presentation

Update `lib/types/system.ts`, resource construction, and total derivation.
Keep the existing Storage accordion and document scroll. Paths wrap on phones;
row taps expose all details. The nearest exemplar is the existing mobile Storage
analysis flow. Both viewports share one view model and snapshot.
Add localized scope explanations and update public operations documentation.

## Tests

The following test names are proposed implementation outputs.

| Criteria | Evidence |
| --- | --- |
| .1, .2 | `databasestore/provider_test.go: TestAnalyzeSQLiteFootprint`, `TestAnalyzeCustomPath`, `TestAnalyzeBackups` |
| .3 | `TestAnalyzeOverlappingRoots`; `storage-totals.test.ts` exactly-once contribution cases |
| .4 | `TestAnalyzeMissingAndUnreadableFiles`; resource unavailable rendering |
| .5 | `overview_cache_progress_test.go` new-source cache/partial/refresh cases |
| .6 | `TestAnalyzeNonSQLiteDoesNotReadFilesystem`; not-applicable total cases |
| .7, .9 | `storage-overview-card.test.tsx` localized row details and scope copy |
| .8 | `TestAnalyzeDoesNotModifyFiles`; overview permission and event payload regression coverage |

The criterion prefixes in this table are `AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-002`.
Task 01 should extend existing authorization/event tests only if projection wiring changes them.
Tests use temporary files and injected failures, never the live database.

## E2E tests

Add `storage-database-footprint.spec.ts` for chromium and
`mobile-storage-database-footprint.spec.ts` for mobile-chrome under `e2e/tests/system`.
Both cover .1, .3, .5, .7, and .9: inspect paths, compare rows and totals with
the same snapshot, refresh after adding a disposable backup, and inspect scope copy.
Controlled overview responses cover .4 and .6 without manipulating a production database.
Phone coverage taps both rows and verifies no document horizontal overflow.
Use the exact rebuilding runner commands in Task 03.

## Work orders

- [x] [Task 01: Database footprint measurements](task-01-database-measurements.md)
- [x] [Task 02: Database storage resource rows](task-02-storage-resource-rows.md)
- [x] [Task 03: Storage flow evidence](task-03-storage-flow-evidence.md)

All work orders are sequential. No delegation is authorized or required.

## Verification results

Implementation verification completed:

- Backend storage and overview checks passed: 210 storage-package tests and 9 targeted backend-app tests.
- Focused web tests passed: 44 tests; web typecheck passed.
- `i18n:zh-hant`, `i18n:check`, and `i18n:ratchet` passed.
- Public documentation validation passed: 61 tests and 46 published pages.
- Desktop and mobile database-footprint E2E checks passed, one test each.

## Risks

- SQLite and backup files can change during scanning; the result is a sampled footprint.
- Logical sizes and allocated blocks differ; this does not guarantee reconciliation with disk capacity.
- Configured roots may overlap existing categories; attribution must prevent additional double counting.
- Unsupported drivers must be explicit, without reporting fabricated zero bytes.
- Existing source-count fixtures and partial-response construction must include both new sources.
