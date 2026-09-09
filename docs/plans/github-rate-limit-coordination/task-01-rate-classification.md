---
id: 01-rate-classification
title: Typed provider failure classification
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-001
system_design: ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 01: Typed Provider Failure Classification

## Acceptance

- REST, GraphQL, and CLI paths distinguish primary, secondary, credentials,
  missing resource, transient, and unknown failures.
- The 5000/5000 plus simultaneous 403 fixture preserves healthy primary state,
  records secondary state, and can clear on an earlier successful response.
- Retry-After and primary reset metadata survive wrapping in GitHubAPIError.

## Verification

`cd apps/backend && go test ./internal/github -run 'Test.*(Rate|Classif|RetryAfter|GitHubAPIError|Stderr)' -count=1`

## Files likely touched

- `apps/backend/internal/github/rate_error.go`
- `apps/backend/internal/github/ratelimit.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/graphql.go`
- `apps/backend/internal/github/gh_client.go`
- corresponding focused tests

## Results

- RED: the field fixture failed because a 403 with core 5000/5000 rewrote the
  snapshot to remaining zero with a synthetic one-hour reset.
- RED: CLI stderr tests failed because rate prose always exhausted its inferred
  primary bucket and returned no typed provider error.
- GREEN: the task verification command passed.
- Full `internal/github` package tests passed after the final Task 01 edit.
- Tests use task-owned `GOCACHE` and `GOMODCACHE` under `/tmp` because the
  environment's shared module cache is read-only.
