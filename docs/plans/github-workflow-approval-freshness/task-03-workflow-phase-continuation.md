---
id: 03-workflow-phase-continuation
title: Resume deferred workflow evidence
status: complete
wave: 3
depends_on:
  - 02-graphql-workflow-phase
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-002
  - REQ-INTEGRATIONS-GITHUB-RATE-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-001.9
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.1
  - AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003.7
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.5
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.6
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
  - AC-INTEGRATIONS-GITHUB-RATE-002.3
  - AC-INTEGRATIONS-GITHUB-RATE-002.7
system_design:
  - ../../specs/integrations/system-design/github-workflow-attention.md
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 03: Resume deferred workflow evidence

## Summary

Carry unfinished workflow-phase work in the PR monitor's existing pending
inventory and resume only that phase at its admission boundary. Prove that, for
a shared principal, the approval badge reaches task summaries before the next
inventory tick.

## In scope

- `poller_pr_continuation.go`: a workflow-phase marker on pending targets.
  `recordPRMonitorPending` accepts unfinished workflow work. A due wake for
  workflow-only work calls the Task 02 phase runner, not the status sync.
- Keep the existing deadline order (future `RetryAt`, then positive `Delay`,
  then the one-second floor). Keep pruning by watch lifetime and credential
  fingerprint, `selectMonitorTickPRWatches` suppression, and stop/restart clearing.
- A target with both unfinished status and workflow work resumes status first.
  Workflow work joins after status completes.
- Update `apps/backend/internal/github/AGENTS.md` continuation notes.

## Out of scope

New schedulers or goroutines, admission policy changes, UI, and REST cache TTLs.

## Acceptance

1. With 30 fork PRs waiting for approval on one principal, real non-blocking
   `RateAdmission` with one-second pacing, and the status batch occupying the
   GraphQL lane: every task's status summary shows `workflow_approval_required`
   before the next one-minute tick. Status GraphQL requests are not repeated by wakes.
2. Removing a watch, changing the head, or rotating credentials while workflow
   work is pending prevents any late write or event. Stop leaves no timer,
   goroutine, or pending entry.
3. A blocked principal does not delay another principal's workflow phase.

## TDD and verification

Run from the repository root. Write the regression first. On Task 02 code it
fails because the deferred phase waits for the next tick.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/github -run '^TestPRMonitorWorkflowAttention_ApprovalVisibleWithinOneMinuteUnderSharedAdmission$' -count=1 -v)
```

Add to `poller_workflow_attention_test.go` (new). Use `testing/synctest`,
SQLite, the real summary projector, and a fake GraphQL executor with call counts:

- `TestPRMonitorWorkflowAttention_ApprovalVisibleWithinOneMinuteUnderSharedAdmission`
- `TestPRMonitorWorkflowAttention_PrunesObsoleteWorkflowWork`
- `TestPRMonitorWorkflowAttention_IndependentPrincipalProgresses`

After the correction, run the backend suite and the rendered regressions:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/github -run 'TestPRMonitorWorkflowAttention_|TestPRMonitorContinuation_|TestWorkflowPhase_|TestPRStatusProgress_|TestPRSidebarDelivery_|Test.*(Poller|PRWatch|RateCoordinator|Batched)' -count=1 -timeout=10m)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/statussummary ./internal/backendapp -count=1)
bash scripts/lint-go-changed $(git diff --name-only -- 'apps/backend/*.go')
(cd apps/web && pnpm e2e:run -- tests/pr/pr-status-badge.spec.ts --project=chromium -g "jobless fork workflow approval")
(cd apps/web && pnpm e2e:run -- tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts --project=mobile-chrome)
git diff --check
```

Do not use real sleeps for retry timing. If the E2E runner's argument form
differs, record the exact command used.

## Files likely touched

- `apps/backend/internal/github/poller_pr_continuation.go`
- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/poller_workflow_attention_test.go` (new)
- `apps/backend/internal/github/AGENTS.md`

## Dependencies

Task 02 supplies the workflow phase runner and its unfinished-work outcome.

## Inputs

- [Monitored workflow phase](../../specs/integrations/system-design/github-workflow-attention.md#monitored-workflow-phase)
- [Admission continuation in the PR monitor](../../specs/integrations/system-design/github-pr-discovery-health.md#admission-continuation-in-the-pr-monitor)
- `pr_sidebar_status_delivery_test.go` and `poller_pr_continuation_test.go` as harness examples.

## Results

Implemented. Pending monitor targets now carry a workflow-only marker.
`recordPRMonitorPending` accepts unfinished workflow work, and
`resumePRMonitorPending` sends a due workflow-only target to the Task 02 phase
without re-fetching status. A status consumer always wins the phase marker, so
status resumes first and the workflow phase joins its completed pass. Pruning,
credential fences, and stop semantics are unchanged.

Added the requested regressions: approval visible before the next one-minute
tick under a shared principal with real `RateAdmission` and the real summary
projector, obsolete workflow work pruned after a removed watch, head rotation,
or credential rotation, and an independent principal progressing while another
is deferred.
The E2E mock GitHub client has no GraphQL executor, so the unchanged badge
remains covered by the existing REST-rendered Playwright specs. Both rendered
regressions passed:

- `pnpm e2e:run -- tests/pr/pr-status-badge.spec.ts --project=chromium -g "jobless fork workflow approval"`
  (1 passed)
- `pnpm e2e:run -- tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts --project=mobile-chrome`
  (4 passed)

Review follow-ups: a GraphQL rate-limit response now retains its
`rateLimit.resetAt` boundary on the returned error, so the monitor's
`prMonitorPendingBoundary` path can use it instead of dropping the provider
deadline. A completed known-PR status refreshes workflow attention even when a
sibling searching phase defers, and workflow pending targets keep their own
short retry boundary instead of inheriting the searching phase's deadline.

Verification after the review follow-ups:

- Full `internal/github` package passed; the Task 03 race command passed.
- `go test -trimpath -tags fts5 ./internal/task/statussummary` passed, and the
  GitHub/PR `internal/backendapp` tests passed.
- Both Playwright specs passed again (desktop 1 passed, mobile 4 passed).
- `bash scripts/lint-go-changed` reported zero issues.
