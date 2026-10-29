---
id: 01-status-progress
title: Preserve completed PR status progress
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-001
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.6
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.7
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.4
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.5
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
---

# Task 01: Preserve completed PR status progress

## Summary

Publish a complete numbered status phase before unrelated branch discovery can
defer. Preserve phase progress and current result ownership across search churn.

## In scope

- Separate numbered and searching progress keys under existing credential scope.
- Introduce an internal outcome for applied results and unfinished groups.
- Preserve complete results through service, poller, workspace, and task callers.
- Restrict transport fallback to unfinished groups. Detect typed local admission
  before fallback selection in every affected caller.
- Preserve lifecycle transactions, attempt generations, and idempotent publication.

## Out of scope

The monitor timer belongs to Task 02. Frontend, schema, API envelopes, admission
policy, Actions cache policy, and general REST continuation remain unchanged.

## Acceptance

1. A complete known-PR status reaches its stored row and event before an unrelated
   searching phase defers. Five changing-search cycles do not restart completed work.
2. Deferred/incomplete evidence cannot erase counts or emit success. Credential,
   host, retirement, detach, and explicit-refresh fences reject old results.
3. Existing callers retain successful results on error and never repeat completed
   groups through fallback. Unchanged state publishes no duplicate task-PR or feedback event.

## TDD and verification

Run commands from the repository root. Add the first named regression before
production edits. It must fail because the completed known-PR update remains pending.

```bash
(cd apps/backend && go test -tags fts5 ./internal/github -run '^TestPRStatusProgress_CompletedKnownPhaseSurvivesDiscoveryPacing$' -count=1 -v)
```

Use real `RateAdmission`, a store-backed service, fake GraphQL responses, and
`testing/synctest`. Add a new searching target each virtual cycle.
Keep the known PR identity unchanged. Assert stored state, emitted events,
and provider call counts. No live database, credentials, or network is required.

Add `TestPRStatusProgress_PreservesEvidenceAndFences` with incomplete thread
pagination, shared-target fan-out, and delayed passive/manual result ordering.
Include retired/deleted watches and credential replacement during a held fetch.

After the correction, run:

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/github -run 'TestPRStatusProgress_|TestFetchBatchedWatchStatuses|TestBatchedPR|TestSyncWatchesBatched|TestSyncWorkspaceWatchesBatched|TestTriggerPRSyncAll|TestPRWatchResult' -count=1 -timeout=5m)
git diff --check
```

Keep existing GraphQL field, fork identity, detached-row, and result-writer tests
in the task selection if their production paths change. Record any added command.

## Files likely touched

- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/github/batch_progress.go`
- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_watch_status_progress_test.go` (new)
- `apps/backend/internal/github/graphql_test.go`
- `apps/backend/internal/github/poller_test.go`

`graphql.go` changes only if phase progress requires a new internal return shape.
Do not duplicate decoding or bypass `applyPRWatchStatusResult`.

## Dependencies

None. This work order defines the outcome consumed by Task 02.

## Inputs

- [Discovery requirements](../../specs/integrations/requirements/github-pr-discovery-health.md), POLLING-001 and POLLING-002.
- [Independent publication design](../../specs/integrations/system-design/github-pr-discovery-health.md#independent-pr-status-publication).
- [Plan evidence and reproduction](plan.md#evidence-and-assumption-check).
- Existing `TestBatchedWatchPhases_NumberedPhaseSurvivesSearchingDeferral`
  (formerly `TestFetchBatchedWatchStatusesKeepsCompletedPhaseAcrossAdmissionDeferral`).
- Existing `service_pr_watch_result.go` transaction and retirement guards.

## Risks

Shared batch callers currently discard results on error. Updating only the
poller leaves passive workspace/task paths inconsistent. Complete phase output
must retain observation identity until the guarded write succeeds.

## Parallelism

`sequential`

## Results

Done on 2026-10-28.

RED (base commit `4222c3654f9b`, temporary worktree):

- `(cd apps/backend && go test -tags fts5 ./internal/github -run
  '^TestPRStatusProgress_CompletedKnownPhaseSurvivesDiscoveryPacing$' -count=1 -v)`
  failed with `known PR must be persisted while unrelated discovery is
  deferred: "[]" should have 1 item(s), but has 0`.

Production change:

- `fetchBatchedWatchStatuses` now runs two phases. The numbered phase has its
  own singleflight/progress key and is applied before the searching phase runs,
  so a searching deferral cannot withhold a completed known-PR update.
- `prWatchSyncOutcome` carries applied results plus unfinished phases and typed
  local deferral. Exported `SyncWatchesBatched` / `SyncWorkspaceWatchesBatched`
  keep their signatures and return partial results with the error.
- Poller, workspace refresh, and passive task sync classify
  `AdmissionDeferredError` before fallback selection, and the fallback receives
  only unfinished watches. Schema/throttling failures keep their previous
  paused/no-error contract.
- Progress keys are split (`batched-pr:` / `batched-branch:`) so a changed
  searching set cannot discard numbered chunk progress.

GREEN:

- `(cd apps/backend && go test -tags fts5 -trimpath -race ./internal/github
  -run 'TestPRStatusProgress_|TestFetchBatchedWatchStatuses|TestBatchedPR|TestSyncWatchesBatched|TestSyncWorkspaceWatchesBatched|TestTriggerPRSyncAll|TestPRWatchResult'
  -count=1 -timeout=10m)`: ok, 6.7s.
- `(cd apps/backend && go test -tags fts5 -trimpath ./internal/github -count=1
  -timeout=25m)`: ok, 105.7s.
- `bash scripts/lint-go-changed <the changed Go paths listed in the plan>`:
  0 issues.
- `git diff --check`: passed.

New coverage: `service_pr_watch_status_progress_test.go`
(`TestPRStatusProgress_CompletedKnownPhaseSurvivesDiscoveryPacing`,
`TestPRStatusProgress_PreservesEvidenceAndFences`,
`TestPRStatusProgress_ExplicitRefreshSupersedesHeldPassiveResult`,
`TestTriggerPRSyncAll_DeferredAdmissionSkipsFallback`) plus updates to
`graphql_test.go` and the two `poller_test.go` caller-shape regressions.

Review remediation: `runBatchedOrPerWatchSync` now skips the REST fallback for a
typed local deferral (and for auth/config boundaries) instead of retrying the
same blocked work. It reloads the completed rows and returns the deferral.
`TestTriggerPRSyncAll_DeferredAdmissionSkipsFallback` covers plain, wrapped, and
joined deferrals with zero REST calls.

Deviations: the phase test (formerly
`TestFetchBatchedWatchStatusesKeepsCompletedPhaseAcrossAdmissionDeferral`) now
drives the numbered and searching phase helpers separately, because the
completed phase is applied immediately and its progress is released. The two `...QueryError_ReturnsError`
tests now accept one unapplied result entry alongside the error instead of
`nil`, matching the partial-outcome contract.

