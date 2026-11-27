---
id: "01-restrict-historical-repair"
title: "Restrict prompt ordering repair to outstanding state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-002
acceptance_criteria:
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-002.1
  - AC-PLATFORM-STARTUP-HOUSEKEEPING-002.2
system_design:
  - ../../specs/platform/system-design/startup-housekeeping.md
---

# Restrict historical prompt repair

## Summary and scope

Use TDD to remove unconditional historical prompt aggregation from schema replay.
Preserve the existing microsecond ordering, tie-by-id behavior, raw legacy rows,
deleted-message semantics, and nondecreasing sequence counters. Use indexed
outstanding-row selection and restrict seeding to sessions without a counter.
No applied-marker ledger, backup change, or migration-framework conversion.

## Owned files and dependencies

- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- Existing prompt-index, schema replay, startup-cost and cancellation tests.
- New `startup_housekeeping_test.go` and PostgreSQL counterpart in that package.
- Real task repository initialization and `internal/testutil.OpenIsolatedPostgres`.

No other work order may edit this boundary concurrently.

## Implementation acceptance

1. An upgraded second initialization performs no full-history message aggregation;
   prove query selection with statement observation/query plans, not just timing.
2. Fresh, partially upgraded, and legacy stores still receive correct ordinals and
   missing counters on both dialects; existing counters never decrease.
3. Cancellation and replay preserve repairability, and populated fixture evidence
   records before/after upgrade and second-start durations on the same environment.

## Verification

From the repository root. Capture RED before the change and GREEN afterwards.
Provision an isolated PostgreSQL service and set its test DSN if absent; a skipped
PostgreSQL selection does not complete this work order. Never print credentials.

```bash
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run 'PromptIndex|PromptSequence|BackfillPromptSeq|NewWithDBContext|Schema.*Replay' -count=1)
(cd apps/backend && test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && go test -race ./internal/task/repository/sqlite -run 'Postgres.*(PromptIndex|PromptSequence|Schema.*Replay)' -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite -run '^TestStartupMigrationCosts(Populated)?$' -count=1 -v)
git diff --check
```

Run changed-file Go lint with `scripts/lint-go-changed` from the repo root for
every Go path changed by this work order. Existing tests that call
`backfillPromptSeq` after inserting raw legacy rows must continue to pass.

## Risks

Restricting the seed query incorrectly can reset counters after deletion or miss
legacy rows inserted after an initial schema initialization. Partial indexes must
be supported and used on both dialects. Do not infer upgrade completion from an
existing column or a global schema version alone.

## Results

Completed with indexed outstanding-session selection and atomic ordinal/counter
repair. A completed replay does not even prepare `MAX`; a SQLite authorizer
positive control rejects historical aggregation, and EXPLAIN confirms the partial
index. Existing counters advance when needed and never decrease.

- RED: denied historical aggregation and missing partial-index plan; a second
  regression caught a counter left behind after a legacy-row repair.
- GREEN: task-defined SQLite and isolated PostgreSQL prompt/index/replay tests,
  including `-trimpath -race`, passed. Cancellation/replay cases passed.
- Populated fixture (Linux amd64, Go 1.26, 24 sessions / 6,000 messages): upgrade
  462 ms, subsequent initialization 82 ms in the recorded race-enabled run.
  These are synthetic measurements, not a production startup prediction.
- `go run -trimpath ./cmd/sqlguard ./internal`: passed.
- PostgreSQL-enabled `go test -trimpath -race
  ./internal/persistence/storeconformance -count=1`: passed (151 seconds).
- `bash scripts/lint-go-changed` for the three changed repository Go files:
  passed, zero issues.
