---
id: "02-bundle-queries"
title: "Durable clarification bundle read queries"
status: done
wave: 2
depends_on: ["01-resolutions-table"]
plan: "plan.md"
spec: "../../specs/external-question-answering/spec.md"
---

# Task 02: Durable clarification bundle read queries

Read bundles from the durable `clarification_request` messages rather than the in-memory store, so
the list tool is correct after a restart and the resolver can resolve a bundle's identity. Both
queries treat "unresolved" as "has no `clarification_resolutions` row" (D4).

- **Acceptance:**
  1. `ListUnresolvedClarificationBundles` returns bundles ordered by `MIN(created_at)` then
     `pending_id`, excludes any bundle with a resolution row regardless of message statuses, and
     includes a bundle whose per-question messages disagree on status.
  2. Owner scoping happens inside the query: a scoped caller never sees a bundle whose task's
     workspace has a different `owner_id`, and still sees bundles whose task has an empty
     `workspace_id` (D5). A page is never short because rows were filtered after `LIMIT`.
  3. A cursor returns only bundles strictly after it under the L6 order; `updated_since` and
     `cursor` intersect (L14); `limit` clamps to 1..200 with a default of 50 (L10).
  4. `GetClarificationBundle` returns `task_id`, `session_id`, and questions ordered by
     `question_index`, degrading a question with unparseable `question` metadata to its
     `question_id` with empty prompt/options rather than dropping the bundle (L15).

- **Verification:**
  ```
  cd apps/backend && \
    gofmt -l internal/task/repository/sqlite internal/task/models && \
    go test -race ./internal/task/repository/sqlite/... && \
    go build ./...
  ```
  Postgres coverage when a DSN is available:
  ```
  cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" \
    go test -race -run 'ClarificationBundle' ./internal/task/repository/sqlite/...
  ```

- **Files likely touched:**
  - `apps/backend/internal/task/repository/sqlite/clarification_bundle.go` (new)
  - `apps/backend/internal/task/models/clarification_resolution.go` (add `ClarificationBundle`,
    `ClarificationBundleQuestion`, `ClarificationBundleFilter`)
  - `apps/backend/internal/task/repository/interface.go`
  - `apps/backend/internal/task/repository/sqlite/clarification_bundle_test.go` (new)

- **Dependencies:** 01 — the `NOT EXISTS` subquery needs the table.
- **Parallelism:** `sequential`.

- **Inputs:**
  - Spec L1–L15, D1, D3, D4, D5, D6, D7.
  - Plan § *Backend → Model and repository* for the query shape and the keyset-cursor predicate.
  - Existing queries to mirror: `FindMessagesByPendingID`
    (`sqlite/message.go:412`) and `FindPendingClarificationMessagesBySessionID` (`:432`) for the
    `dialect.JSONExtract` idiom and `scanMessageRows`; `pendingActionsBySessionQuery` for a
    driver-parameterized query builder.
  - `age_seconds` (L3) and RFC3339 formatting are the caller's job, not the repository's — the
    repository returns `time.Time`. D7's floor-at-0 belongs with the MCP handler in task 05.
  - Do not filter on `metadata.status` alone; it is a pre-filter at most (L2).

- **Output contract:** summary, files changed, tests run with counts, blockers, risks, and the
  task/plan status update in the same conversation.

## Results

Implemented across commits `8bccf9298`, `842853393`, `324677546`. Added
`internal/task/repository/sqlite/clarification_bundle.go` with
`ListUnresolvedClarificationBundles` (owner-scoped `NOT EXISTS` against
`clarification_resolutions`, keyset cursor on `(bundle_created_at, pending_id)`, `updated_since`
intersection, `limit` clamped 1..200 default 50) and `GetClarificationBundle` (per-question rows
ordered by `question_index`, degrading unparseable `question` metadata to id-only rather than
dropping the bundle), plus the `ClarificationBundle*` model types and the third
`ensureMessageMetadataIndexes` index.

Verified in this session's final gauntlet (Wave 10): `go test ./internal/task/repository/sqlite/...`
passes, including `clarification_bundle_test.go` (ordering, owner scoping never seeing empty-scope
rows, page-not-short-after-LIMIT, cursor exclusivity, `updated_since`/cursor intersection, mixed
per-question status, degraded-question case). `gofmt -l` reports no files; `go build ./...` succeeds.
Postgres-DSN coverage was not independently re-run in this session (no `KANDEV_TEST_POSTGRES_DSN`
available in this environment); the required SQLite path is green.

No external side effects.
</content>
