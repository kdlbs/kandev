---
id: "05-conformance-framework"
title: "Build the conformance framework"
status: done
wave: 5
depends_on:
  - "01-required-store-catalog"
  - "02-shared-sql-rendering"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.6
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.7
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.3
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 05: Build the conformance framework

## Summary

Create the reusable dual-engine scenario runner and central adapter contract. Prove it with representative core stores.

## In scope

- Open isolated SQLite databases and PostgreSQL schemas through one engine interface.
- Define baseline and capability-specific adapter callbacks.
- Run identical assertion callbacks for both engines.
- Add task, workflow, system-settings, and one small support-store adapter.
- Prove fresh, replay, CRUD, boolean, timestamp, conflict, and transaction scenarios.

## Out of scope

- Add every catalog adapter.
- Enable the CI completeness gate.
- Add previous-stable fixtures.

## Acceptance

- The runner produces stable `engine/store/scenario` test names.
- Capability callbacks fail when omitted or duplicated.
- Core adapters demonstrate every scenario type on both engines.

## Verification

```bash
KANDEV_TEST_POSTGRES_DSN='<test DSN>' go test -race ./internal/testutil/storeconformance ./internal/persistence/storeconformance -run '^TestStoreConformance/(sqlite3|pgx)/(task|workflow|system-settings|user)/' -v
```

## Files likely touched

- `apps/backend/internal/testutil/storeconformance/engine.go`
- `apps/backend/internal/testutil/storeconformance/suite.go`
- `apps/backend/internal/testutil/storeconformance/suite_test.go`
- `apps/backend/internal/persistence/storeconformance/suite_test.go`
- `apps/backend/internal/persistence/storeconformance/core_adapters_test.go`

## Dependencies

- Task 01 defines descriptors and capabilities.
- Task 02 provides common SQL behavior for adapters.

## Risks

- A generic callback can hide different behavior if assertions depend on engine names.

## Parallelism

`sequential`

## Inputs

- System design section: Conformance suite.
- Existing `testutil.OpenIsolatedPostgres` and store-specific PostgreSQL tests.

## Results

Implemented the reusable dual-engine runner with stable subtest names,
capability validation, isolated engines, and replay support. The runner tests
passed, followed by the complete catalog suite on SQLite and PostgreSQL 16.
