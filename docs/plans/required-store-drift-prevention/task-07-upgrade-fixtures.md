---
id: "07-upgrade-fixtures"
title: "Add previous-stable upgrade fixtures"
status: done
wave: 7
depends_on:
  - "04-required-store-bootstrap"
  - "06-catalog-conformance"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006
acceptance_criteria:
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.4
system_design:
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 07: Add previous-stable upgrade fixtures

## Summary

Commit provenance-checked `v0.93.0` fixtures for SQLite and PostgreSQL. Run current bootstrap, replay, and sentinel checks against both.

## In scope

- Generate fixtures from tag `v0.93.0` and record its source commit.
- Preserve representative rows for every schema owner present in the stable release.
- Represent the PostgreSQL partial store state from issue #3352.
- Add a fixture manifest, checksums, and an explicit update script.
- Run current schema initialization twice and execute conformance reads.
- Add PostgreSQL 16 and 18 upgrade entry points.

## Out of scope

- Store customer data or credentials in fixtures.
- Test upgrades from every historic release.
- Add automatic fixture rotation during release.

## Acceptance

- Fixture provenance and checksums fail on an unreviewed change.
- SQLite and PostgreSQL upgrades preserve all sentinel values.
- A second current initialization changes no expected data and returns no error.

## Verification

```bash
KANDEV_TEST_POSTGRES_DSN='<test DSN>' go test -race ./internal/persistence/storeconformance -run '^TestPreviousStableUpgrade/(sqlite3|pgx)/v0.93.0$' -v
```

## Files likely touched

- `apps/backend/internal/persistence/storeconformance/upgrade_test.go`
- `apps/backend/internal/persistence/storeconformance/testdata/upgrades/v0.93.0/manifest.json`
- `apps/backend/internal/persistence/storeconformance/testdata/upgrades/v0.93.0/sqlite.sql`
- `apps/backend/internal/persistence/storeconformance/testdata/upgrades/v0.93.0/postgres.sql`
- `apps/backend/scripts/update-store-schema-fixtures.sh`

## Dependencies

- Task 04 supplies current bootstrap order.
- Task 06 supplies all conformance reads.

## Risks

- A generated PostgreSQL dump can include unstable ownership or extension statements.

## Parallelism

`sequential`

## Inputs

- System design sections: Upgrade fixtures, PostgreSQL version coverage.
- Tag `v0.93.0` and issue #3352.

## Results

Committed schema-only SQLite and PostgreSQL fixtures generated from tag
`v0.93.0` at commit `866bd324855fda1f71e13b8863d2c71d4a11237e`, with checksums,
sentinels, and known missing provider stores. Current initialization, replay,
sentinel checks, and PostgreSQL 18 upgrade coverage passed.
