---
id: 04-watch-progress
title: Resume background watch polling
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-002
  - REQ-INTEGRATIONS-GITHUB-RATE-003
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-RATE-002.2
  - AC-INTEGRATIONS-GITHUB-RATE-002.3
  - AC-INTEGRATIONS-GITHUB-RATE-002.7
  - AC-INTEGRATIONS-GITHUB-RATE-003.5
  - AC-INTEGRATIONS-GITHUB-RATE-003.7
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 04: Resume background watch polling

## Summary

Resume deferred issue/review filters and rotate eligible watches. Preserve
completed discoveries without blocking the global poller on one principal.

## Confirmed root cause

Both repository-filter loops swallow admission deferrals and restart from
filter zero next tick. Each successful first request establishes pacing again.
The outer watch loops also iterate in a stable order, so cursor repair must
cover competing watches as well as filters within one watch.

## In scope

Add `TestWatchPollingEventuallyReadsEveryFilter` for both watch types, with real
PAT admission and a fake HTTP provider under `testing/synctest`. Assert exact
provider requests and published discoveries across repeated cycles. Add a mixed
fixture with several watches sharing a principal and another independently
eligible principal. Include overlapping issue/review polling, terminal provider
errors, an interactive waiter, reserve exhaustion/reopening, changed configuration
and credential scope, disabled/deleted watches, and stop/start. Preserve orphan
cleanup even when all searches defer. Use small cursor state and fresh authorization
on resume. Do not add per-watch goroutines or waits inside the polling loop.
Use service/batch progress and poller cursor code as lifecycle references.

## Out of scope

New UI, persistence changes, external publication, and unrelated CI failures.
Public integration guidance describes resumed polling and identity isolation.

## Acceptance

- Every enabled watch and filter progresses under recurring capacity, including multiple principals and mixed deferred/successful filters.
- Deferral preserves completed discoveries, respects reserve/interactive priority, and cannot bypass current workspace scope.
- Configuration/credential changes, disable/delete, manual checks, cancellation, and restart cannot reuse stale progress or duplicate tasks.

## Verification

Run from the repository root. First add the named regression and record its
expected failure. Apply the correction, then run:

```bash
(cd apps/backend && go test -trimpath -race ./internal/github -run 'Test.*(WatchPolling|WatchProgress|Issue|Review|Poller|Admission|Coordinator|Batched)' -count=1 -timeout=5m)
(cd apps/backend && go test -trimpath -race ./internal/github ./internal/workflowsync -count=1 -timeout=10m)
(cd apps/backend && /tmp/pr3143-tools/golangci-lint run ./internal/github/... ./internal/workflowsync/... --new-from-rev=00fce40d68869ed476f4602962ad2d61f32efa76 --timeout=5m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/poller_watch_queue.go`
- `apps/backend/internal/github/service.go`
- `apps/backend/internal/github/service_issues.go`
- `apps/backend/internal/github/service_reviews.go`
- `apps/backend/internal/github/watch_poll_progress.go`
- `apps/backend/internal/github/poller_watch_progress_test.go`
- `apps/backend/internal/github/watch_poll_lifecycle_test.go`
- `apps/backend/internal/github/watch_poll_delivery_test.go`
- `apps/backend/internal/github/service_issues_poll_test.go`
- `apps/backend/internal/github/service_reviews_test.go`

## Dependencies

No implementation dependency. Follow the plan order in this single session.

## Risks

Preserve the existing admission, workspace identity, and cancellation contracts.
Use bounded deterministic fixtures and retain provider-call assertions.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-rate-limit-coordination.md).
- [System design](../../specs/integrations/system-design/github-rate-limit-coordination.md).
- [Plan evidence and compatible paths](plan.md).
- ADR 0047 and ADR-2026-08-29-github-provider-rate-coordination.

## Results

RED: issue/review sweeps repeated the first repository and stamped incomplete
polls as complete. Stable watch order starved later watches, and independent
issue/review loops could starve one entire watch kind.

Implemented scoped filter cursors and one five-minute discovery loop with a
rotating combined inventory. Completed discoveries retain the existing event
and reservation path. Each resumed pass resolves workspace authorization,
credentials, and repository scope. Manual reads use their own sweep. Disabled
or deleted watches are pruned, and stop/start clears progress.

Passed before the final integration run:

- Specified focused watch/admission race command: 39.194s.
- `go test -trimpath -race ./internal/github -run 'TestWatch(Polling|Progress)' -count=1 -timeout=3m`: 7.082s.
- CI-pinned golangci-lint v2.9.0: zero issues in the affected packages. The executable is checksum-verified in `/tmp/pr3143-tools`.
- Specification catalog, specification lint, public docs validation, and `git diff --check` passed.

Final verification passed on 2026-10-21:

- `go test -trimpath -race ./internal/github ./internal/workflowsync -count=1 -timeout=10m`: GitHub 271.518s, Workflow Sync 1.302s.
- The full run includes every added or changed regression after the final production edit.
- `/tmp/pr3143-tools/golangci-lint run ./internal/github/... ./internal/workflowsync/... --new-from-rev=00fce40d68869ed476f4602962ad2d61f32efa76 --timeout=5m`: zero issues.
- `python3 scripts/list-docs.py validate`: 378 decisions and 1547 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node scripts/validate-public-docs.mjs`: 47 published pages passed.
- `node /tmp/pr3143-workspace-preflight.cjs`: all four work orders covered, no errors.
- `git diff --check`: passed.

Tests ran in the isolated PR checkout. All 24 changed Go files were compared
byte-for-byte with the task workspace and match. No real GitHub quota or browser
E2E run was required for these backend repairs. The user subsequently authorized
commit, push, and CI verification. The conversation records delivery and
current-head CI results separately from this local test receipt.

Pre-push validation also scans the full backend against the PR base. Its first
run found the obsolete `legacyWorkspaceIdentity` constant, which was removed.
The conversation records the final lint and commit-hook receipts.
