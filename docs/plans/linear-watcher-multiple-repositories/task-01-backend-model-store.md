---
id: "01-backend-model-store"
title: "Backend: multi-repo model, schema, and store"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/linear-watcher-multiple-repositories/spec.md"
---

# Task 01: Backend — multi-repo model, schema, and store

## Acceptance

- `IssueWatch` carries `Repositories []IssueWatchRepository`; `IssueWatchRepository{RepositoryID, BaseBranch}` is defined in `apps/backend/internal/linear/models.go`. Custom `UnmarshalJSON` on `IssueWatch` fills `Repositories` from the legacy `repositoryId`/`baseBranch` keys when `repositories` is absent (plural wins when both present).
- `linear_issue_watches` gains `repositories_json TEXT NOT NULL DEFAULT ''` in `createTablesSQL` plus an idempotent `addIssueWatchRepositoriesJSONColumn()` migration (`PRAGMA table_info` guard + `ALTER TABLE`) that **backfills** `repositories_json` from non-empty legacy `repository_id`/`base_branch` and is registered in `initSchema`. Fresh installs get the column from DDL.
- Store CRUD persists/reads `repositories_json` as canonical and **derives** the legacy `repository_id`/`base_branch` columns from the first entry (or `''` when unbound) **at write time** — the store is the sole writer of the DB columns, so the mirror cannot drift; the service mirrors the same values on the response object. Reads fall back to the legacy columns only when `repositories_json` is empty.
- `CreateIssueWatchRequest` / `UpdateIssueWatchRequest` gain `Repositories []IssueWatchRepository` (update: nil slice = unchanged, empty = clear); `NewLinearIssueEvent` replaces `RepositoryID`/`BaseBranch` with `Repositories`.

## Verification

```bash
cd apps/backend && go test ./internal/linear/... -run 'IssueWatch|Repository|Migration|Backfill' -count=1
cd apps/backend && go build ./...
```

## Files likely touched

- `apps/backend/internal/linear/models.go`
- `apps/backend/internal/linear/store.go`
- `apps/backend/internal/linear/store_issue_watch.go`
- `apps/backend/internal/linear/store_issue_watch_repository_test.go` (extend: multi-entry round-trip, legacy fallback, migration with backfill; add a pre-multi-repo old-schema constant WITH the repo columns)
- `apps/backend/internal/linear/store_issue_watch_test.go` (fixture `newTestIssueWatch` if it needs the new field)

## Dependencies

None (first wave). Downstream tasks consume the model/store contract.

## Inputs

- Spec: `Data model`, `API surface`, first three scenarios.
- Plan: `Design > Wire & storage shape`, `Schema + migration`, `Store`.
- Existing patterns: `addIssueWatchRepositoryColumns` (`store.go:163-192`), `encodeFilter`/`toIssueWatch` (`store_issue_watch.go`), Sentry `SearchFilter.UnmarshalJSON` precedent (`apps/backend/internal/sentry/models.go`).

## Risks

- Backfill must be idempotent and must not clobber a `repositories_json` already written by a newer binary running against an older DB (`repositories_json = ''` guard in the UPDATE).
- Keep the legacy mirror columns in sync on every write so the read-fallback never serves stale singular values.
- JSON encode/decode of the array must not break rows whose `repositories_json` is `''` (old rows before backfill).

## Output contract

Report the model/store/migration changes, exact test command results, and any divergence from the plan; mark this task `done` and update its checkbox in `plan.md`.
