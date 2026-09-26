---
spec: docs/specs/ui/requirements/ci-pr-automation.md
created: 2026-08-04
status: complete
---

# Implementation Plan: Exact CI Popover Review-Thread Counts

## Overview

The lightweight GitHub GraphQL poll currently requests only the first 100
review threads, then treats every omitted thread as unresolved. This produces
false counts on busy PRs and can incorrectly trigger auto-fix or block
auto-merge. Keep the normal batched query unchanged for PRs that fit in one
page, paginate only truncated review-thread connections, and persist a count
only after every page completes.

---

## Backend

### GraphQL response contract

In `apps/backend/internal/github/graphql.go`:

- Extend the internal `reviewThreads` response shape with
  `pageInfo { hasNextPage endCursor }` and request those fields from
  `prFieldsBlock`.
- Count only fetched nodes whose `isResolved` value is false. Remove the
  `totalCount - len(nodes)` fallback; `totalCount` cannot reveal resolution
  state.
- Represent an incomplete connection as an internal continuation containing
  PR identity, the destination `PRStatus`, and the next cursor. No public
  `PRStatus`, `TaskPR`, database, HTTP, or WebSocket shape changes.

### Follow-up pagination

In `apps/backend/internal/github/graphql.go`:

- Have the numbered-PR decoder collect continuations when a review-thread
  connection has another page. Branch lookup only discovers PR metadata and
  must not fail because an unused review-thread continuation fails; the next
  numbered-watch sync owns the complete count.
- Execute follow-up review-thread page queries in bounded batched rounds,
  reusing `GraphQLExecutor` with a dedicated five-PR continuation chunk size.
  Each round adds only the unresolved nodes it actually received and advances
  through GitHub's cursor until `hasNextPage` is false.
- Include the top-level rate-limit selection in follow-up queries so existing
  PAT and `gh` executors continue recording GraphQL quota.
- Reject an absent, empty, or repeated continuation cursor, pagination beyond
  the initial `totalCount` page budget, a missing/null response alias, a decode
  failure, or any top-level GraphQL error. Return no partially completed status
  map on such failure, allowing the existing batched-query fallback to retain
  the previous complete count.
- Preserve existing partial-success behavior for repositories that are
  independently classified as missing. If another repository's continuation
  later fails, retain the typed missing-repository error around that failure so
  the caller still populates its negative cache while rejecting all partial
  statuses. Issue no follow-up request when all selected PRs fit in their first
  page.

### Persistence and automation

No new persistence code is required. `Service.SyncTaskPR` already stores
`PRStatus.UnresolvedReviewThreads` only when
`UnresolvedReviewThreadsPopulated` is true, and the CI popover plus automation
read that stored field. Successful pagination therefore replaces a fabricated
count with the exact count; failed pagination must not produce a populated
partial result.

---

## Frontend

No frontend implementation changes. The existing popover hides
`pr-comments-row` when `unresolved_review_threads` is zero and already has
desktop E2E coverage for that behavior.

---

## Tests

- **What:** A PR with more than 100 fully resolved threads reports zero, while
  unresolved threads on later pages contribute exactly once.
  **File:** `apps/backend/internal/github/graphql_test.go`.
  **How:** Drive `runBatchedPRQuery` through a sequenced `GraphQLExecutor` and
  assert the initial query, continuation query, query count, and final
  `UnresolvedReviewThreads` value.
- **What:** Branch discovery returns a selected PR without fetching unused
  review-thread continuation pages.
  **File:** `apps/backend/internal/github/graphql_test.go`.
  **How:** Return a selected branch PR whose review threads have another page,
  provide no continuation response, and assert the PR is still discovered with
  one GraphQL query.
- **What:** A continuation error or invalid cursor never returns a partial
  status map.
  **File:** `apps/backend/internal/github/graphql_test.go`.
  **How:** Fail the sequenced executor after the first page and assert an error
  plus no consumable partial result.
- **What:** A continuation failure does not hide repositories already
  classified as unresolvable by the initial batch.
  **File:** `apps/backend/internal/github/graphql_test.go`.
  **How:** Return a missing-repository GraphQL error beside a busy PR, fail its
  continuation, and assert the typed missing-repository wrapper retains the
  pagination failure while the status map remains nil.
- **What:** The exact multi-page count reaches the persisted `TaskPR` used by
  the popover and automation.
  **File:** `apps/backend/internal/github/poller_test.go`.
  **How:** Use `setupBatchedPollerTest`, seed a numbered watch and prior nonzero
  count, return fully resolved pages, run the real batched poll/apply path, and
  assert the stored count becomes zero.

---

## E2E Tests

- **Scenario:** GIVEN the backend reports zero unresolved review threads,
  WHEN the user opens the CI popover, THEN no unresolved-comments row appears.
- **File:** Existing
  `apps/web/e2e/tests/pr/pr-topbar-popover.spec.ts` scenario
  `comments row hidden when unresolved_review_threads = 0`.
- **What to verify:** Rerun the existing Chromium scenario. No E2E source
  change is expected because the defect is in the backend count producer.

---

## Verification Results

- Exact review-thread pagination is implemented for numbered PRs. Follow-up
  pages remain batched, cursor cycles and incomplete responses fail closed, and
  no partial count reaches persistence. Branch discovery remains independent
  of unused review-thread continuation pages.
- Focused regression tests, the full GitHub backend package, scoped Go lint,
  and the existing Chromium popover scenario all pass. Exact commands and
  timings are recorded in the task file.

---

## Implementation Waves And Parallel Candidates

Sequential:

- [x] [task-01-paginate-review-threads](task-01-paginate-review-threads.md)

No parallel candidate: query code, unit coverage, persistence coverage, and
the final status field form one tightly coupled backend change.

---

## Risks

- A malformed or non-advancing GitHub cursor could otherwise loop forever;
  cursor validation must fail the sync instead.
- Follow-up pages add GraphQL work only for PRs over 100 review threads. The
  implementation must retain batching so a workspace with several busy PRs
  does not regress to one subprocess per PR.
- Review-thread state can change while pages are fetched. This fix follows
  GitHub connection cursors and accepts the completed traversal as the poll's
  snapshot, matching existing eventual-consistency behavior.

## Out of Scope

- Renaming the existing UI label from comments to threads.
- Changing PR-watch terminal reset behavior.
- Adding a new persisted completeness column or changing public API payloads.
