---
id: "01-defer-unenriched-review-prs"
title: "Defer review PRs whose detail read failed"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.2
  - AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.5
system_design:
  - ../../specs/integrations/system-design/github-fork-review-start.md
---

# Task 01: Defer review PRs whose detail read failed

## Summary

A review watch publishes a new PR only after its details have been read. If
the read fails, the PR stays untracked and the next poll picks it up again.

## Change

`enrichPRDetails` returns the PRs that are ready to publish, and
`CheckReviewWatch` publishes that list. A PR whose `GetPR` call returns an
error or a nil PR is left out of the list. A successful `GetPR` response that
has no head repository is kept, and that PR stays fail-closed downstream.

## Out of scope

`buildReviewTaskRequest`, backoff, and new retry state.

## Root cause and reproduction

`enrichPRDetails` (`internal/github/service_reviews.go:535`) uses `continue`
after a failed `GetPR` and leaves the incomplete search result in the publish
list. The test cases "detail lookup failure defers PR until a later successful
poll" and "nil detail response defers PR until a later successful poll" in
`internal/github/service_reviews_test.go` cover both retry paths.

## Steps

1. Change those two cases to expect zero returned PRs and no event. Run them
   and confirm that they fail.
2. Return a `ready` slice from `enrichPRDetails` and assign it in
   `CheckReviewWatch`.

## Verification

```bash
go -C apps/backend test -trimpath ./internal/github -run 'TestCheckReviewWatch' -count=1
go -C apps/backend test -trimpath ./internal/github -run 'TestEnterpriseWatchDelayedResultsFenceHostAndGeneration|TestEnterpriseWatchPublicationUsesCapturedGeneration' -count=1
go -C apps/backend test -trimpath ./internal/orchestrator -run 'TestReviewWatchSearchResultIdentityControlsAutoStart' -count=1
go -C apps/backend test -trimpath ./internal/github ./internal/orchestrator -count=1
bash scripts/lint-go-changed apps/backend/internal/github/service_reviews.go apps/backend/internal/github/service_reviews_test.go apps/backend/internal/github/enterprise_watch_fences_test.go apps/backend/internal/orchestrator/session_ensure.go apps/backend/internal/orchestrator/session_ensure_fork_review_test.go apps/backend/internal/orchestrator/event_handlers_github_review_test.go
```

## Results

The regression failed first for both failed and nil detail responses because
the watch returned one PR. After the change, the test confirms neither response
publishes a PR, and a later successful poll enriches and publishes it. The
retry assertions use a small scenario struct and event capture so the test does
not pass an event pointer-to-pointer through an eight-argument helper. The
orchestrator identity test now expects a failed detail read to emit no event,
while a successful response with missing head identity remains fail-closed.
Enterprise review-watch fence fixtures return complete PR details for their
positive controls. The full GitHub and orchestrator packages pass with these
updates.

Passed:

- `go -C apps/backend test -trimpath ./internal/github -run 'TestCheckReviewWatch' -count=1`
- `go -C apps/backend test -trimpath ./internal/github -run 'TestEnterpriseWatchDelayedResultsFenceHostAndGeneration|TestEnterpriseWatchPublicationUsesCapturedGeneration' -count=1`
- `go -C apps/backend test -trimpath ./internal/orchestrator -run 'TestReviewWatchSearchResultIdentityControlsAutoStart' -count=1`
- `go -C apps/backend test -trimpath ./internal/github ./internal/orchestrator -count=1`
- `bash scripts/lint-go-changed apps/backend/internal/github/service_reviews.go apps/backend/internal/github/service_reviews_test.go apps/backend/internal/github/enterprise_watch_fences_test.go apps/backend/internal/orchestrator/session_ensure.go apps/backend/internal/orchestrator/session_ensure_fork_review_test.go apps/backend/internal/orchestrator/event_handlers_github_review_test.go`
