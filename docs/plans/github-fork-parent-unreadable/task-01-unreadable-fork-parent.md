---
id: "01-unreadable-fork-parent"
title: "Treat an unreadable fork parent as no parent PR"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.6
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
---

# Task 01: Treat an unreadable fork parent as no parent PR

## Summary

`findPRInForkParent` returns no PR and no error when the parent PR query is
not found, and stores a non-fork value in the scoped fork-parent cache so the
parent is not queried again until the entry expires.

## In scope

- Not-found detection for the parent `FindPRByHead` error: `GitHubAPIError`
  with `StatusCode == 404` (REST client), `isNotFoundErr`, or the GraphQL
  missing-repository response from `gh pr list` (GH CLI client).
- On not-found: replace the `forkParentCache` entry for the fork's scoped key
  (`scopedCacheKey(cacheScope, repoErrorCacheKey(owner, repo))`) with
  `forkParent{}` using the cache's default lifetime, then return `nil, nil`.
  Skip the cache write when the service has no cache.
- Regression tests in a new file
  `apps/backend/internal/github/service_pr_fork_parent_unreadable_test.go`,
  using a test-local client that embeds the existing mock and returns a
  404 `GitHubAPIError` from `FindPRByHead` for the parent repository.

## Out of scope

- `classifyPRDiscoveryError`, `isRepoNotResolvableErr`, health categories,
  frontend, and mock client changes.
- Any error other than not-found from the parent query.

## Acceptance

1. Per-watch path: a searching watch on a fork with no fork PR and a parent
   404 completes with no PR, no error, and healthy discovery health.
2. Batched path: after a definitive-empty alias, a parent 404 leaves the watch
   searching with no failure; a second sync inside the cache lifetime issues
   no parent query (assert the call count).
3. A parent 5xx (or rate-limit) error still records the existing category,
   and a PR later opened in the fork is still discovered.
4. After the cache entry expires, the next lookup finds a PR in a parent that
   has become readable.

## Verification

Write the tests first and confirm acceptance 1 and 2 fail with the
`unavailable` outcome before the change.

```bash
# from the repository root
(cd apps/backend && go test -trimpath -tags fts5 -count=1 \
  -run 'ForkParent|ClassifyPRDiscovery|SyncWatchesBatched' ./internal/github/)
(cd apps/backend && go test -trimpath -tags fts5 -race -count=1 ./internal/github/)
bash scripts/lint-go-changed apps/backend/internal/github/service_automation_auth.go \
  apps/backend/internal/github/service_pr_fork_parent_unreadable_test.go
```

## Likely files

- `apps/backend/internal/github/service_automation_auth.go`
- `apps/backend/internal/github/service_pr_fork_parent_unreadable_test.go` (new)

## Results

- Regression tests written first; acceptance 1 and 2 failed with the parent
  404 surfacing as `unavailable` (per-watch: lookup error; batched: degraded
  health). The parent 502 test passed before and after the change.
- `findPRInForkParent` now returns no PR and no error for a not-found parent
  query and caches the fork as having no searchable parent for the credential
  scope.
- `go test -trimpath -tags fts5 -race -count=1 ./internal/github/`: pass.
- `scripts/lint-go-changed` (golangci-lint v2.9.0): 0 issues.
- Review fixup: added the post-expiry recovery test (acceptance 4) and aligned
  the recovery wording with the polling schedule.
- Manual check on a copy of a real installation (GitHub App connection,
  private fork parent): an explicit `github.task_pr.sync` on the searching
  watch completed with discovery health `healthy`; the unpatched v0.101.0
  instance reports `degraded`/`unavailable` for the same target. The
  integration settings page shows no discovery warning.

### Maintainer review follow-up (2026-11-16)

- Added the GraphQL missing-repository response from `gh pr list` to the
  parent-only not-found handling. The service still returns other parent
  errors, including typed non-404 errors with missing-repository text.
- Extended the service regression cases for the GraphQL response, invalid
  credentials, forbidden access, integration permission errors, and rate limits.
- Added the unreadable-parent outcome and recovery timing to the public
  integration troubleshooting guide.
- Tests, builds, linters, and commit validation hooks were not run for this
  follow-up, as requested. The earlier validation results describe the
  original contribution.
