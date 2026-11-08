---
id: "04-continuations"
title: "Resume deferred synchronization"
status: done
wave: 4
depends_on: ["03-cancellation"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-002
  - REQ-INTEGRATIONS-GITHUB-RATE-003
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-RATE-002.2
  - AC-INTEGRATIONS-GITHUB-RATE-002.3
  - AC-INTEGRATIONS-GITHUB-RATE-003.5
  - AC-INTEGRATIONS-GITHUB-RATE-003.7
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 04: Resume deferred synchronization

## Summary

Preserve completed fetch steps when admission defers the next request. Keep waits outside the bounded workers without restarting from the directory.

## In scope

- Add a narrow fetch continuation to the automatic job.
- Retain directory entries, completed file contents, and the next file index.
- Validate configuration, credential fingerprint, and local sync generation before resumption and application.
- Invalidate continuations on manual completion, configuration mutation, credential change, and shutdown.
- Release workspace locks and worker slots before waiting.
- Apply only a complete current file set. Clear continuation data on every terminal path.
- Trace GitHub poller multi-request consumers of nonblocking admission. Add a production-path progress regression for affected consumers.
- Do not weaken interactive priority, provider retry windows, or background pacing.

## Out of scope

Other work orders, external GitHub writes, and unrelated refactoring.

## Regression evidence

Add TestAutomaticSyncResumesFileFetchAfterPacing with real rate admission and a local HTTP provider. Assert request counts, complete apply, and persisted success. Add TestAutomaticSyncDiscardsStaleContinuation with configuration replacement and manual-success orderings. Use channels or synctest rather than wall-clock sleeps.

## Acceptance

- A directory with two workflow files completes with one directory read and one successful read per file under unchanged configuration.
- Four deferred principals do not prevent a fifth ready workspace from progressing.
- Stale continuations cannot apply, overwrite recovery state, or survive shutdown.

## Verification

Run from the repository root after implementation:

```bash
(cd apps/backend && go test ./internal/workflowsync -run 'Test.*(Automatic|Deferred|Continuation|Pacing|SyncDue)' -count=1)
(cd apps/backend && go test ./internal/github -run 'Test.*(Admission|Coordinator|Poller|Batched)' -count=1)
(cd apps/backend && go test -race ./internal/workflowsync ./internal/github -run 'Test.*(Automatic|Deferred|Continuation|Pacing|Admission|Coordinator|Batched)' -count=1)
```

## Files likely touched

- apps/backend/internal/workflowsync/automatic_scheduler.go
- apps/backend/internal/workflowsync/service.go
- apps/backend/internal/workflowsync/automatic_scheduler_test.go
- apps/backend/internal/workflowsync/service_provider_dispatch_test.go
- apps/backend/internal/workflowsync/service_test.go
- apps/backend/internal/github/rate_coordinator_test.go
- apps/backend/internal/github/poller_test.go
- apps/backend/internal/github/service_pr_watch_batched_budget_test.go

## Dependencies

03-cancellation. Execute sequentially.

## Risks

A single admission-unit test cannot prove operation progress. Do not replace the bounded pool with one provider goroutine per workspace.

## Parallelism

`sequential`

## Inputs

RATE-002 and RATE-003. System design: Principal-wide coordinator. Regression commit 64738cc23. Existing scheduler cancellation and bounded-work tests.

## Results

Completed 2026-09-27. Automatic workflow jobs now retain the directory entries,
fetched file bodies, and next entry while waiting outside the bounded worker
pool. Resumption validates the configuration fingerprint, credential
fingerprint, and job-local generation. Manual syncs and config mutations cancel
waiting jobs; credential changes discard fetched data and restart from a fresh
directory listing; shutdown clears pending progress. The service applies only
the full current file set.

The GitHub poller trace found the same restart pattern in multi-chunk batched
PR and branch GraphQL requests. Those production paths now retain completed
chunks and review-thread page cursors under the full credential-scoped query
key. Inactive progress expires after 24 hours. Each query-type cache evicts its
oldest inactive entries when it reaches 256 entries; active entries remain
until their requests complete. Both caches are cleared with PR cache
invalidation or service shutdown.

Regression coverage includes the production workflow scheduler with a local
HTTP provider, stale config/manual/shutdown ordering, credential replacement,
and real nonblocking admission for PR and branch GraphQL chunks. The existing
bounded scheduler regression confirms four deferred jobs do not block a fifth
ready workspace.

Validation passed:

- `go test ./internal/workflowsync -run 'Test.*(Automatic|Deferred|Continuation|Pacing|SyncDue)' -count=1`
- `go test ./internal/github -run 'Test.*(Admission|Coordinator|Poller|Batched)' -count=1`
- `go test -race ./internal/workflowsync ./internal/github -run 'Test.*(Automatic|Deferred|Continuation|Pacing|Admission|Coordinator|Batched)' -count=1`
- `go test -race ./internal/github -run 'Test.*(Batched|Coordinator|Admission)' -count=1`

The branch-progress cache now expires stale entries when branch queries resume.
`TestBatchedBranchQueryProgressExpiresAfterTTL` failed before this correction
and passed afterward.

The existing retry-boundary test uses the asynchronous automatic scheduler.
It now waits for the workspace job to leave the in-flight map before checking
the persisted retry deadline. This removes a race between the provider call
and the durable result read. The test passed 100 repeated runs and 10
race-enabled runs.

An independent review found that an in-flight directory or file request could
fail after its credential rotated. The error path now revalidates the
continuation before returning the provider error, so a changed credential
discards the old progress and restarts from a fresh directory listing. The
channel-controlled regression proves the failed old-credential request is not
recorded as the automatic sync result and only the replacement result applies.

Validation passed:

- `go test ./internal/workflowsync -run '^TestAutomaticSync(RestartsAfterFailedRequestUsesRotatedCredential|RestartsFetchAfterCredentialChange)$' -count=1`
- `go test ./internal/workflowsync -count=1 -timeout=3m`
- `go test -race ./internal/workflowsync -run 'Test.*(Automatic|Deferred|Continuation|Pacing|SyncDue)' -count=1 -timeout=3m`
