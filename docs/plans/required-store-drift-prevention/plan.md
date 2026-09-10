---
created: 2026-09-05
status: implemented
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-008
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
legacy_specs: []
---

# Implementation Plan: Required Store Drift Prevention

## Overview

This package makes SQL store coverage catalog-driven and fail-closed. It starts from `origin/main` commit `d74e85883`, which contains PR #3372.

The work establishes metadata and shared SQL helpers first. Bootstrap, conformance, upgrade, health, CI, and contributor documentation then consume those contracts.

## Scope

### In scope

- Define one inventory for every built-in SQL schema owner.
- Make required store initialization fatal before readiness.
- Preserve independent degradation for external provider credentials and APIs.
- Add shared schema rendering, query rebinding, and static SQL checks.
- Run one behavior suite against SQLite and PostgreSQL for every catalog entry.
- Add clean-install and previous-stable upgrade coverage for both engines.
- Expose required-store health through readiness, diagnostics, and service errors.
- Replace marker-based PostgreSQL CI discovery with fixed catalog-driven tests.
- Document the contributor and operator contracts.

### Out of scope

- Active-active backend support.
- Cross-engine data migration.
- Automatic PostgreSQL backup or restore.
- Automatic repair of a damaged schema.
- Provider API or authentication policy changes.
- Filesystem-only stores.
- A change to `/health` liveness behavior.

## Technical approach

### Catalog and startup tracking

Add `internal/persistence/requiredstores` with immutable descriptors and a runtime tracker. Audit every SQL schema owner reached from `internal/backendapp`.

The tracker rejects unknown, duplicate, out-of-order, and missing store results. Bootstrap passes the tracker through every current repository and secondary store phase.

### Shared SQL contracts

Extend `internal/db/dialect` with token-based schema rendering. Add shared execution helpers that rebind at the final database or transaction boundary.

Remove store-local DDL replacements from the PR #3372 store set. Keep driver differences in central helpers or explicit dialect branches.

### Static SQL analysis

Add `internal/db/sqlguard` and `cmd/sqlguard`. The analyzer uses Go syntax trees and exact exemptions from `internal/db/sqlguard/exemptions.json`.

The first rules cover SQLite catalog syntax, conflict syntax, raw placeholders, boolean integers, SQLite date functions, and direct `DATETIME` types.

### Required and degradable phases

Initialize provider-owned SQL stores before provider services. Then pass those stores into service constructors.

Remote credentials, authentication, probes, and pollers remain nonfatal. Local store errors stop startup and leave readiness unavailable.

### Conformance and upgrades

Add a reusable runner in `internal/testutil/storeconformance`. Add fixed aggregation adapters in `internal/persistence/storeconformance`.

The aggregation package compares adapter IDs and capabilities with the catalog before it opens an engine. It runs the same behavior against SQLite and PostgreSQL.

Commit `v0.93.0` schema fixtures with provenance and sentinel rows. Run current bootstrap and replay against each fixture.

### Health and caller behavior

Run an immediate required-store probe before readiness and a periodic probe after startup. Project aggregate state through `/ready` and authenticated system diagnostics.

Gate stateful HTTP and WebSocket entry points while persistence is unhealthy. Return a stable `persistence_unavailable` response without raw database data.

### CI and documentation

Replace the `PostgresDSNFromEnv` grep in `.github/workflows/backend-tests.yml` with fixed package commands. Run full conformance on PostgreSQL 16.

Run clean-boot and upgrade coverage on PostgreSQL 18. Mirror and pin the service image through `.github/workflows/ci-base-image.yml`.

Update `docs/public/backend-development.md`, `docs/public/operations.md`, the root `AGENTS.md`, and `apps/backend/AGENTS.md`.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.1` through `.7` | Central SQLite and PostgreSQL conformance plus boot tests |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002.1`, `.4` | Failure-injected required-store bootstrap tests |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002.2`, `.3` | Provider credential and remote-probe isolation tests |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.1` through `.4` | Catalog, bootstrap-visit, adapter, engine, and diagnostics completeness tests |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.1` through `.4` | Reusable scenario runner and fixed aggregation package |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.1` through `.5` | SQL guard fixtures and shared dialect helper tests |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.1` through `.4` | `v0.93.0` fixture upgrades on SQLite, PostgreSQL 16, and PostgreSQL 18 |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.1` through `.5` | Readiness, diagnostics, middleware, recovery, and liveness tests |
| `AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-008.1` through `.3` | Public-doc checks and workflow contract tests |

## E2E tests

No browser test is required. Backend HTTP tests cover the user-visible readiness and `503` contracts.

## Work orders

- [x] [Task 01: Create the required-store catalog](task-01-required-store-catalog.md)
- [x] [Task 02: Centralize shared SQL rendering](task-02-shared-sql-rendering.md)
- [x] [Task 03: Enforce SQL dialect safety](task-03-sql-dialect-safety.md)
- [x] [Task 04: Enforce required-store bootstrap](task-04-required-store-bootstrap.md)
- [x] [Task 05: Build the conformance framework](task-05-conformance-framework.md)
- [x] [Task 06: Complete catalog conformance](task-06-catalog-conformance.md)
- [x] [Task 07: Add previous-stable upgrade fixtures](task-07-upgrade-fixtures.md)
- [x] [Task 08: Expose required-store health](task-08-required-store-health.md)
- [x] [Task 09: Install CI and contributor gates](task-09-ci-contributor-gates.md)

## Verification results

Implemented and verified with the targeted package suites, full SQLite catalog
conformance, PostgreSQL 16 conformance and upgrade coverage, and PostgreSQL 18
upgrade and boot coverage. The broad `go test ./internal/...` audit reached
9,902 passing tests; nine config and launcher home-discovery tests failed in
this runner because `/root/.kandev/config.yaml` took precedence over their
temporary home fixtures.

## Risks

- Store constructors use varied dependencies and startup phases. The audit can find hidden schema owners after the catalog shape lands.
- Some provider constructors combine schema work with external setup. Their split must preserve existing cleanup and polling behavior.
- Static placeholder analysis cannot infer arbitrary runtime strings. Shared execution helpers reduce this blind spot.
- The `v0.93.0` PostgreSQL fixture must preserve the incident's partial initialization state.
- Runtime health gating can block diagnostics unless the route exclusions are exact.
- PostgreSQL 18 adds CI time and needs an immutable mirrored image.
