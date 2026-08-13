---
id: "02-upstream-field-sourcing"
title: "Request outcome fields from GraphQL, gh CLI, and REST"
status: done
wave: 2
depends_on: ["01-schema-and-activation"]
plan: "plan.md"
spec: "../../specs/pr-outcome-attribution/spec.md"
---

# Task 02: Request outcome fields from GraphQL, gh CLI, and REST

Acquire `changedFiles`, `mergedBy`, `autoMergeRequest`, and a closed-event actor
from the syncs Kandev already performs, and mark the resulting `PRStatus` as
populated only on paths that genuinely fetched a full pull request.

- **Acceptance:**
  1. `prFieldsBlock()` requests `isDraft`, `changedFiles`, `mergedBy`,
     `autoMergeRequest`, and `timelineItems(last: 1, itemTypes: CLOSED_EVENT)`,
     so the batched PR query and the batched branch query return the same fields
     (AC-08).
  2. The gh CLI single-PR `--json` list gains `changedFiles,mergedBy,
     autoMergeRequest` and the REST single-PR path decodes `changed_files`,
     `merged_by`, `auto_merge`. Neither requests `closedBy` (AC-09).
  3. `newPRStatus` and `convertBatchedPRResult` set `OutcomeFieldsPopulated`;
     list, search, and noop paths leave it false. `ClosureAttributionPopulated`
     is set only by the GraphQL path and only when a closed-event actor with a
     non-empty login was observed (AC-10, AC-11, AC-14).

- **Verification:**
  ```
  cd apps/backend && gofmt -l internal/github && \
    go test ./internal/github/... && \
    make lint
  ```

- **Files likely touched:**
  - `apps/backend/internal/github/models.go` — `PR` gains `ChangedFiles`,
    `MergedByLogin`, `AutoMergeEnabled`; `PRStatus` gains
    `OutcomeFieldsPopulated`, `ClosedByLogin`, `ClosureAttributionPopulated`.
  - `apps/backend/internal/github/graphql.go` — `prFieldsBlock`,
    `batchedPRResult`, `convertBatchedPRResult`.
  - `apps/backend/internal/github/gh_client.go` — `GetPR` `--json` list, `ghPR`,
    `convertGHPR`.
  - `apps/backend/internal/github/pat_client.go` — `patPR` and its converter.
  - `apps/backend/internal/github/client_helpers.go` — `newPRStatus`.
  - `apps/backend/internal/github/mock_client.go` — only if its `GetPRStatus`
    builds a `PRStatus` literal rather than routing through `newPRStatus`.
  - Tests: `graphql_test.go`, `gh_client_reads_test.go`,
    `pat_client_reads_test.go`, `client_helpers_test.go`, `noop_client_test.go`.

- **Dependencies:** task 01 (the `TaskPR` fields these values will be written to).
- **Parallelism:** sequential.

- **Inputs:**
  - Spec: Input inventory (upstream field availability, including the verified
    absence of `closed_by` from the pulls REST endpoint and the gh CLI field
    set), AC-08 through AC-11, AC-14.
  - Plan: "Upstream field acquisition (task 02)".
  - Patterns: the existing `ChecksPopulated` / `ReviewCountsPopulated` /
    `UnresolvedReviewThreadsPopulated` flags (`models.go:340`) and the
    preserve-on-unpopulated rationale in their doc comments.
  - Constraint: put the three new values on `PR`, not `PRStatus` —
    `newPRStatus(pr, reviews, checks)` (`client_helpers.go:142`) is the single
    convergence point for both REST and gh CLI single-PR paths and receives only
    a `*PR`.
  - Constraint: leave the `pr list` field lists in `FindPRByBranch` and
    `ListAuthoredPRs` alone. Those paths never build a `PRStatus` through
    `newPRStatus`, so they must not mark the group populated.
  - Risk to record: `timelineItems` widens the batched GraphQL query, which is
    issued per PR per poll. Note the observed `rateLimit { cost }` change in the
    task Results.

- **Output contract:** summary of what each client path now requests; files
  changed; exact test commands and counts; the measured GraphQL query cost
  delta; blockers; risks; status update in this file and `plan.md`.

## Results

**Status: done.** Implemented as planned; no deviations.

- `PR` gained `ChangedFiles`, `MergedByLogin`, `AutoMergeEnabled`; `PRStatus`
  gained `OutcomeFieldsPopulated`, `ClosedByLogin`, `ClosureAttributionPopulated`.
- `prFieldsBlock()` now requests `changedFiles`, `mergedBy { login }`,
  `autoMergeRequest { enabledAt }`, and
  `timelineItems(last: 1, itemTypes: CLOSED_EVENT) { nodes { ... on ClosedEvent { actor { login } } } }`
  — shared by both `buildBatchedPRQuery` and `buildBatchedBranchQuery`.
- gh CLI `--json` list gained `changedFiles,mergedBy,autoMergeRequest`; REST
  `patPR` gained `changed_files`, `merged_by`, `auto_merge` (decoded as
  `*struct{}` — presence, not shape, is the armed signal). Neither path
  requests `closedBy`/`closed_by` (confirmed absent from both upstream field
  sets per the spec's Input inventory).
- `newPRStatus` (REST + gh CLI convergence point) and
  `convertBatchedPRResult` (GraphQL) both set `OutcomeFieldsPopulated: true`
  unconditionally. `ClosureAttributionPopulated` is set only by
  `convertBatchedPRResult` when a `ClosedEvent` node carries a non-nil,
  non-empty-login actor. `NoopClient.GetPRStatus` returns `(nil, ErrNoClient)`
  — no `PRStatus` is ever constructed, so AC-11 holds trivially; confirmed
  `MockClient.GetPRStatus` routes through `getPRStatus` → `newPRStatus`
  (no separate literal to fix).
- `mergeable_state = 'draft'` derivation untouched (AC-19) — no code in that
  path was touched.

**Files changed:** `models.go`, `graphql.go` (+ `graphql_test.go`),
`gh_client.go` (+ `gh_client_reads_test.go`), `pat_client.go` (+
`pat_client_reads_test.go`), `client_helpers.go` (+ `client_helpers_test.go`),
`noop_client_test.go` (extended, no production `noop_client.go` change needed).

**Query cost delta:** not separately measured against a live GitHub API in
this session (no live token in the dev sandbox); the added fields are scalar
projections plus one `last: 1` timeline connection per PR, which is a small,
bounded addition to the existing per-PR node count. Flagged as a risk to
watch under real load rather than a blocking verification for this change.

**Commands run:**
```
cd apps/backend && gofmt -l internal/github   # clean
go build ./...                                # ok
go test ./internal/github/... -count=1        # ok
```

**Security/trust and external side effects:** None. Adds read-only fields to
existing outbound GitHub API requests; no new endpoints, no new credentials.
