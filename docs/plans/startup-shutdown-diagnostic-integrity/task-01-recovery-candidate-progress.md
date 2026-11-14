---
id: "01-recovery-candidate-progress"
title: "Count resolved recovery candidates"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-STARTUP-PROGRESS-001
  - REQ-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001
acceptance_criteria:
  - AC-PLATFORM-STARTUP-PROGRESS-001.3
  - AC-PLATFORM-STARTUP-PROGRESS-001.21
  - AC-PLATFORM-STARTUP-PROGRESS-001.22
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.4
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.5
  - AC-PLATFORM-RUNTIME-FAILURE-ATTRIBUTION-001.7
system_design:
  - ../../specs/platform/system-design/startup-progress-visibility.md
  - ../../specs/platform/system-design/runtime-failure-attribution.md
---

# Task 01: Count resolved recovery candidates

## Summary

Advance recovery progress for each conclusively resolved candidate, including successful enumeration that proves no matching instance.
Keep retracking success and unresolved work separate.

## In scope

- Recheck current main and the issue checkbox before implementation. Reuse any separately landed correction and retain only missing coverage.
- Add a completion ledger over exact filtered inventory candidates in `Manager.Start`.
- Use existing detailed outcomes after conclusive inspection, and completed reconstruction/refusal after existing guard work settles.
- Consolidate progress advancement through that ledger. Duplicate or uncorrelated returned instances cannot add progress.
- Leave failed enumeration, unknown omissions, incomplete deadline/cancellation work, and unreadable inventory truthful.
- Preserve completion after owned stop workers finish and keep zero-inventory behavior.

## Out of scope

- Changing provider recovery support, identity guards, stop decisions, time budgets, persistence, or reporter warning behavior.
- Treating successful empty agentctl inventory as proof that an OS process died.

## Acceptance

- One confirmed absent input changes from done=0,total=1 to done=1,total=1, with retracked_count=0 and no below-total warning.
- Mixed recovered, absent, and conclusively refused candidates each contribute once. Duplicate results do not exceed the input total.
- Unknown, failed, canceled, and unfinished deadline cases retain incomplete progress and existing guards. Step closure still waits for stop workers.

## Regression evidence

Add `TestRecoveryProgressCountsConfirmedAbsence`, `TestRecoveryProgressMixedResolvedCandidates`, and `TestRecoveryProgressRetainsIncompleteOutcomes`.
Include duplicate results, unrelated results, partial enumeration, zero inventory, unknown inventory, and repeated passes.
The first test must fail on the recorded base because progress remains 0/1.
Use `newStandaloneControlServer`, the real standalone executor, the manager, and the startup reporter.

## Verification

Run the first regression before the correction and record its expected failure.
After implementation, run this block from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle ./internal/startup -run 'RecoveryProgress|RecoverySummary|RecoverySessionsStep|KeepsRecoveryStep|CountsBackendOmissions|SeparatesRetracked|RecoveryGuard' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go apps/backend/internal/agent/runtime/lifecycle/manager_recovery_progress.go apps/backend/internal/agent/runtime/lifecycle/manager_recovery_progress_test.go
git diff --check
```

The new progress helper keeps the already-large lifecycle method within local lint limits.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_recovery_progress.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/manager_recovery_progress_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/manager_recovery_sessions_step_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_recovery_outcomes_test.go`

## Dependencies

None. The existing detailed recovery interface is already present.

## Risks

Detailed producer evidence can be provisional for a returned candidate.
Advance only after its final classification and required side effects settle.
A session correlation is not permission to count an unrelated input row.

## Parallelism

`sequential`

## Inputs

- [Plan evidence and compatibility matrix](plan.md).
- [Startup requirement](../../specs/platform/requirements/startup-progress-visibility.md).
- [Attribution requirement](../../specs/platform/requirements/runtime-failure-attribution.md).
- Referenced designs, particularly session recovery and enumeration outcomes.
- Existing `manager_recovery_sessions_step_test.go` and `manager_recovery_outcomes_test.go` fixtures.

## Results

- Added a per-pass recovery completion ledger over the exact filtered input candidates. Each resolution publishes its newly completed rows immediately, so live snapshots advance while later candidates and stop workers are still running.
- Confirmed standalone absence and conclusive reconstruction refusals now advance once. Remote begin/transport failures, operational environment reads, durable evidence failures, settings/replay failures, and deadline or cancellation outcomes remain incomplete.
- Added `TestRecoveryProgressCountsConfirmedAbsence`, `TestRecoveryProgressMixedResolvedCandidates`, `TestRecoveryProgressRetainsIncompleteOutcomes`, `TestRecoveryProgressPublishesBeforeStopWorkersFinish`, `TestRecoveryProgressRetainsOperationalReconstructionFailures`, and a duplicate-result regression.
- Verification passed: focused lifecycle tests, lint, and `git diff --check`.
