---
id: "01-bound-discovery"
title: "Bound protected discovery continuation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-005.1
  - AC-PLATFORM-DELIVERY-RETENTION-005.5
  - AC-PLATFORM-DELIVERY-RETENTION-005.6
  - AC-PLATFORM-DELIVERY-RETENTION-006.3
  - AC-PLATFORM-DELIVERY-RETENTION-006.6
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention-throughput.md
---

# Task 01: Bound protected discovery continuation

## Summary

Make worker discovery progress relative to the uninterrupted continuation chain.
Keep immediately productive cleanup and finite protected discovery, then wait
at the configured idle interval after no novel frontier remains.

## In scope

- Add committed before/after typed vectors for the three discovery sources.
- Share repository progress calculation between SQL writes and result vectors.
- Add worker-owned component maxima across immediate passes and reset on waits.
- Add real repository and worker integration coverage plus fake-clock tests.

## Out of scope

Eligibility, schema, configuration, provider protocols, UI, live data and restarts.

## Acceptance

1. `TestDeliveryRetentionWorkerStaggeredWrapYields` fails before the correction
   because staggered source wraps allow indefinite immediate passes. Fake time
   must cross at least two pass budgets; then assert an idle timer, no removals,
   finite candidate visits, bounded batches, aggregate-only snapshots and prompt
   cancellation. Include receipt/compaction, prompt/compaction and all-source
   variations; update old fake repositories to supply committed vectors.
2. `TestDeliveryRetentionProtectedSweepRepository` seeds small disjoint raw,
   pruned-receipt and protected-prompt inventories through existing fixture
   helpers on SQLite and environment-gated PostgreSQL. Drive the actual worker
   with injectable clock/timer hooks. Assert eventual idle, unchanged protected
   evidence and no synthetic settlement. A mixed case reaches a later eligible
   row, including when progress crosses pass boundaries. Do not settle for a
   helper-only regression or a fake that simply declares exhaustion.
3. `TestDeliveryRetentionDiscoveryVectorCommit` covers rollback returning no
   vector, actual persisted empty/wrapped positions, reopen preserving cursor
   authority, and bounded later idle cycles visiting newly eligible lower keys.
   Existing continuous-productive, phase fairness, gate contention, backoff,
   shutdown and historical-sweep tests remain green.

## Verification

Run the regression first, record the expected failure, then implement. Commands
below run from repository root. PostgreSQL coverage is required before marking
the order done; use a disposable test database through the existing test harness.
Do not print credentials. Record engine skips explicitly if unavailable.

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/deliveryretention -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run '^TestDeliveryRetention(ProtectedSweepRepository|DiscoveryVectorCommit|IntegrationHistoricalSweep|WorkerRepository|Metrics|StorageProtectedGap.*)$' -count=1 -timeout=4m)
bash scripts/lint-go-changed apps/backend/internal/task/models/agent_delivery.go apps/backend/internal/task/deliveryretention/worker.go apps/backend/internal/task/deliveryretention/discovery.go apps/backend/internal/task/deliveryretention/worker_protected_sweep_test.go apps/backend/internal/task/deliveryretention/worker_discovery_progress_test.go apps/backend/internal/task/deliveryretention/worker_continuation_test.go apps/backend/internal/task/repository/sqlite/agent_delivery_retention.go apps/backend/internal/task/repository/sqlite/agent_delivery_retention_discovery.go apps/backend/internal/task/repository/sqlite/agent_delivery_retention_protected_sweep_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If extracting a production helper into another Go file, add that exact changed
path to the lint command and include its owning package in these tests.

## Files likely touched

- `apps/backend/internal/task/models/agent_delivery.go`
- `apps/backend/internal/task/deliveryretention/worker.go`
- `apps/backend/internal/task/deliveryretention/discovery.go` (new)
- `apps/backend/internal/task/deliveryretention/worker_protected_sweep_test.go` (new)
- `apps/backend/internal/task/deliveryretention/worker_discovery_progress_test.go`
- `apps/backend/internal/task/deliveryretention/worker_continuation_test.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_discovery.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_protected_sweep_test.go` (new)

## Dependencies

None. Single vertical scheduling repair, sequential in the primary conversation.

## Risks

Frontier identifiers are private scheduling evidence; never publish them in logs
or metrics. Preserve source ordering, transaction commit semantics and idle reset.
Do not reset the discovery tracker merely because cleanup removed a row.

## Parallelism

`sequential`

## Inputs

- [Throughput design](../../specs/platform/system-design/backend-agent-delivery-retention-throughput.md), especially Discovery across immediate passes.
- [Retention requirements](../../specs/platform/requirements/backend-agent-delivery-retention.md), 005 and 006.
- Existing historical-sweep, worker-repository, discovery and continuation tests.
- Backend TDD guidance and the reproduction recorded in plan.md.

## Results

Implemented committed before/after vectors and component-wise discovery maxima
across immediate passes, resetting on waits and worker shutdown. No eligibility,
schema, configuration, provider, logging identity or recovery authority changed.

RED: the real repository-to-worker `TestDeliveryRetentionProtectedSweepRepository`
failed on the original code after 101 protected-only batches, reaching retry
instead of idle. GREEN: the targeted worker race suite passed (1.057s), including
three source combinations over multiple passes, idle rechecks, tuple ordering,
phase fairness and cancellation. The documented repository race command passed
(10.552s) against both SQLite and disposable PostgreSQL 17. It covers protected
raw/receipt discovery, protected legacy prompt proof, bounded lower-key rechecks,
rollback, committed vectors, reopen, empty resets and existing historical/gap
regressions. A lower-key row may require more than the first post-wrap idle
cycle; the fixture requires pruning within three cycles rather than inferring
starvation from a single cycle.

Changed-package lint passed with 0 issues. Catalog validation, specification lint,
PR-documentation coverage preflight against actual changed paths, and diff checks
passed. The new discovery.go helper is included in the lint command above.
Public-doc impact checked through docs-maintainer: existing configuration.md
already defines idle cadence and immediate productive continuation, so this
repair requires no public text or screenshot change. No browser UI changed.

No delegation was used. PR checks and merge are tracked in the delivery session;
local results do not claim remote checks or merge completion.
