---
id: "01-attribute-writer-occupancy"
title: "Attribute writer occupancy"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
acceptance_criteria:
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.1
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.4
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.6
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.7
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.8
system_design:
  - ../../specs/platform/system-design/runtime-failure-attribution.md
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
---

# Task 01: Attribute writer occupancy

## Summary

Measure real writer workloads on disposable storage.
Produce enough evidence to select a narrowly scoped repair or identify the remaining observation gap.

## In scope

- Inventory direct writer queries, transactions, external I/O inside transactions, and nested connection acquisition.
- Record candidate identity, entry duration, admitted span, outcome, pool deltas, and health-probe result.
- Exercise event reception, canonical projection, and terminal settlement with concurrent sidebar snapshots.
- Cover the workload matrix and measurement boundaries in the [plan](plan.md#technical-approach).
- Create `evidence.md` with reproducible commands, versions, operation counts, p50/p95/p99 durations, throughput, and probe failures.
- Retain meaningful correctness coverage and remove temporary instrumentation.

## Out of scope

- Live load, live configuration changes, engine migration, and production workload optimization.
- New database wrappers, permanent driver replacement, and changed health or transaction policy.

## Acceptance

- Each reported transaction span names a real operation and observes admission and settlement. Unknown coverage remains explicit.
- The real repository fixture preserves delivery order, replay idempotency, terminal settlement, cancellation cleanup, and separate-reader progress.
- The evidence ranks measured contributors and defines a specific next repair with acceptance criteria, or records why attribution remains unresolved.

## Verification

Run from the repository root. The test and benchmark names below are implemented deliverables.
The fixture must reject any externally supplied database path and create its own temporary directory.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run 'Test(WriterWorkload|.*AgentDelivery|SidebarQuery|UpdateMessage|CreateMessage|.*Payload|.*ConversationSource)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestWriterWorkload' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^$' -bench '^BenchmarkWriterWorkload$' -benchtime=512x -count=3)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/requiredstores ./internal/backendapp ./internal/db -run 'Test(PersistenceContentionFixture|RuntimeHealth|StartupHealth|HealthCheck|ProbeTables|RequiredPersistence|PersistenceMiddleware|SQLiteWriterTransactionAdmission)' -count=1)
python3 docs/plans/database-writer-contention/measure.py -- go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestWriterWorkload' -bench '^BenchmarkWriterWorkload$' -benchtime=512x -count=3
# Fixed production path: no redundant reservation UPDATE.
python3 docs/plans/database-writer-contention/measure.py -- go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^$' -bench '^BenchmarkWriterMessageUpdate$' -benchtime=128x -count=3
# Historical comparison: restore the reservation UPDATE in a disposable overlay.
python3 docs/plans/database-writer-contention/measure.py --restore-message-reservation -- go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^$' -bench '^BenchmarkWriterMessageUpdate$' -benchtime=128x -count=3
python3 docs/plans/database-writer-contention/measure.py -- go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestWriterWorkload' -count=1 -v
# Check correctness on both the fixed path and the restored comparison.
python3 docs/plans/database-writer-contention/measure.py -- go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run 'Test(UpdateMessage|CreateMessage|.*Payload|.*ConversationSource)' -count=1
python3 docs/plans/database-writer-contention/measure.py --restore-message-reservation -- go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run 'Test(UpdateMessage|CreateMessage|.*Payload|.*ConversationSource)' -count=1
git diff --check -- docs/plans/database-writer-contention apps/backend/internal/task/repository/sqlite
```

The benchmark logs its synthetic sizes and concurrency cases.
Fixed iteration counts replace the original ten-second calibration because fixture growth changes cost during a run.
The 512-round comparison uses identical operation counts with and without the transaction overlay.
The additional message-update benchmark uses 128 updates at each fixed content size.
The restored-reservation overlay represents the earlier message-update path. Durable-delivery methods were added after the incident build.
The extra case therefore separates current-checkout capacity evidence from paths present during the incident.
Run cases sequentially to avoid overlapping load. Report thresholds as observations, not universal performance guarantees.
Use barriers for correctness assertions. Do not substitute sleeps for proof of writer admission.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/writer_workload_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/writer_workload_bench_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/writer_workload_observation_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/writer_workload_isolated_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/writer_message_workload_bench_test.go` (new)
- `docs/plans/database-writer-contention/measure.py` (new, creates disposable Go overlays)
- `docs/plans/database-writer-contention/evidence.md` (new)
- `docs/plans/database-writer-contention/measurements.json` (new, synthetic observations)

Temporary observation sites in a disposable source copy:

- `apps/backend/internal/task/repository/sqlite/agent_delivery.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_settlement.go`
- `apps/backend/internal/task/repository/sqlite/message_payload_replay.go`

## Dependencies

None. Reuse the completed mechanism fixture instead of repeating its original investigation.

## Risks

- Transaction-entry time combines connection checkout and native BEGIN work.
- Cancellation can trigger automatic rollback. An application return is not sufficient proof of connection release.
- Candidate coverage does not establish ownership of every production write.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md)
- [Runtime failure attribution requirements](../../specs/platform/requirements/runtime-failure-attribution.md)
- [Required persistence requirements](../../specs/platform/requirements/postgres-domain-store-parity.md)
- Both system designs named in frontmatter.
- [Existing contention evidence](../runtime-log-reliability/persistence-evidence.md)
- [Writer admission decision](../../decisions/2026-10-05-sqlite-writer-transaction-admission.md)
- [Maintenance health decision](../../decisions/2026-09-17-maintenance-health-probe-coordination.md)
- `apps/backend/internal/persistence/requiredstores/health_contention_test.go`
- Existing delivery and sidebar snapshot tests beside the repository implementation.
- Backend guidance and the `/tdd` backend testing reference.

## Results

Completed on October 9, 2026. See [evidence.md](evidence.md) for findings and limitations,
and [measurements.json](measurements.json) for per-run counts and distributions.

The fixed-count delivery matrix ran with and without transaction observations (21 runs each).
Message updates ran with and without the reservation UPDATE (nine runs each), followed by three alternating large-message pairs.
Correctness, replay, sidebar, health, transaction-admission, and payload checks passed with the race detector.
The disposable experimental update also passed the selected payload and conversation-source checks.
The fixture correctness and isolated operation controls passed with the overlay and in the normal build.
No performance thresholds were introduced into CI assertions. No PostgreSQL integration server was exercised.

The original large-message runs produced nine failed probes and 318 successful sidebar reads.
The experimental removal reduced work but still produced failures, including four failures in the alternating comparison.
The unobserved delivery control also recorded two probe failures. Timing variability prevents a reliable observer-overhead estimate.

Production files and live configuration are unchanged. The initial aborted synthetic directory was removed.
Go removed completed fixture directories; overlay copies were temporary.
Historical attribution and independent workspace filesystem errors remain unresolved.
The next repair must preserve missing-message errors and transaction authority; the experimental source removal is not a complete patch.
