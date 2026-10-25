---
id: "03-analytics-admission"
title: "Bound analytics database occupancy"
status: done
wave: 1
depends_on:
  - 02-query-aggregation
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-002
  - REQ-PLATFORM-INTERACTIVE-READS-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
acceptance_criteria:
  - AC-PLATFORM-INTERACTIVE-READS-002.1
  - AC-PLATFORM-INTERACTIVE-READS-002.2
  - AC-PLATFORM-INTERACTIVE-READS-002.3
  - AC-PLATFORM-INTERACTIVE-READS-002.4
  - AC-PLATFORM-INTERACTIVE-READS-001.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.2
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.5
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 03: Bound analytics database occupancy

## Summary

Limit shared analytics database occupancy and preserve cancellation and real health failures.
Prove normal reads and health probes succeed while analytics work is queued.

## In scope

- Own the repository admission gate, eight public method boundaries, ten-second deadline, Stats HTTP timeout mapping, and bounded diagnostic fields.
- Own deterministic shared-pool pressure tests and final admission-enabled reference timings.

## Out of scope

Other work orders, new product metrics, health-policy changes, and live-instance mutation.

## Acceptance

- At most two analytics operations execute across HTTP and plugin callers sharing the repository; queued calls hold no SQL connection.
- Cancellation, deadline, SQL error, and row-scan cleanup release capacity. Actual missing stores still reject stateful traffic and recover normally.
- Shared-pool reads/probes pass during controlled analytics load; final all-seven benchmark meets the five-second p95 target.

## Regression tests

Use `TestAnalyticsAdmissionBound`, `TestAnalyticsAdmissionCancellation`, and `TestAnalyticsAdmissionReleasesOnError` with explicit channels/barriers, not sleeps. `TestAnalyticsReadAvailability` must use the real four-reader SQLite pool and shared repository wiring, hold two admitted operations, queue more, and run a real required-store health check plus normal context read. Exercise HTTP and service/plugin entry points, not two unrelated gate instances. Keep one real missing-table case to show there is no fail-open change. `BenchmarkStatsHTTPReference` launches the seven authorized Stats handlers against the reference fixture and records per-cycle completion; report individual samples and p95, since Go's mean benchmark output alone is insufficient.

## Verification

Run from the repository root. Before the first pnpm command in a fresh worktree,
run `(cd apps && pnpm install --frozen-lockfile)`. New test paths below must be
created by this work order; verify test discovery before claiming a pass.

```bash
(cd apps/backend && go test -race ./internal/analytics/repository/sqlite ./internal/analytics/handlers ./internal/analytics/service ./internal/persistence/requiredstores)
(cd apps/backend && go test ./internal/backendapp -run 'TestRequiredPersistence' -count=1)
(cd apps/backend && go test ./internal/analytics/handlers -run '^TestAnalyticsReadAvailability' -count=1)
(cd apps/backend && go test ./internal/analytics/handlers -run '^$' -bench '^BenchmarkStatsHTTPReference' -benchtime=10x -count=1)
(cd apps/backend && go test ./internal/analytics/repository/sqlite -run '^$' -bench '^BenchmarkStatsReference' -benchtime=10x -count=1)
```

If additional test files are changed during extraction, add their exact commands
here and run them before completion. Record skipped external services as blockers
to that validation, never as passing evidence.

## Files likely touched

- `apps/backend/internal/analytics/repository/sqlite/{repository.go,stats.go}`
- New `apps/backend/internal/analytics/repository/sqlite/admission.go` and `admission_test.go`
- `apps/backend/internal/analytics/handlers/stats_handlers.go` and `stats_handlers_test.go`
- New `apps/backend/internal/analytics/handlers/stats_reference_bench_test.go`
- New `apps/backend/internal/analytics/handlers/stats_read_availability_test.go` and its real SQLite fixture
- Existing `apps/backend/internal/persistence/requiredstores/health_test.go` and middleware tests as verification inputs
- `apps/backend/internal/analytics/repository/provider.go` and `apps/backend/internal/backendapp/storage.go` if bounded logging needs constructor wiring
- `docs/public/operations.md`: explain retryable analytics pressure separately from real persistence failure when implementation ships
- Backend scoped `AGENTS.md` if the analytics execution-boundary description needs updating.

## Dependencies

02-query-aggregation

## Risks

Metric compatibility, portable SQL, cancellation cleanup, and misleading performance claims. See the manifest risks.

## Parallelism

`sequential`

## Inputs

- Applicable requirements and system designs from frontmatter, read in full.
- Investigation evidence and current source pointers in [plan.md](plan.md).
- Existing tests adjacent to owned files; E2E uses `test-base`, API seeding, and causal waits.

## Results

Implemented a two-operation admission gate shared by all eight analytics
repository reads. Queue wait occurs before a SQL connection is acquired and is
bounded by a ten-second total operation deadline. HTTP Stats maps admission
pressure to HTTP 503 with `analytics_busy` and `Retry-After: 2`; real SQL and
required-store failures retain their existing error paths.

RED evidence was the incident's shared-reader saturation: seven independent
Stats requests competed with the four-reader pool and caused the two-second
required-store probe to time out. GREEN evidence proves the bounded path:

- `cd apps/backend && go test -race ./internal/analytics/repository/sqlite ./internal/analytics/handlers ./internal/analytics/service ./internal/persistence/requiredstores` passes.
- `cd apps/backend && go test ./internal/backendapp -run 'TestRequiredPersistence' -count=1` passes the real required-persistence fail-closed/recovery checks.
- `cd apps/backend && go test ./internal/analytics/handlers -run '^TestAnalyticsReadAvailability' -count=1` passes with a production-shaped separate writer and four-reader SQLite pool. Two concurrent HTTP operations occupy real reader connections; the analytics service/plugin path queues behind the shared repository gate, while a normal reader query and required-store probe remain healthy. The companion test drops the required table and still observes a failed health check.
- `cd apps/backend && go test ./internal/analytics/handlers -run '^$' -bench '^BenchmarkStatsHTTPReference$' -benchtime=10x -count=1 -v` passes against the required disposable fixture. The month range reports `1,957,036,192 ns/op` and a nearest-rank cycle p95 of `3,033,327 us`; the all range reports `1,671,665,105 ns/op` and a nearest-rank cycle p95 of `1,939,193 us`. Each cycle launches all seven authorized handlers concurrently through one shared two-slot repository admission gate, and `-v` logs the individual request durations.
- `cd apps/backend && go run ./cmd/sqlguard ./internal` passes.

The handler-package serialization benchmark remains a mock-repository diagnostic
and the SQLite `BenchmarkStatsReference` all-seven sub-benchmark remains a
sequential query baseline owned by Task 02. Neither is used as the final
admission latency gate. The concurrent HTTP measurements above use the required
700-task, 800-session, 5,000-turn, 650,000-message, 2,200-commit, five-repository
shape in each of two workspaces. They are disposable reference measurements, not
production latency claims. The PostgreSQL parity subtest was skipped because
`KANDEV_TEST_POSTGRES_DSN` was not set.
