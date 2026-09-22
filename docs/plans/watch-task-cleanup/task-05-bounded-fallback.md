---
id: "05-bounded-fallback"
title: "Bound per-workspace PR fallback"
status: done
wave: 5
depends_on: ["04-minimal-cleanup"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.1
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.2
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.3
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.4
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
---

# Task 05: Bound per-workspace PR fallback

## Summary

Bound per-workspace PR fallback. Implement with TDD and preserve workspace identity.

## In scope

Replace the all-workspace boolean result from tryBatchedPRWatchCheck with
per-workspace outcomes. Retain auth, rate-limit, and invalid-query suppression.
Publish successful workspace results even when another workspace fails.

Permit at most 5 fallback targets per workspace and 10 per cycle globally.
Rotate both workspaces and targets across cycles. Unsupported GraphQL clients
use the same budget. Stop the affected workspace on auth or rate-limit failure.
Do not mark deferred work as checked or consume admission permits indefinitely.

Add TestPRWatchFallbackBudgetAndFairness to poller_test.go. Cover multiple
workspaces, successful earlier batches, unsupported clients, transient errors,
rate limits during fallback, cancellation, event publication, and fair rotation.
Retain TestCheckPRWatches_BatchedAuthFailureSkipsPerWatchFallback.

## Out of scope

New UI controls, credential policies, global schedulers, and unrelated providers.

## Acceptance

- Meet every linked acceptance criterion with deterministic regressions.
- Preserve existing policy, scope, and error-handling contracts.
- Record the expected red test result and passing package results.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test ./internal/github -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/poller_test.go`
- `apps/backend/internal/github/poller_circuit_test.go`
- `apps/backend/internal/github/service_pr_watch_batched.go`

## Dependencies

04-minimal-cleanup.

## Risks

A target can cost several HTTP requests. The budget limits targets, not pagination.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-discovery-health.md)
- [Design](../../specs/integrations/system-design/github-pr-discovery-health.md)
- Source files and existing adjacent tests listed above. New helper/test files are created as needed.

## Results

Implemented per-workspace PR batch outcomes and bounded REST fallback. A
successful workspace publishes its results immediately and is never replayed
through REST when another workspace fails. Authentication, rate-limit, and
invalid-query outcomes remain deferred; transient failures and clients without
GraphQL support use a rotating round-robin fallback with a five-target
workspace budget and ten-target cycle budget. Fallback stops the affected
workspace on rate-limit or authentication errors and stops the cycle on
cancellation. The passive workspace refresh path applies the same five-target
workspace and ten-target scheduling-window budgets with fair target rotation;
it classifies batch failures before fallback and runs fallback checks
sequentially so an authentication or rate-limit error leaves no unscheduled
tail of calls for that workspace.
Equivalent workspace targets are deduplicated before workspace and global
budgets are admitted, and one fallback result is applied to every watch in the
target group.

Added deterministic coverage for mixed workspaces, successful event
publication, unsupported and transient clients, target rotation, fallback
budgets, rate-limit stopping, and cancellation. Verification passed:

```bash
(cd apps/backend && go test ./internal/github -count=1)
```
