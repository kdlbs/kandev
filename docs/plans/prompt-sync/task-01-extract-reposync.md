---
id: "01-extract-reposync"
title: "Extract shared repository-sync helpers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-005
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-005.2
  - AC-INTEGRATIONS-PROMPT-SYNC-005.4
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 01: Extract shared repository-sync helpers

## Summary

Move the provider-neutral parts of `internal/workflowsync` into a new
`internal/reposync` package so Prompt Sync can reuse them. Workflow Sync
behavior and its existing tests must not change.

## In scope

- `reposync.Source` (provider, workspace, GitHub host, owner, repo, project
  path, branch, path) and `NormalizeSource(req, defaultPath)`, moved from the
  Workflow Sync request normalization.
- `reposync.Reader` with `List(ctx, Source) ([]Entry, error)` and
  `Get(ctx, Source, path) ([]byte, error)`, wrapping the existing
  `GitHubClientProvider` and `GitLabClientProvider` interfaces (which move
  here), plus the not-configured sentinels.
- `ClassifyError`, `SafeErrorMessage`, `SafeStoredErrorMessage`,
  `RetryPolicyDelay`, and `BuildFailureDirective(input FailureInput, err, now,
  jitter)`, where `FailureInput` carries the provider, interval, consecutive
  failures, circuit state, and next retry time instead of `*workflowsync.Config`.
- `workflowsync` delegates to these. Its continuation, admission-deferral, and
  scheduler logic stay in place.

## Out of scope

Any behavior change, new metrics, or Prompt Sync code.

## Acceptance

- Every existing `internal/workflowsync` test passes unchanged, apart from
  imports and renamed unexported helpers.
- `internal/reposync` has its own tests for classification, sanitizing,
  backoff growth, cap, provider retry-time precedence, and GitHub
  suspension, moved or ported from `workflowsync`.

## Verification

```bash
cd apps/backend
go test -trimpath ./internal/reposync ./internal/workflowsync -count=1
go test -trimpath -race ./internal/workflowsync -count=1
cd ../.. && bash scripts/lint-go-changed $(git diff --name-only -- '*.go')
```

## Likely files

- `apps/backend/internal/reposync/{source.go,reader.go,failure.go,backoff.go}` (new) and tests
- `apps/backend/internal/workflowsync/{service.go,backoff.go,failure.go,models.go}`

## Risks

- The automatic continuation path calls `listProviderEntries` and
  `fileGetterForConfig` in several places. Keep their signatures as thin
  adapters so the continuation tests stay meaningful.
- `goleak` in `workflowsync` must stay green, and `reposync` must start no
  goroutines.
