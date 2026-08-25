---
id: "02-evaluate-repair-and-requeue"
title: "Evaluate repair and requeue"
status: done
wave: 2
depends_on:
  - "01-persist-queue-recovery-evidence"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.1
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.2
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.3
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.4
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.5
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.6
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.7
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.8
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.1
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.2
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.3
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.4
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.5
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.6
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.7
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-003.8
system_design:
  - ../../specs/integrations/system-design/github-pr-merge-queue-recovery.md
---
# Task 02: Evaluate Repair and Requeue

## Summary

Extend CI automation with queue-removal checkpoints and failed-check evidence.
Use the PR head SHA to permit one safe queue attempt after a repair.

## In scope

- Map reviewed removal-reason forms to normalized causes.
- Keep `beforeCommit` separate from an exact merge-group commit identity.
- Add queue-removal evidence to the auto-fix checkpoint and prompt snapshot.
- Record one accepted auto-fix round per actionable removal event.
- Add the head SHA to the merge signature.
- Store the last accepted or observed queue-attempt head.
- Baseline Kandev-created and externally observed queue attempts.
- Adopt an active attempt when auto-merge is enabled while already queued.
- Evaluate one retained actionable removal when auto-fix becomes enabled.
- Prevent a same-head automatic requeue.
- Add focused service and orchestrator tests.

## Out of scope

- A new queue preference.
- A same-head flaky-check retry policy.
- UI and E2E changes.

## Acceptance

- One actionable removal produces at most one auto-fix round.
- A manual or unknown removal without evidence produces no repair prompt.
- The removal event commit does not produce merge-group check evidence.
- A new eligible head produces one queue-aware merge request.
- Enabling auto-merge on an active entry produces no duplicate request.

## Verification

```bash
go test -tags fts5 ./internal/github ./internal/orchestrator -run 'Test.*CIAutomation.*MergeQueueRecovery'
```

Run the command from `apps/backend`.

## Files likely touched

- `apps/backend/internal/github/service_ci_automation.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
- `apps/backend/internal/github/service_ci_automation_test.go`

## Dependencies

- Task 01 supplies the durable provider snapshot.

## Risks

- GitHub can change its free-form removal reasons. Unknown forms must fail
  closed and remain observable.
- Prompt text must preserve the existing untrusted-data boundary.

## Parallelism

`sequential`

## Inputs

- Integration requirements 002 and 003.
- Existing CI checkpoint, queue dispatch, round-limit, and merge-signature code.

## Results

- Added reviewed queue-removal cause classification with fail-closed handling
  for manual, branch-protection, and unknown provider reasons. Merge conflict
  and `UNMERGEABLE` evidence remain actionable without treating `beforeCommit`
  as a merge-group check identity.
- Added queue-removal checkpoint and prompt evidence, durable event deduplication,
  round-limit accounting, and current-head eligibility checks.
- Added durable active-queue adoption and attempted-head persistence. The merge
  signature now includes `TaskPR.HeadSHA`; active entries are adopted, removed
  heads are blocked, and a changed eligible head can be queued once.
- Added focused store, service wiring, and orchestrator coverage for the
  recovery state machine.
- `go test -tags fts5 ./internal/github ./internal/orchestrator -run 'Test.*CIAutomation.*MergeQueueRecovery'` passed (13 tests).
- `go test -tags fts5 ./internal/github` passed (1,661 tests).
- `go test -tags fts5 ./internal/orchestrator` passed.
- `git diff --check` and targeted `gofmt -l` checks passed.
