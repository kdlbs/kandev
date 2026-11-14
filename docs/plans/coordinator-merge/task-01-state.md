---
id: "01-state"
title: "State and wakes"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-MERGE-001
acceptance_criteria:
  - AC-COORDINATOR-MERGE-001.1
  - AC-COORDINATOR-MERGE-001.2
  - AC-COORDINATOR-MERGE-001.3
system_design:
  - ../../specs/coordinator/system-design/merge.md
---

# Task 01-state: State and wakes

## Summary

Change observer (reuse PR/MR status sync, 60s freshness, backoff), normalized readings, condition rows with generation and last_observed_at, wake kinds change_red/change_conflict/change_threads, Queue and turn-context projection (backend). Register stores in requiredstores and storeconformance; SQLite and env-gated PostgreSQL tests.

## In scope

- Deliver the listed acceptance criteria with the named tests, against the paired design.

## Out of scope

- Releases, branch pushes, provider bypass and the parallel answer proposal.

## Acceptance

- AC-001.1: TestChangeReadingsNormalization (no pipeline, pending, cancelled, skipped, multiple suites), TestUnavailableNeverPassing, TestObservationReuseUnder60s.
- AC-001.2: TestWakeEpisodeRecurrence, TestStaleObservationDiscarded (by read start), TestThreadsWakeOnZeroToPositive, TestExistingThreadsNoWake, TestUnavailableNeverClears, TestHeadChangeClearsRed, TestBaselineNeverWakes, TestWakeFanOutToEveryAutonomyCoordinator, TestBackstopDerivesMissingWakes (cap, late autonomy), restart/backstop convergence, PostgreSQL concurrent transition.
- AC-001.1 additions: TestGitHubCommitStatusAndRequiredContexts, TestFailedReadNotCachedRetryBypasses, TestSingleFlightRead, TestNoChecksLabel.
- AC-001.3: TestChangeOrderingByProviderRepoNumber.

## Verification

```bash
(cd apps/backend && go test -race ./internal/coordinator/... ./internal/github/... ./internal/gitlab/... ./internal/persistence/storeconformance)
```

## Files likely touched

- apps/backend/internal/coordinator; internal/github; internal/gitlab; internal/persistence/requiredstores

## Dependencies

Answer-proposals slice.

## Risks

Provider reads may fail or race; preserve the fail-closed and uncertain-outcome rules.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/coordinator/requirements/merge.md) and [design](../../specs/coordinator/system-design/merge.md).
- Existing coordinator and provider code and tests.

## Results

Pending.
