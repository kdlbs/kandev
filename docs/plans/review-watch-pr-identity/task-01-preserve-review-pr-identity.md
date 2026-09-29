---
id: "01-preserve-review-pr-identity"
title: "Preserve review PR identity through enrichment"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.1
  - AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.2
system_design:
  - ../../specs/integrations/system-design/github-fork-review-start.md
---

# Task 01: Preserve review PR identity through enrichment

## Summary

Carry complete `GetPR` identity and related metadata onto review-watch search
results before they become events. Prove that the resulting same-repository
event grants an automatic launch token while fork and unknown identities retain
the existing manual-start boundary.

## In scope

- Write a failing enrichment regression using a search-shaped PR and a fake
  `GetPR` result in `service_reviews_test.go`.
- Copy the requested head/base repository, SHA, mergeability, and maintainer
  fields in `enrichPRDetails`, preserving the current branch and diff fields.
- Exercise `TriggerReviewWatch` to event to `buildReviewTaskRequest` in backend
  tests, including same-repository, fork, and unavailable-detail cases. Pass the
  same emitted PR into the request builder.
- Let `MockClient` provide separate review-search and detail responses so the
  test reproduces GitHub's lightweight search result.

## Out of scope

- A changed fork policy, metadata backfill, database migration, and UI changes.

## Acceptance

1. The new enrichment regression is RED before the code change and GREEN after
   it; all requested fields and existing fields match the `GetPR` result.
2. A same-repository review PR from the watch passes
   `reviewPRHeadMatchesBase`, has `auto_start_claimed`, and lacks
   `fork_pr_requires_manual_start`.
3. A fork or identity-incomplete PR keeps `fork_pr_requires_manual_start` and
   has no auto-start token, including when `GetPR` fails.

## Verification

```bash
(cd apps/backend && go test ./internal/github -run 'TestEnrichPRDetails|TestCheckReviewWatch' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'TestReviewWatchSearchResultIdentityControlsAutoStart|TestBuildReviewTaskRequest_ForkPRRequiresManualStart' -count=1)
(cd apps/backend && go test -race ./internal/github -run 'TestCheckReviewWatchEnrichesSearchResultsBeforePublishingEvent|TestMockClient_Reset' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run 'TestReviewWatchSearchResultIdentityControlsAutoStart|TestBuildReviewTaskRequest_ForkPRRequiresManualStart' -count=1)
make -C apps/backend test
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/github/service_reviews.go`
- `apps/backend/internal/github/mock_client.go`
- `apps/backend/internal/github/service_reviews_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_review_test.go`

## Dependencies

None.

## Risks

- Keep the lookup tied to the search result's target repository. Missing
  provider identity must never be inferred from that target.
- Existing tests construct complete PRs and may miss a search-to-detail mapping
  regression unless the new tests use separate search and detail objects.

## Parallelism

`sequential`

## Inputs

- [Fork review-start requirement](../../specs/integrations/requirements/github-fork-review-start.md),
  [design](../../specs/integrations/system-design/github-fork-review-start.md),
  and [security decision](../../decisions/2026-09-23-fork-pr-review-watch-manual-start.md).
- `CheckReviewWatch`, `enrichPRDetails`, `publishNewReviewPREvent`,
  `buildReviewTaskRequest`, and nearby backend test helpers.

## Results

Implemented full PR detail enrichment for review-watch search results whose
head identity is absent, even when both branch names are already present. The
published event retains head/base repository identity, revisions, mergeability,
maintainer permission, branch names, and diff counts. The regression was RED
because `GetPR` was not called, then passed with the fix. The same-repository,
fork, and failed-detail paths all preserve the expected launch boundary. The
orchestrator test sends the actual `TriggerReviewWatch` event PR into
`buildReviewTaskRequest` and asserts both the event identity and launch metadata.

Verification:

- PASS: `go test ./internal/github -run 'TestEnrichPRDetails|TestCheckReviewWatch' -count=1`
- PASS: `go test ./internal/orchestrator -run 'TestBuildReviewTaskRequest|TestReviewWatch' -count=1`
- PASS: `go test ./internal/github -count=1`
- PASS: `go test ./internal/orchestrator -run 'TestReviewWatchSearchResultIdentityControlsAutoStart|TestBuildReviewTaskRequest_ForkPRRequiresManualStart' -count=1`
- PASS: `go test -race ./internal/github -run 'TestCheckReviewWatchEnrichesSearchResultsBeforePublishingEvent|TestMockClient_Reset' -count=1`
- PASS: `go test -race ./internal/orchestrator -run 'TestReviewWatchSearchResultIdentityControlsAutoStart|TestBuildReviewTaskRequest_ForkPRRequiresManualStart' -count=1`
- PASS: `make -C apps/backend build`
- PASS: `make -C apps/backend lint`
- BLOCKED by unrelated existing probe failures: `make -C apps/backend test`
- BLOCKED by the same two real-process probe failures after config isolation:
  `env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE -u KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG make -C apps/backend test`
- Confirmed: the isolated `TestProbeRealTree_AllDescendantsPreTurn_Settled` and
  `TestProbeRealTree_NewDescendantAfterTurnStart_Live` tests both return `live`
  where they expect `settled`.
