---
id: 02-admission-continuation
title: Resume deferred PR monitoring
status: done
wave: 2
depends_on:
  - 01-status-progress
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-001
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-002
  - REQ-INTEGRATIONS-GITHUB-RATE-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.1
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.3
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.5
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.1
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.2
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.3
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.4
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.5
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.6
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
  - AC-INTEGRATIONS-GITHUB-RATE-002.2
  - AC-INTEGRATIONS-GITHUB-RATE-002.3
  - AC-INTEGRATIONS-GITHUB-RATE-002.7
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 02: Resume deferred PR monitoring

## Summary

Resume unfinished PR monitoring at its admission boundary through the existing
monitor loop. Retain one-minute inventory discovery and independent workspace progress.

## In scope

- Add a bounded pending inventory and one cancelable timer under `Poller` ownership.
- Retain unfinished phases across normal ticks and deduplicate equivalent targets.
- Use future `RetryAt`, then positive `Delay`, then the one-second undated retry floor.
- Recheck current admission, authorization, credentials, and active watch lifetime.
- Keep local deferral outside fallback and provider-failure circuits.
- Preserve rotating workspace/target order and 5/10 fallback budgets across timer wakes.
- Clear pending state on completion, pruning, stop, and restart.

## Out of scope

No new scheduler framework, coordinator policy, per-watch goroutine, schema,
UI, global quota gate, or general REST continuation is part of this task.

## Acceptance

1. A one-second deferral resumes unfinished work before the next minute.
   Longer provider boundaries remain intact, and repeated deferrals do not busy-loop.
2. A blocked principal leaves another eligible principal able to progress.
   Timer wakes do not repeat completed requests or replenish fallback budgets.
3. Removal, retirement, detach, changed credentials, explicit refresh, and shutdown
   invalidate obsolete work. No late write/event crosses those boundaries.

## TDD and verification

Run from the repository root. First add the timer regression and record its
failure before production edits. Current code waits until the next minute.

```bash
(cd apps/backend && go test -tags fts5 ./internal/github -run '^TestPRMonitorContinuation_ResumesAtAdmissionBoundary$' -count=1 -v)
```

Use virtual time and real admission. Assert provider-call boundaries and durable
progress before the next inventory tick. Include an undated busy deferral,
a provider retry longer than one minute, and repeated unavailable admission.

Add these focused regressions in `poller_pr_continuation_test.go`:

- `TestPRMonitorContinuation_LocalDeferralSkipsFallback`: typed deferral survives
  wrapping, produces no REST fallback, and leaves authentication circuits unchanged.
- `TestPRMonitorContinuation_PreservesFairnessAndBudgets`: mix two principals,
  shared-principal workspaces, unsupported GraphQL, transport failure, and successful groups.
  Count completed and fallback target calls across normal ticks and timer wakes.
- `TestPRMonitorContinuation_PrunesObsoleteWorkAndStops`: remove or retire a held
  target, rotate credentials, supersede with explicit refresh, and stop/restart.
  Assert no obsolete application, leaked timer, goroutine, or retained entry.

After the correction, run:

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/github -run 'TestPRStatusProgress_|TestPRMonitorContinuation_|Test.*(Poller|PRWatch|PRMonitoring|RateCoordinator|Batched|PRDiscovery)' -count=1 -timeout=8m)
git diff --check
```

Do not use real sleeps for retry timing. Reuse fixed clocks or `testing/synctest`.
Record the exact resulting command if focused test names differ.

## Files likely touched

- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/poller_pr_continuation.go` (new, if extraction is needed)
- `apps/backend/internal/github/poller_pr_continuation_test.go` (new)
- `apps/backend/internal/github/poller_schedule_test.go`
- `apps/backend/internal/github/poller_circuit_test.go`

`service.go` or batch lifecycle cleanup can change only if pending ownership
needs service invalidation. Preserve the public service and coordinator contracts.

