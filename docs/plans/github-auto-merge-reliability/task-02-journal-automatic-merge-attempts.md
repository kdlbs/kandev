---
id: "02-journal-automatic-merge-attempts"
title: "Journal automatic merge attempts"
status: done
wave: 2
depends_on:
  - "01-bind-and-pace-asynchronous-merge"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.2
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.3
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.5
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.6
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-002.8
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.1
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.2
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.3
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.7
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.8
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue.md
  - ../../specs/integrations/system-design/github-pr-merge-queue-recovery.md
---
# Task 02: Journal Automatic Merge Attempts

## Summary

Persist the readiness signature, attempted head, result, and error kind before
the provider side effect. Reconcile queue, merge, failure, and restart states.

## In scope

- Add attempt-result and error-kind columns to CI per-PR state.
- Round-trip the fields through every store path.
- Classify only recognized legacy automatic-merge errors during migration.
- Reserve an `in_flight` attempt in a transaction before the provider call.
- Record failed and accepted results.
- Reconcile active queue and merged observations as accepted.
- Clear only typed automatic-merge errors.
- Expire an unreconciled restart attempt to failed without resubmission.
- Preserve same-head removal guards and changed-head requeue behavior.
- Add store, evaluator, failure-injection, concurrency, and restart tests.

## Out of scope

- The HTTP retry command and frontend action.
- New queue-removal classifications.

## Acceptance

- One readiness signature causes no more than one automatic provider request.
- A storage failure causes no provider request and exposes a retryable error.
- Queue or merged state repairs an ambiguous provider result safely.
- Unrelated automation errors survive queue and merged reconciliation.
- A changed eligible head can authorize one later attempt.

## Verification

```bash
go test -tags fts5 ./internal/github ./internal/orchestrator -run 'Test.*(MergeAttempt|AutoMerge|QueueAttempt|MergeSignature)'
go test -race -tags fts5 ./internal/github ./internal/orchestrator -run 'Test.*(MergeAttempt|AutoMerge)'
```

Run the commands from `apps/backend`.

## Files likely touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/service_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_recovery.go`
- Focused store, service, evaluator, and recovery test files.

## Dependencies

- Task 01 provides the head-bound provider request.

## Risks

- Every CI-state insert, update, restore, and replace path must preserve fields.
- An overly broad legacy migration can misclassify an unrelated error.
- Singleflight alone cannot prevent a duplicate after restart.

## Parallelism

`sequential`

## Inputs

- Task 01 provider contract.
- Integration acceptance criteria 002.2, 002.3, 002.5, 002.6, and 002.8.
- Existing queue-recovery acceptance criteria 003.1 through 003.8.

## Results

- Added durable `in_flight`, `failed`, and `accepted` attempt results.
- Reserved each readiness signature before the GitHub side effect.
- Bound completion results to the reserved signature to reject stale results.
- Added typed automatic-merge errors and a narrow legacy backfill.
- Reconciled active queue and merged observations without clearing unrelated errors.
- Expired stale in-flight attempts to failed without automatic resubmission.
- Extended the readiness signature with every merge gate.
- Verification: focused backend tests passed, 32 tests.
- Verification: focused race tests passed, 23 tests.
