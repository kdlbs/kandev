---
id: "02-continuous-retention"
title: "Drain productive retention continuously"
status: done
wave: 2
depends_on:
  - "01-snapshot-copy"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-005.1
  - AC-PLATFORM-DELIVERY-RETENTION-005.5
  - AC-PLATFORM-DELIVERY-RETENTION-005.6
  - AC-PLATFORM-DELIVERY-RETENTION-006.3
  - AC-PLATFORM-DELIVERY-RETENTION-006.5
  - AC-PLATFORM-DELIVERY-RETENTION-006.6
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention-throughput.md
---

# Task 02: Drain productive retention continuously

## Summary and scope

Remove artificial startup, inter-batch and productive inter-pass sleeps from the
existing worker. Wait only after exhausted work, contention or errors. Preserve
eligibility, grace, recovery proofs, all lease limits and maintenance priority.
No UI, schema, new settings or live database replacement is included.

## Acceptance and tests

1. TestDeliveryRetentionWorkerDrainsBeforeWaiting in worker_continuation_test.go
   covers both committed deletion and protected forward discovery across multiple
   bounded passes, followed by exhaustion and idle waiting (.005.5/.005.6/.006.6).
   The old worker fails because it requests a timer before any work.
2. Existing worker tests cover bounded leases, phase fairness, maintenance contention,
   cancellation, restart, corruption backoff and goroutine cleanup (.005.1). Timer
   assumptions are updated; production logs expose continuation_pending (.006.3).
3. Existing small historical sweep and protected-gap SQLite tests preserve repository
   proof coverage. No repository predicate, dialect logic or rendered UI changed;
   initial local verification reused earlier desktop/phone and PostgreSQL evidence.
   Rerun affected scenarios when CI identifies stale fixture assumptions. No million-row CI tests.

## Files likely touched

- apps/backend/internal/task/deliveryretention/worker.go
- apps/backend/internal/task/deliveryretention/worker_continuation_test.go
- apps/backend/internal/task/deliveryretention/worker_productive_test.go and worker lifecycle tests
- apps/backend/internal/task/repository/sqlite/agent_delivery_retention_progress_test.go and agent_delivery_retention_storage_test.go
- apps/web/e2e/tests/session/delivery-retention.spec.ts
- apps/web/e2e/tests/session/mobile-delivery-retention.spec.ts
- docs/public/operations.md and docs/public/configuration.md

## Verification

Run from repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/deliveryretention)
(cd apps/backend && go test -trimpath -race ./internal/task/repository/sqlite -run 'TestDeliveryRetention(IntegrationHistoricalSweep|StorageProtectedGap)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

## Dependencies and parallelism

Task 01 first for shared public operations wording and sequential integration.
No runtime dependency between algorithms. Sequential in the primary session.

## Inputs and risks

[Throughput design](../../specs/platform/system-design/backend-agent-delivery-retention-throughput.md),
existing worker/discovery tests and repository committed-progress semantics.
Zero sleeps must not cause protected wrap or contention to spin. Release the gate at
every batch boundary. Pass budgets are accounting/admission boundaries, not mandated
rest periods. Ordinary reconciliation alone does not delete redundant raw payloads.

## Results

Passed: deliveryretention race suite and the two targeted repository regressions;
changed-package golangci-lint (0 issues); all listed documentation/diff gates. The
new drain-before-wait regression failed on the old initial timer, then passed for
both deletion and forward discovery. Initial verification omitted unchanged browser
and PostgreSQL layers per the user's focused-test instruction. CI remediation follows.

## CI remediation

The independent SQLite holder test still expected the removed startup timer, causing
both the main backend shard and PostgreSQL job to fail on that SQLite case. Remove
obsolete startup-timer advancement from repository worker fixtures. The steady-state
fixture drains startup work before changing its virtual clock; its pass helper now
follows productive continuation until the worker reaches an actual idle timer.
Snapshot waiting retains the prior ten-second bounded-pass guard.

Verification: go test -trimpath -race ./internal/task/repository/sqlite -run
'^TestDeliveryRetention(IntegrationSQLiteIndependentHolder|WorkerRepository|Metrics|StorageSteadyState.*)$'
-count=1 -timeout=4m passed (22.7 seconds before the wait-guard adjustment). Full
backend golangci-lint against the PR base passed with zero issues. Final repeat of
the affected fixtures is recorded below before pushing.

Final affected-fixture race run passed (20.97 seconds); full backend lint passed
again with zero issues. PostgreSQL-specific branches were skipped locally; the
reported CI failure was the SQLite holder case in both jobs.

The recovery browser failures reproduced with retries disabled. The pruned-history
fixture now accepts the two existing uncertain-outcome representations: reconstruction
preserves the empty outcome, while native-resume reconciliation writes `unknown`. It
still requires interrupted-unknown state, zero terminal sequence, exact gap/cursors,
and preserved native identity. The composer readiness predicate checks `contenteditable`
without throwing while the editor is temporarily disabled after cancellation.

Verification: the full pause/resume and desktop pruned-recovery specs passed together
(4 tests, 50.4 seconds), and mobile pruned recovery passed (1 test, 18.8 seconds),
using one worker, retries disabled, and a fresh managed production build. Full backend
lint passed with zero issues. No production behavior or timeout was changed for these
browser fixture corrections.


## Deadline batch-sizing follow-up

This follow-up keeps the bounded-cleanup contract in
AC-PLATFORM-DELIVERY-RETENTION-005.1, 005.5 and 005.6, with the aggregate
failure diagnostics in AC-PLATFORM-DELIVERY-RETENTION-006.3. A repository
deadline reduces only that phase's process-local candidate ceiling by half,
down to one. Parent cancellation and other errors do not reduce it. Configured
and background-event ceilings, byte/time budgets, eligibility proofs, rollback,
failure accounting and retry backoff remain unchanged; a newly constructed
worker starts from the configured ceilings.

The implementation is in apps/backend/internal/task/deliveryretention/worker.go
and worker_batches.go. The existing throughput design and public configuration
and operations wording describe this correction.

The committed worker_batches_test.go cases cover the deadline sequence and
one-row floor, independent phase limits, unrelated errors and parent
cancellation, failed-phase backoff and reset on new worker construction:

- TestDeliveryRetentionWorkerDeadlineBatchLimit
- TestDeliveryRetentionWorkerPhaseBatchLimitsAreIndependent
- TestDeliveryRetentionWorkerBatchLimitIgnoresOtherErrorsAndParentCancellation
- TestDeliveryRetentionWorkerBatchRetryBackoffIncludesFailedPhase
- TestDeliveryRetentionWorkerNewConstructionResetsBatchLimits

The original verification records above remain historical evidence. This
delivery-record correction links the already committed implementation and cases
to their existing requirements and design. It changes no production behavior
and makes no new Go, PostgreSQL, installed-provider or browser execution claim.
Focused worker and repository results and updated-head CI remain separate
results; an environment-gated PostgreSQL skip is not an execution pass.
