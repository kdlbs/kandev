---
id: "02-cleanup-backoff"
title: "Bound eligible cleanup requests"
status: pending
wave: 2
depends_on:
  - "01-archive-eligibility"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-BACKOFF-001
  - REQ-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-BACKOFF-001.1
  - AC-PLATFORM-PROVIDER-BACKOFF-001.2
  - AC-PLATFORM-PROVIDER-BACKOFF-001.3
  - AC-PLATFORM-PROVIDER-BACKOFF-001.4
  - AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.1
  - AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.3
system_design:
  - ../../specs/platform/system-design/pr-watch-and-bounded-storage.md
  - ../../specs/integrations/system-design/github-review-task-cleanup.md
---

# Task 02: Bound eligible cleanup requests

## Summary

Scheduled feedback obeys Core quota and classified backoff. A provider
failure stops later requests in its scope, while healthy workspaces and
explicit user actions retain their existing behavior.

## In scope

- Add a review-cleanup circuit distinct from the PR-monitor circuit.
- Report feedback failures to the poller without changing public cleanup
  response signatures or using a successful search as cleanup success.
- Stop the affected workspace batch after shared auth or rate failure.
  Isolate record-specific configuration failures.
- Check Core exhaustion before scheduled feedback. Apply credential and watch
  fingerprint resets and bounded-label metrics.

## Out of scope

- Frontend status or settings changes, credential storage, and manual cleanup
  admission.
- New general-purpose rate limiter or changes to task-owned PR-watch polling.

## Acceptance

1. An auth or rate-limit failure on the first eligible row prevents later
   feedback calls in that workspace during the cycle. An open circuit makes
   zero calls until a due probe or matching fingerprint change.
2. A PR-specific configuration failure does not block a healthy sibling.
   A healthy search or PR-monitor call does not clear the cleanup circuit.
   An exhausted Core snapshot prevents scheduled feedback.
3. Separate workspace credentials keep separate circuits. Metrics expose
   skips, resets, and failure classes without identifiers. Manual cleanup
   retains its current path.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/github -run 'Test(ReviewCleanupCircuit|ReviewCleanupCoreQuota|ReviewCleanupFailureScope|ReviewCleanupManualBypass|PollerCircuits|ClassifyPollErr|ReviewCleanupMetrics)' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/github -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/poller_circuit.go`
- `apps/backend/internal/github/service_cleanup.go`
- `apps/backend/internal/github/metrics_vars.go`
- `apps/backend/internal/github/poller_circuit_test.go`
- `apps/backend/internal/github/service_cleanup_policy_test.go`
- `apps/backend/internal/github/metrics_vars_test.go`

## Dependencies

Task 01 must supply the scheduled-only cleanup path and its eligible-row tests.

## Risks

The current cleanup helper swallows feedback errors. Preserve manual response
behavior while exposing a typed poller outcome. Do not let one record's 404
block every PR in its workspace.

## Parallelism

`sequential`

## Inputs

- [Platform requirement](../../specs/platform/requirements/pr-watch-and-bounded-storage.md)
- [Platform design](../../specs/platform/system-design/pr-watch-and-bounded-storage.md)
- [Review cleanup design](../../specs/integrations/system-design/github-review-task-cleanup.md)
- Existing `authcircuit`, Core rate tracker, and PR-monitor circuit tests.

## Results

Pending.
