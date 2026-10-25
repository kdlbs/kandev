---
id: "01-correct-auto-fix-signals"
title: "Correct auto-fix signals"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001
  - REQ-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.1
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.2
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.3
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.4
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.5
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.6
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.7
  - AC-INTEGRATIONS-GITHUB-PR-AUTO-FIX-CONFLICTS-001.8
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.1
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.3
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.7
  - AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-RECOVERY-002.9
system_design:
  - ../../specs/integrations/system-design/github-pr-auto-fix-conflicts.md
  - ../../specs/integrations/system-design/github-pr-merge-queue-recovery.md
---
# Task 01: Correct Auto-Fix Signals

## Summary

Add ordinary merge conflicts to the auto-fix checkpoint and prompt. Let an
actionable failed-check queue removal use durable attempted or adopted queue
evidence, and fail closed when its head provenance is unavailable.

## In scope

- Add an additive conflict snapshot to `ciAutomationCheckpoint`.
- Build current and delta conflict state from `TaskPR`.
- Render sanitized conflict context when `{{pr.feedback}}` is present.
- Clear resolved conflict state without using a round.
- Re-arm a changed or recurring conflict.
- Require a matching queue-attempt head and merge signature for queue-removal
  auto-fix eligibility.
- Add focused orchestrator regression tests with TDD.

## Out of scope

- UI copy, public documentation, or E2E tests.
- Queue-attempt persistence changes.
- Auto-merge retry behavior.
- Database schema changes.

## Acceptance

- One settled `dirty` snapshot produces one auto-fix round with conflict
  context. An unchanged snapshot produces no duplicate.
- An authoritative resolution clears the conflict checkpoint without a round.
  Unknown state preserves deduplication. A recurring or changed conflict can
  produce one new round.
- One retained `checks_failed` removal with matching durable queue-attempt
  evidence produces one round. A removal first observed after a head change
  without that evidence produces no prompt or round. Its event ID still
  deduplicates later polls.

## Verification

Run from `apps/backend`:

```bash
go test -tags fts5 ./internal/orchestrator -run 'Test(CIAutomationConflictSignal|HandleTaskPRCIAutomationAutoFixesConflict|CIAutomationMergeQueueRecoveryRequiresAttemptEvidence|CIAutomationMergeQueueRecoveryQueuesOneRepairPerRemoval)$'
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_merge_queue_recovery_test.go`

## Dependencies

None.

## Risks

- Checkpoint refresh must remove resolved conflict state without erasing active
  failed-check or comment state.
- Combined conflict and queue-removal feedback must use one dispatch and round.

## Parallelism

`sequential`

## Inputs

- Both referenced integration designs.
- Existing feedback delta, checkpoint refresh, prompt renderer, and
  queue-removal tests.

## Results

Implemented additive conflict checkpoint and prompt rendering, authoritative
conflict clearing with unknown-state preservation, conflict deduplication and
re-arming, durable session pinning across checkpoint refreshes, and fail-closed
queue-removal provenance. The focused Go command passed with 8 tests, and the
conflict rendering regression passed separately.
