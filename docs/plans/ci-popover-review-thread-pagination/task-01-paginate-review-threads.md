---
id: "01-paginate-review-threads"
title: "Paginate review threads"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 01: Paginate review threads

Implement complete GitHub review-thread pagination for lightweight numbered-PR
GraphQL queries, keep branch discovery independent from unused continuation
pages, then prove the exact count reaches the persisted task PR used by the CI
popover and automation.

## Acceptance

1. PRs with more than 100 fully resolved review threads persist zero
   unresolved threads; unresolved nodes on later pages are counted exactly.
2. PRs fitting on one page make no follow-up request. Numbered PRs with
   continuations use bounded batched page queries; branch discovery returns PR
   metadata without depending on unused continuation pages.
3. Missing, invalid, or failed continuation data returns no partial populated
   count, preserving the prior complete stored value through existing fallback
   behavior. Any missing-repository classifications from the initial batch
   remain available to the existing negative cache.

## Files likely touched

- `apps/backend/internal/github/graphql.go`
- `apps/backend/internal/github/graphql_test.go`
- `apps/backend/internal/github/poller_test.go`
- `docs/specs/ui/requirements/ci-pr-automation.md`
- `docs/plans/ci-popover-review-thread-pagination/plan.md`
- `docs/plans/ci-popover-review-thread-pagination/task-01-paginate-review-threads.md`

The existing `apps/web/e2e/tests/pr/pr-topbar-popover.spec.ts` is verification
input and should not need modification.

## Dependencies

None.

## Parallelism

`sequential`. Query orchestration and persistence coverage share the GitHub
test executors and status contract.

## Inputs

- `docs/specs/ui/requirements/ci-pr-automation.md` requirements for complete counts and
  partial-page failure behavior.
- `docs/plans/ci-popover-review-thread-pagination/plan.md` backend design,
  risks, and out-of-scope boundaries.
- Existing batching/error patterns in
  `apps/backend/internal/github/graphql.go` and
  `apps/backend/internal/github/service_pr_watch_batched.go`.

## Verification

```bash
cd apps/backend && go test ./internal/github -run 'TestBuildBatchedPRQuery_GroupsByRepo|TestRunBatched(PR|Branch)Query_.*ReviewThread|TestTryBatchedPRWatchCheck_.*ReviewThreadCount' -count=1 -v
cd apps/backend && go test ./internal/github -count=1
cd apps/backend && golangci-lint run ./internal/github/...
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run --project chromium tests/pr/pr-topbar-popover.spec.ts -- --grep 'comments row hidden when unresolved_review_threads = 0'
git diff --check
```

## Output contract

Report the cursor/query design, every changed file, exact test and E2E outcomes,
any external GraphQL behavior assumption, cleanup evidence, and blockers or
remaining risks. Mark this task `done`, check it in `plan.md`, and synchronize
`plan.md` verification results only after all commands pass.

## Results

- Cursor/query design: the initial batched query requests
  `pageInfo { hasNextPage endCursor }`; only connections with another page
  become continuations for numbered PRs. Continuations for up to five PRs share
  each follow-up query round. Each status counts only returned unresolved
  nodes. Empty or repeated cursors, pages beyond the initial `totalCount`
  budget, missing/null aliases, decode failures, executor failures, and
  top-level GraphQL errors return no status map.
- Branch discovery intentionally skips unused review-thread continuation pages;
  after association, the next numbered-watch sync owns the complete count.
- Shared owner/repository grouping now drives initial query aliases, error
  aliases, and response decoding. This keeps aliases deterministic and brought
  the decoder under the repository complexity limit.
- TDD red evidence: the old logic reported `2` instead of `0` for 102 fully
  resolved threads and accepted repeated-cursor and null-page responses. Each
  regression turned green after pagination and validation were implemented.
- PR review remediation added a `totalCount`-derived page budget, dedicated
  five-PR continuation chunks, empty-cursor and three-page coverage, a precise
  null-connection fixture, and a branch-discovery regression. The duplicated
  numbered/branch completion tail now shares one helper, and page decoding and
  advancement are split into focused helpers to stay within complexity limits.
- A later Codex regression proved that a continuation failure previously hid a
  missing-repository classification from the negative cache. The typed missing
  error now wraps the pagination failure while returning no partial statuses.
- TDD review-remediation evidence: the combined missing-repository and
  continuation-failure test first returned only the pagination error, then
  passed after preserving the typed wrapper.
- Focused final regression command passed all pagination-budget, cursor,
  multi-round, null-connection, continuation-batching, and branch-discovery
  cases in `0.184s`.
- The focused missing-repository preservation regression passed in `0.016s`.
- Full backend package: `go test ./internal/github -count=1` passed in
  `13.265s`.
- Scoped lint: `golangci-lint run ./internal/github/...` reported `0 issues`.
- Workspace install: lockfile unchanged; 1,143 packages reused/installed in
  `4.8s`.
- Fresh-build Chromium E2E: the existing
  `comments row hidden when unresolved_review_threads = 0` scenario passed;
  `1 passed (10.3s)` after the production backend and Vite builds.
- External assumption: GitHub's `pageInfo` and opaque cursors define a complete
  traversal. Concurrent thread changes are accepted as eventual consistency,
  matching the existing poll model.
- Cleanup: the managed E2E runner exited successfully and removed its temporary
  fixture workspace. `git diff --check` passes; only the intended source,
  regression-test, spec, and plan files remain changed.
