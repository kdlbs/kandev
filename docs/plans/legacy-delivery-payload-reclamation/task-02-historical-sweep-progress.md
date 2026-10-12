---
id: "02-historical-sweep-progress"
title: "Resume fair historical scanning"
status: done
wave: 2
depends_on:
  - "01-canonical-payload-proof"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-005.1
  - AC-PLATFORM-DELIVERY-RETENTION-005.3
  - AC-PLATFORM-DELIVERY-RETENTION-005.5
  - AC-PLATFORM-DELIVERY-RETENTION-006.2
  - AC-PLATFORM-DELIVERY-RETENTION-006.3
  - AC-PLATFORM-DELIVERY-RETENTION-006.6
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention-payloads.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 02: Resume fair historical scanning

## Summary

Let the automatic worker follow committed discovery progress through protected pages
and reach historical eligible output within its normal pass budget. Restart resumes
the same sweep without an expensive startup migration or manual session deletion.

## In scope and owned files

- `apps/backend/internal/task/models/agent_delivery.go`: bounded structural batch-result
  progress/exhaustion fields, if needed by the shared repository/worker contract.
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention.go` and
  `agent_delivery_retention_discovery.go`: report committed forward progress, distinguish
  phase exhaustion/wrap, retain persisted frontiers and atomicity.
- `apps/backend/internal/task/deliveryretention/worker.go`, `worker_productive_test.go`
  and new `worker_discovery_progress_test.go`: repeat advancing phases within the
  existing budget and preserve fair first visits to orphan/retirement phases.
- Repository worker/storage tests: historical protected prefix plus ongoing traffic
  and interruption/reopen evidence.

## Out of scope

No extra worker, cadence setting, feature flag, full startup scan, relaxed lease,
unbounded COUNT/JSON query, identity-labelled metric or forced recovery settlement.

## Acceptance

1. A committed protected-only page advances to later eligible output within the same
   pass. Empty/exhausted pages, completed wraps without further progress, failed commit
   and busy/deadline cases cannot create a repeat loop or claim progress.
2. Keep 50-candidate background batches, lower operator ceilings, 4 MiB payload limits,
   250 ms admitted SQL leases, five-second passes and 100 ms cancellable yields. Visit
   all three phases fairly; Stop joins admitted work before database closure.
3. A legacy mixed-history real-repository fixture drains eligible bodies across normal
   startup/restart passes while ongoing canonical projection/replay and protected
   terminal/prompt evidence remain correct. Production telemetry distinguishes scan
   advancement from deleted rows and sampled backlog from complete exhaustion.

## TDD and exact verification

Write `TestDeliveryRetentionWorkerProtectedDiscoveryProgress` first. Its current-code
failure must show a later eligible page was never reached after zero-deletion progress.
Use a deterministic clock and mixed protected/productive/exhausted pages. Add a real
repository restart case, not only a fake that returns its own expected flags.

From `apps/backend`:

```sh
go test ./internal/task/deliveryretention -run '^TestDeliveryRetentionWorker(ProtectedDiscoveryProgress|BackgroundQuantum|ProductivePhases|RemainingYieldBudget)' -count=1
go test -race ./internal/task/deliveryretention -count=1
go test ./internal/task/repository/sqlite -run '^TestDeliveryRetention(IntegrationHistoricalSweep|StorageSteadyState|IntegrationSQLiteIndependentHolder)' -count=1
```

The new integration test is `TestDeliveryRetentionIntegrationHistoricalSweep`.
Run its PostgreSQL subtests using disposable `KANDEV_TEST_POSTGRES_DSN`; report skips honestly.
Use the existing storage-rate fixture conventions and keep all returned command session
handles until terminal completion.

## Risks and results

Selected candidates are not durable progress. A wrapped frontier can compare lower
lexically while advancing the traversal; define exhaustion explicitly. Preserve retry
backoff and maintenance yielding, including honest SQLite busy-timeout behavior.

Results: complete. The worker test reproduced its failure to reach a later productive
page after protected-only discovery. Worker race tests and the persisted-frontier
integration now pass, including rollback, repository reopen and frontier wrap.
`DiscoveryAdvanced` is returned only after commit; `discovery_batches` reports forward
scan service separately from removed bytes. The named sustained-arrival tests passed
on SQLite and disposable PostgreSQL, including lower candidate ceilings and the
independent-holder case. Limits and maintenance admission remain unchanged.
