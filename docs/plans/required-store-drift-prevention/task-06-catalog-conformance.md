---
id: "06-catalog-conformance"
title: "Complete catalog conformance"
status: done
wave: 6
depends_on:
  - "04-required-store-bootstrap"
  - "05-conformance-framework"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.5
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.3
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 06: Complete catalog conformance

## Summary

Add conformance adapters for every remaining catalog store. Turn catalog-to-adapter and dual-engine parity into mandatory test assertions.

## In scope

- Add support, system, plugin, provider, sync, automation, and feature-store adapters.
- Declare and satisfy each catalog capability.
- Compare catalog, bootstrap visits, adapter IDs, and engine declarations.
- Reject unknown, duplicate, missing, and SQLite-only adapters.
- Retire redundant store-specific parity scenarios after equivalent central coverage exists.

## Out of scope

- Remove store-specific regression tests that cover behavior beyond the suite.
- Change domain semantics to fit the generic runner.
- Modify CI discovery.

## Acceptance

- The central suite has one adapter for every catalog entry and both engines.
- Completeness fails before PostgreSQL setup when an adapter or capability is absent.
- Every required store passes the catalog scenarios on SQLite and PostgreSQL.

## Verification

```bash
KANDEV_TEST_POSTGRES_DSN='<test DSN>' go test -race ./internal/persistence/storeconformance ./internal/backendapp -run '^(TestStoreCatalogCompleteness|TestStoreConformance|TestRequiredStoreBootstrapCompleteness)$' -v
```

## Files likely touched

- `apps/backend/internal/persistence/storeconformance/core_adapters_test.go`
- `apps/backend/internal/persistence/storeconformance/support_adapters_test.go`
- `apps/backend/internal/persistence/storeconformance/provider_adapters_test.go`
- `apps/backend/internal/persistence/storeconformance/feature_adapters_test.go`
- `apps/backend/internal/persistence/storeconformance/suite_test.go`
- `apps/backend/internal/backendapp/required_store_bootstrap_test.go`
- Existing `store_postgres_test.go` files where scenarios become redundant.

## Dependencies

- Task 04 supplies final bootstrap constructors.
- Task 05 supplies the runner and adapter contract.

## Risks

- The central package imports many store packages and can expose import cycles.

## Parallelism

`sequential`

## Inputs

- System design sections: Conformance suite, Completeness enforcement.
- Task 01 final catalog.

## Results

Implemented fixed adapters for every catalog entry using the owning store
constructors, exact catalog/adapter/capability validation, and both-engine
coverage. SQLite passed 283 catalog tests; PostgreSQL 16 passed 565 conformance
and upgrade tests.
