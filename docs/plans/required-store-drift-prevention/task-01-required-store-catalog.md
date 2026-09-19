---
id: "01-required-store-catalog"
title: "Create the required-store catalog"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.4
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 01: Create the required-store catalog

## Summary

Create the authoritative SQL store catalog and its state tracker. Audit all bootstrap paths and record every schema owner.

## In scope

- Add catalog descriptors, stable IDs, dependencies, tables, and capabilities.
- Add tracker states and validation for unknown, duplicate, out-of-order, and missing results.
- Audit schema constructors in all `backendapp` startup phases.
- Add catalog unit tests that run without a database.

## Out of scope

- Change startup error handling.
- Add engine conformance adapters.
- Add runtime health polling.

## Acceptance

- The catalog contains every built-in SQL schema owner that uses the selected Kandev database.
- Catalog validation rejects duplicate IDs, missing dependencies, cycles, and invalid capabilities.
- The tracker returns deterministic snapshots in catalog order.

## Verification

```bash
go test -race ./internal/persistence/requiredstores -run '^Test(Catalog|Tracker)' -v
```

## Files likely touched

- `apps/backend/internal/persistence/requiredstores/catalog.go`
- `apps/backend/internal/persistence/requiredstores/tracker.go`
- `apps/backend/internal/persistence/requiredstores/catalog_test.go`
- `apps/backend/internal/persistence/requiredstores/tracker_test.go`
- `apps/backend/internal/backendapp/storage.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/backendapp/worktree.go`

## Dependencies

None.

## Risks

- A store can create schema inside a service constructor that does not use a store name.

## Parallelism

`sequential`

## Inputs

- System design sections: Required store catalog, Bootstrap contract.
- Existing constructors found from `db.Pool`, `Writer`, and `Reader` usage.

## Results

Implemented the immutable catalog, dependency-aware tracker, runtime status
snapshot, and table-level health probe. Verified with:

```text
go test -race ./internal/persistence/requiredstores -run 'Test(Catalog|Tracker)' -count=1
```

Five tests passed.
