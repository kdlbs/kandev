---
id: "02-github-rate-limits"
title: "Stop GitHub cleanup on rate limits"
status: done
wave: 2
depends_on: ["01-github-candidates"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-WATCH-CLEANUP-002
acceptance_criteria:
  - AC-INTEGRATIONS-WATCH-CLEANUP-002.1
  - AC-INTEGRATIONS-WATCH-CLEANUP-002.2
  - AC-INTEGRATIONS-WATCH-CLEANUP-002.3
  - AC-INTEGRATIONS-WATCH-CLEANUP-002.4
system_design:
  - ../../specs/integrations/system-design/watch-task-cleanup.md
---

# Task 02: Stop GitHub cleanup on rate limits

## Summary

Stop GitHub cleanup on rate limits. Implement with TDD and record the regression results.

## In scope

Add `TestCleanupBatchStopsOnRateLimit` in
`service_cleanup_policy_test.go`, with review and issue cases. Include an empty
reservation first, partial deletion counts, wrapped 403/429 errors, unrelated
403 errors, authenticated-user errors, context cancellation, and later retries.
Add `TestCleanupPollCycleSkipsRemainingCleanupAfterRateLimit` in
`poller_test.go`. Cover per-watch and final orphan calls for both poll loops.

Propagate errors through the private deletion gates, batch helpers, and public
cleanup methods in `service_cleanup.go`. Use the existing typed classifier,
including normalization of GitHub CLI stderr rate-limit failures at the
`GHClient` boundary. Thread the resolved automation tracker into batch
admission and stop against the resource used by the resolved client (including
GraphQL for CLI-backed PR and issue reads). Include a stub that exhausts the
tracker while returning successful feedback. Preserve scope: do not consult
another workspace's tracker.

Preserve ordinary row-error continuation and completed counts. In `poller.go`,
stop further cleanup calls for the current cycle after a rate-limit error.
Keep scheduled recovery and discovery behavior intact.

## Out of scope

New policies, UI, schema migrations, and unrelated provider changes.

## Acceptance

- Review and issue batches stop on errors and tracker exhaustion.
- Public cleanup methods preserve partial counts and expose the stop error.
- Poller regressions prove remaining cleanup and orphan calls are suppressed.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test ./internal/github -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/service_cleanup.go`
- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/service_cleanup_policy_test.go`
- `apps/backend/internal/github/service_review_cleanup_test.go`
- `apps/backend/internal/github/poller_test.go`
- `apps/backend/internal/github/poller_cleanup_log_test.go`

## Dependencies

01-github-candidates.

## Risks

Preserve task inventory outside cleanup. Seed actual active tasks in fixtures.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/watch-task-cleanup.md)
- [Design](../../specs/integrations/system-design/watch-task-cleanup.md)
- Existing source and tests listed above.

## Results

Implemented rate-limit and cancellation stop propagation through review and
issue cleanup batches. Batches preserve completed deletion counts, continue
after ordinary row errors, honor the resolved workspace tracker (Core for REST
clients and GraphQL for GHClient), and return recognized 403/429 errors.
GHClient now normalizes CLI rate-limit
stderr into the typed error path, and cleanup admission checks the resolved
client's resource bucket so GraphQL-backed CLI cleanup does not continue after
its quota is exhausted. Poller cycles stop before later watches and the final
orphan sweep after a stop-worthy cleanup error. Added service and poller
regressions for wrapped errors, tracker exhaustion, CLI-shaped rate limits,
cancellation, partial counts, ordinary 403 continuation, and later retry
recovery.
Cleanup admission also reopens automatically once the tracked resource reset
deadline has passed, even when no intervening provider call refreshed the
tracker.

Verification: `go test ./internal/github -count=1` passed.
