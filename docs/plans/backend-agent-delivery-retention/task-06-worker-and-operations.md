---
id: "06-worker-and-operations"
title: "Run bounded cleanup with configuration and telemetry"
status: complete
wave: 6
depends_on:
  - 05-deletion-and-orphans
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-005.1
  - AC-PLATFORM-DELIVERY-RETENTION-006.2
  - AC-PLATFORM-DELIVERY-RETENTION-006.3
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 06: Run bounded cleanup with configuration and telemetry

## Summary

Start automatic cleanup only after receipt-aware consumers, settlement proofs and deletion fencing work. Integrate maintenance admission, quiescence and typed operational configuration.

## In scope

- Add internal/task/deliveryretention with idempotent Start/Stop/restart, WaitGroup ownership, fake clock tests and goleak TestMain.
- Wire startup after readiness/reconciliation; wire shutdown and backup/restore/reset quiescence before pool close.
- Reuse per-pool TryAcquire, fair keysets, one batch context capped at 250 ms/remaining pass time for BEGIN and SQL, and a 5-second pass start cutoff. Never renew the transaction context after BEGIN. SQLite native BEGIN may remain busy up to its existing 5-second timeout after cancellation; measure it separately, prohibit post-cancel work and join settlement before shutdown/pool reuse. Do not change global busy_timeout or add an admission retry loop.
- Register exact startup keys/env aliases/defaults/ranges/provenance in the stable config catalog. Do not add profile or runtime flag entries.
- Implement production structured aggregate pass snapshots on both engines, including idle/protected/busy progress and bounded backlog samples. Add dev/pprof expvar only as a supplement; test prod without `/debug/vars` and PostgreSQL without a CLI dry-run surface. Counters advance after commit; keep lag/unavailable semantics.
- Add tests for busy maintenance, saturated traffic, protected first page, independent writer contention held past cancellation, failed-BEGIN reuse, reader progress, cancellation during candidate/hash work, disk full and schema/hash errors.

## Out of scope

Implementation was authorized by the user on 2026-10-09.
Agentctl journal policy, canonical-history expiry, new UI and feature flags are excluded.
This order does not authorize commit, push, merge or delegation.

## Acceptance

- The worker scans and changes bounded work, yields between batches and drains before restore/reset closes the pool; Stop-to-Start is safe.
- Defaults require no flag or config edit; invalid values fail startup and no safety predicate is configurable away.
- Production logs distinguish removals, protected/skipped work, unavailable samples and failures on both engines without identity labels or content. Dev/pprof expvar is supplemental; no new UI or production HTTP endpoint is promised.

## TDD evidence

Start each changed-logic scenario with a failing test against the pre-change behavior. Record Red, Green and refactor results.
Use existing backend test helpers, fake clocks and deterministic barriers. Do not use sleeps to manufacture race order.
Required new test groups: `TestDeliveryRetentionWorker`, `TestDeliveryRetentionConfig`, `TestDeliveryRetentionMetrics`, `TestDeliveryRetentionQuiescence`.
Names are planned deliverables, not existing passing tests. Before using a -run selector, list tests and confirm every required group exists.
The real repository and retained journal must prove state transitions where named; fakes do not prove database crash semantics.
Read `.agents/skills/tdd/SKILL.md` and its `references/backend-tests.md` at implementation time.

## Verification

Run from repository root. Commands are implementation-time checks, not run during planning.

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/deliveryretention -count=1)
(cd apps/backend && go test -trimpath -race ./internal/common/config -run 'TestDeliveryRetentionConfig' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetentionMetrics' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/system/database ./internal/backendapp -run 'TestDeliveryRetentionQuiescence' -count=1)
```

Worker integration tests must exercise SQLite and PostgreSQL pools. PostgreSQL readiness/worker tests are mandatory with KANDEV_TEST_POSTGRES_DSN; fake repository tests supplement them.
For every environment-gated test, record executed and skipped subtests. A skip cannot complete the corresponding acceptance.
The package helper checks document links/IDs; it does not prove runtime behavior.

## Files likely touched

- `apps/backend/internal/task/deliveryretention/` (new)
- `apps/backend/internal/backendapp/`
- `apps/backend/internal/common/config/catalog.go`
- `apps/backend/internal/common/config/source.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_metrics.go`
- `apps/backend/internal/system/maintenance/guard.go`
- `apps/backend/internal/system/database/`

Add focused `_test.go` files beside changed logic; split oversized tests rather than extending files past lint limits.
Paths marked new are intended deliverables. Other directory references identify the existing integration boundary.

## Dependencies

Complete `05-deletion-and-orphans` first. Execute sequentially in the primary session.

## Inputs

- [Requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md).
- [Design](../../specs/platform/system-design/backend-agent-delivery-retention.md); sections matching this scope.
- [Plan](plan.md), including baseline, storage thresholds and engine gates.
- Scoped `apps/backend/AGENTS.md`, existing delivery tests and nearby maintenance patterns.

## Risks

The process-local maintenance guard does not replace database serialization. Restore requires explicit worker quiescence, not a cancelled parent context that nobody drains.

## Parallelism

`sequential`. No delegation is authorized.

## Results

Implemented the cancellable bounded worker and wired it after readiness publication. Each retention, orphan and retirement batch independently uses the per-pool maintenance guard and a 250 ms transaction context within a five-second pass cutoff. Stop cancels and joins in-flight work; reset, restore and shutdown quiesce the worker before database mutation or close. Structural format/schema/hash/disk errors stop later destructive phases and suspend the worker until a successful repository readiness probe; transient failures use jittered, cancellable exponential retry capped at five minutes.

Added strict typed startup keys and aliases for payload grace, replay age/count, cadence and batch rows/bytes. Defaults and ranges are validated at startup, all six settings are YAML/environment-only, and no profile or runtime flag is registered. Production emits aggregate pass logs. Dev/pprof expvar supplements the logs with bounded candidate samples, sample-limit availability, oldest eligible age and post-commit counters; the metrics contain no delivery identities.

Red/Green/refactor coverage includes fake-clock lifecycle, retry and structural-resume tests; strict configuration validation; real-repository committed metric assertions; and reset/shutdown quiescence. Required checks passed:

- `cd apps/backend && go test -trimpath -race ./internal/task/deliveryretention -count=1`
- `cd apps/backend && go test -trimpath -race ./internal/common/config -run 'TestDeliveryRetentionConfig' -count=1`
- `cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetentionMetrics' -count=1`
- `cd apps/backend && go test -trimpath -race ./internal/system/database ./internal/backendapp -run 'TestDeliveryRetentionQuiescence' -count=1`
- `KANDEV_TEST_POSTGRES_DSN=<isolated test DSN> go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(Metrics|WorkerRepository)$' -count=1`

The dual-engine worker test executed SQLite and PostgreSQL subtests, including format readiness, deletion retirement and final evidence removal. The metrics acceptance ran against a real SQLite repository and verified counters advanced only after committed work. `gofmt` and whitespace checks passed. Other W06 failure/concurrency regressions from the preceding repository work orders remain covered by their recorded SQLite/PostgreSQL receipts.
