---
id: "01-persist-retirement"
title: "Persist canonical watch retirement"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.1
  - AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.6
  - AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.7
  - AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.8
system_design:
  - ../../specs/integrations/system-design/github-pr-watch-retirement.md
---

# Task 01: Persist canonical watch retirement

## Summary

Add retirement state and transactional store operations. Public deletion keeps its current behavior until Task 02.
This work establishes storage primitives for the complete acceptance criteria.

## In scope

- Nullable `retired_at`, model mapping, additive migration, and rebuild preservation.
- Idempotent retirement and transactional resumption with a new watch ID.
- Canonical identity reservation and active-only operational inventory.
- Inclusive lookup and list primitives for authorization and explicit inventory.
- Shared transaction admission for retirement and result application.
- Archive preservation, permanent task cleanup, migration replay, and PostgreSQL concurrency.

## Out of scope

Public endpoint changes, provider calls, service dispatch, and rendered UI.

## Acceptance

1. Fresh and upgraded stores retain retirement across replay and table rebuilds. Existing rows remain active.
2. Retirement and resumption serialize with result admission. Failure rolls back all changes. Canonical collisions preserve retirement.
3. Active inventory excludes retired rows. Authorization inventory includes them. Archive cleanup preserves retirement, and permanent deletion removes it.

## TDD sequence

Add `TestPRWatchRetirement_MigrationReplay` and store lifecycle tests first.
Prove that missing retirement storage fails these tests before implementing the store changes.
Use `TestPRWatchRetirement_PostgresConcurrency` with separate connections for retirement, resume, create, and result admission races.
The PostgreSQL test must execute behavior, not only replay schema text.
Keep lock identities consistent across store operations. Do not hold transactions across provider requests.

## Verification

Run from the repository root. Configure an isolated PostgreSQL DSN before these commands.
An unset DSN causes a skip, which is not PostgreSQL validation.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/github -run 'TestPRWatchRetirement_|TestPRWatchMigration_|TestPostgresStoreSchemaReplay' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/github -run '^TestPRWatchRetirement_PostgresConcurrency$' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
mapfile -t retirement_go_files < <(git ls-files --modified --others --exclude-standard -- apps/backend | rg '\.go$')
if ((${#retirement_go_files[@]})); then bash scripts/lint-go-changed "${retirement_go_files[@]}"; fi
```

The PostgreSQL command uses `KANDEV_TEST_POSTGRES_DSN`.
Run `bash scripts/lint-go-changed` with every modified Go path. Record the expanded command and actual results.

## Files likely touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/store_pr_watch_retirement.go` (new)
- `apps/backend/internal/github/store_pr_watch_retirement_test.go` (new)
- `apps/backend/internal/github/store_pr_watch_migration_test.go`
- `apps/backend/internal/github/store_postgres_test.go`
- `apps/backend/internal/github/store_task_cleanup.go`
- `apps/backend/internal/github/store_task_cleanup_test.go`
- `apps/backend/internal/persistence/storeconformance/testdata/upgrades/v0.93.0/manifest.json` and its fixture if schema-history updates are required.

## Dependencies

None. Task 02 consumes these primitives and completes public behavior.

## Risks

Watch-table rebuilds can discard added fields. PostgreSQL read-modify-write operations need transaction-level locking.
Generic collision coalescing must not remove a retired identity.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-watch-retirement.md), ACs .1, .6-.8.
- [Design](../../specs/integrations/system-design/github-pr-watch-retirement.md), persistence and concurrency.
- Existing canonical watch migration and PostgreSQL store tests.
- `apps/backend/AGENTS.md`, including conformance and schema migration rules.

## Results

Implemented nullable retirement state and replay-safe schema upgrades, including both legacy watch-table rebuilds. Canonical migration collisions preserve retirement, operational inventory excludes retired watches, retirement is idempotent, resumption replaces the watch with a fresh ID and clears polling watermarks, archive cleanup removes active rows while retaining retired rows, and permanent task cleanup removes both.

Validation passed on 2026-10-26. PostgreSQL checks ran against an isolated local PostgreSQL 16 container with separate connections:

- `KANDEV_TEST_POSTGRES_DSN=<isolated local PostgreSQL DSN> go test -trimpath -tags fts5 ./internal/github -run 'TestPRWatchRetirement_|TestPRWatchMigration_|TestPostgresStoreSchemaReplay' -count=1`.
- `KANDEV_TEST_POSTGRES_DSN=<isolated local PostgreSQL DSN> go test -trimpath -race -tags fts5 ./internal/github -run '^TestPRWatchRetirement_PostgresConcurrency$' -count=1`.
- `go run ./cmd/sqlguard ./internal`.
- `KANDEV_TEST_POSTGRES_DSN=<isolated local PostgreSQL DSN> go test -trimpath -race ./internal/persistence/storeconformance -count=1`.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.
- `bash scripts/lint-go-changed apps/backend/internal/github/models.go apps/backend/internal/github/store.go apps/backend/internal/github/store_pr_watch_migration_test.go apps/backend/internal/github/store_watch_reset_test.go apps/backend/internal/github/store_pr_watch_retirement.go apps/backend/internal/github/store_pr_watch_retirement_postgres_test.go apps/backend/internal/github/store_pr_watch_retirement_test.go` reported 0 issues.