## Dependencies

Task 01 supplies applied results, unfinished groups, and typed deferral evidence.

## Inputs

- [Discovery requirements](../../specs/integrations/requirements/github-pr-discovery-health.md).
- [Admission continuation design](../../specs/integrations/system-design/github-pr-discovery-health.md#admission-continuation-in-the-pr-monitor).
- [Principal admission requirements](../../specs/integrations/requirements/github-rate-limit-coordination.md#req-integrations-github-rate-002-principal-wide-provider-admission).
- [Rate coordination design](../../specs/integrations/system-design/github-rate-limit-coordination.md).
- `poller_watch_queue.go` and `watch_poll_progress.go` as scoped progress examples.
- `rate_coordinator.go:AdmissionDeferredError` and existing poller lifecycle tests.

## Risks

Do not turn a short local delay into an authentication backoff. A timer wake is
an opportunity to request admission, not permission to bypass it. Shared quotas
must not become shared authorization or result caches.

## Parallelism

`sequential`

## Results

Done on 2026-10-28.

RED (base commit `4222c3654f9b`, temporary worktree):

- `(cd apps/backend && go test -tags fts5 ./internal/github -run
  '^TestPRMonitorContinuation_ResumesAtAdmissionBoundary$' -count=1 -v)` failed
  with `the deferred phase resumes at its admission boundary: expected 1,
  actual 0` after two virtual seconds, proving the base build waits for the next
  one-minute inventory tick.

Production change:

- `poller_pr_continuation.go` adds a bounded, target-keyed pending inventory and
  one reusable timer owned by `prMonitorLoop`. Deadlines prefer a future
  `RetryAt`, then a positive `Delay`, then the one-second floor; provider
  boundaries stay lower bounds.
- Pending entries are pruned against each active inventory, are skipped by
  fresh ticks until due, resume in rotating workspace order, recheck open
  circuits, and never run the transport fallback, so a wake cannot replenish a
  cycle's fallback budget.
- `Stop` and loop cancellation clear the inventory; a restart rebuilds work from
  the normal initial pass. Current host, credential, lifetime, and
  explicit-refresh fences remain the authority at apply time.

GREEN:

- `(cd apps/backend && go test -tags fts5 -trimpath -race ./internal/github
  -run 'TestPRStatusProgress_|TestPRMonitorContinuation_|Test.*(Poller|PRWatch|PRMonitoring|RateCoordinator|Batched|PRDiscovery)'
  -count=1 -timeout=15m)`: ok, 11.9s.
- `(cd apps/backend && go test -tags fts5 -trimpath ./internal/github -count=1
  -timeout=25m)`: ok, 105.7s.
- `bash scripts/lint-go-changed <the changed Go paths listed in the plan>`:
  0 issues.
- `git diff --check`: passed.

New coverage in `poller_pr_continuation_test.go`:
`TestPRMonitorContinuation_ResumesAtAdmissionBoundary`,
`TestPRMonitorContinuation_LocalDeferralSkipsFallback`,
`TestPRMonitorContinuation_PreservesFairnessAndBudgets`,
`TestPRMonitorContinuation_PrunesObsoleteWorkAndStops`,
`TestPRMonitorContinuation_RepeatedDeferralRearmsForward`,
`TestPRMonitorContinuation_HonorsExtendedProviderDeadline`,
`TestPRMonitorContinuation_ResumesEverySharedTargetConsumer`,
`TestPRMonitorContinuation_CredentialReplacementDropsStaleBoundary`, and
`TestPRMonitorContinuation_ResumeLifetimeDropsStaleSuppression`.

Review remediation (P1 repeated-deferral spin, P2 shared-target consumers,
P2 credential/lifetime fencing) is recorded in the plan and covered by the last
four tests above. Pending work is now keyed by provider target with a consumer
set and a per-target deadline; pruning compares the workspace credential
fingerprint and live watch IDs.

