---
id: "02-git-capture-recovery"
title: "Recover transient Git capture races"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GIT-CAPTURE-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.1
  - AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.2
  - AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.3
  - AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.4
  - AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.5
system_design:
  - ../../specs/platform/system-design/workspace-git-capture-recovery.md
---

# Task 02: Recover transient Git capture races

## Summary

Recover a brief basic capture race inside the existing shared observation. Deliver only validated enriched state to the baseline consumer.

## In scope

- One typed evidence-change retry in the tracker-owned basic capture.
- Existing deadlines, admission classes, singleflight sharing, temporary index cleanup, and publication guards.
- Expected-race log classification in API/tracker boundaries and real enriched baseline integration.

## Out of scope

- New Git endpoints, metadata-only baselines, launch retry loops, Git mutations, and frontend layout or recovery changes.

## Acceptance

- A deterministic first-attempt mutation recovers on the second attempt. Continuous mutation performs at most two basic captures and returns unavailable.
- Concurrent waiters share correction. Caller cancellation, deadline, shutdown, identity replacement, and unrelated failures preserve the documented invariants.
- A public status request and launch baseline consumer accept the recovered enriched result. Unavailable detail never establishes a false baseline.

## Verification

Run this block from the repository root after the implementation result exists.
New test names below are required planned regressions, not claims of existing coverage.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'Test.*(GitStatus|CaptureRecovery)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/orchestrator/executor -run 'Test.*BaseCaptureRecovery' -count=1)
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_capture.go`
- `apps/backend/internal/agentctl/server/process/workspace_git_status_capture_recovery_test.go (new)`
- `apps/backend/internal/agentctl/server/api/git.go`
- `apps/backend/internal/agentctl/server/api/git_capture_recovery_test.go (new)`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_base_capture_recovery_test.go (new)`

## Dependencies

None. Follow the plan's sequential priority order.

## Risks

- Retry must not allocate a fresh deadline or reset the enrichment correction policy.
- Shared work cannot inherit a canceled caller context. Failed index snapshots must close before another capture.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), especially evidence, contract ownership, and completion rules.
- [Design](../../specs/platform/system-design/workspace-git-capture-recovery.md) and its linked requirements.

- Scoped backend/agentctl instructions for any touched package.
- Existing source and tests listed above. Preserve completed companion-package results.

## Results

Implemented the one-time typed evidence-change correction within the existing shared context and admission class. Failed index snapshots are disposed by the capture before retry. Continuous churn remains unavailable; unrelated command errors, missing repositories, deadlines, and tracker shutdown do not start another capture. Concurrent waiters share correction, and one canceled waiter does not cancel another. The API reports recovered enriched status and emits one bounded warning after exhausted churn. Launch baseline capture now requires a successful complete enriched status before it can use BaseCommit or the detached-HEAD fallback.

Verification passed:

- `go test -trimpath -tags fts5 -race ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'Test.*(GitStatus|CaptureRecovery)' -count=1`
- `go test -trimpath -tags fts5 -race ./internal/orchestrator/executor -run 'Test.*BaseCaptureRecovery' -count=1`

Existing public Git documentation was reviewed. The status payload and user recovery flow are unchanged, so no public documentation update was needed.
