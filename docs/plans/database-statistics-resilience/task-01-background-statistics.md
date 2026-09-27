---
id: "01-background-statistics"
title: "Cache and bound database statistics"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.1
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.2
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.3
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.4
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.1
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.2
system_design:
  - ../../specs/system-page/system-design/database-statistics-snapshot.md
---

# Task 01: Cache and bound database statistics

## Summary

Make the database status endpoint return promptly while one background worker measures logical totals. Preserve last good data across concurrent reads and transient failure, with explicit freshness and nullable cold totals.

## In scope

- Split fast metadata from logical totals, add snapshot state and measurement time to the API.
- Implement one cancellable, bounded batch scanner with TTL, backoff, maintenance coordination, and generation invalidation.
- Update metric publication and focused SQLite/PostgreSQL test coverage.

## Out of scope

- UI copy, layout, and compaction analysis behavior.
- Persisting the snapshot across backend restarts.

## Acceptance

1. Cold, concurrent, and expired GETs return metadata without waiting for one shared logical scan; missing totals are null, not zero.
2. A failed or cancelled scan keeps the last complete values and measured time, while maintenance and replacement prevent stale publication.
3. Batches release database resources, observe cancellation, and only a fully completed scan updates logical totals and gauges.

## Verification

```bash
(cd apps/backend && go test ./internal/system/database ./internal/system/maintenance -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
# With KANDEV_TEST_POSTGRES_DSN configured, rerun the database package test for PostgreSQL behavior.
```

## Files likely touched

- `apps/backend/internal/system/database/stats.go`
- `apps/backend/internal/system/database/stats_cache.go` (new)
- `apps/backend/internal/system/database/stats_cache_test.go` (new)
- `apps/backend/internal/system/database/stats_test.go`
- `apps/backend/internal/system/database/metrics_vars.go`
- `apps/backend/internal/system/system.go`
- `apps/backend/internal/system/database/maintenance.go`
- `apps/backend/internal/system/database/reset.go`

## Dependencies

None. Inspect the actual maintenance and restore call sites before wiring invalidation; file names are candidates, not authority.

## Risks

- PostgreSQL keyset types and SQLite ordering may differ.
- Invalidation must reject a worker result from the previous database generation.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/system-page/requirements/database-statistics-snapshot.md)
- [Design](../../specs/system-page/system-design/database-statistics-snapshot.md)
- Existing `storage.OverviewCache` and database stats tests.

## Results

Implemented the process-local logical statistics cache, bounded keyset scanner, live metadata fallback, generation invalidation, maintenance deferral, and metric publication. Restore and reset clear and stop database-derived workers; shutdown joins the scanner.

Verification passed:

- `cd apps/backend && go test ./internal/system/database ./internal/system/maintenance ./internal/system -count=1`
- `cd apps/backend && go run ./cmd/sqlguard ./internal`
- `cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1`

The PostgreSQL DSN-gated integration was not run. SQLite scanner coverage and fake-PostgreSQL route coverage passed.

## Review remediation

Missing required tables now fail the scan. A cold cache reports unavailable, and a failed refresh after a complete scan preserves the prior values and measurement time. Empty existing tables still measure as zero.

Verification after the review fix:

- `cd apps/backend && go test ./internal/system/database ./internal/system -count=1`
- `cd apps/backend && make build`
