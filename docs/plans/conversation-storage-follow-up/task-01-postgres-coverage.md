---
id: "01-postgres-coverage"
title: "Prove PostgreSQL source and cleanup behavior"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-007
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.5
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.10
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.11
  - AC-PLUGINS-PROMPT-HISTORY-HOST-007.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-007.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-007.3
system_design:
  - ../../specs/plugins/system-design/conversation-source-reconciliation.md
---

# Task 01: Prove PostgreSQL source and cleanup behavior

## Summary

Add actual PostgreSQL tests for the replacement and run them on a disposable database.
Resolve demonstrated dialect defects while preserving the approved source and receipt contract.

## In scope

- Own repository PostgreSQL test coverage and any resulting source, receipt, or cleanup fixes.
- Reuse `testutil.PostgresDSNFromEnv` and `testutil.OpenIsolatedPostgres`; never target user data.
- Add named tests `TestConversationPostgresSource`, `TestConversationPostgresReceipts`, and
  `TestConversationPostgresCleanup` in `conversation_source_postgres_test.go` (new).
- Cover fresh and legacy schemas, direct SQL changes, inserts/updates/deletes, cross-session moves,
  turn completion, session/task cascades, atomic rollback, and revision-row bounds.
- Exercise real concurrent connections, deterministic revision locks, and complete receipt intervals.
- Cover repeatable-read pages, keyset boundaries, task/author filters, and expected-revision mismatch.
- Build legacy PostgreSQL fixtures from the comparison base's DDL, not translated SQLite SQL.
- Verify exact old tables, triggers, and functions disappear without unrelated object removal.
  Inject cleanup failure, prove rollback and successful retry, and preserve source bytes and indexes.
- Run existing previous-stable upgrade conformance. Record which dialect each subtest actually uses.

## Out of scope

SQLite performance reruns, production upgrades, automatic compaction, and new durable storage.

## Acceptance

1. All three named PostgreSQL tests run without skipping and cover the listed matrix.
2. Fresh, repeated, interrupted, and legacy upgrades preserve source data and remove legacy copies.
3. Concurrent writes and consistent reads meet the existing revision/receipt contract; evidence includes
   PostgreSQL version, connection concurrency, counts, command output, and tested commit without credentials.

## Verification

Run from the repository root after adding the named tests. Missing tests or skips fail this gate.
Provision a disposable PostgreSQL service through available local tooling, or report the missing DSN.

```bash
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run '^TestConversationPostgres' -count=1 -v)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run '^TestConversation' -count=1)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -run 'PreviousStableUpgrade|UpgradeFixtureManifest' -count=1 -v)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/conversation_source_postgres_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/conversation_source.go`
- `apps/backend/internal/task/repository/sqlite/conversation_receipts.go`
- `apps/backend/internal/task/repository/sqlite/conversation_journal_cleanup.go`
- `apps/backend/internal/persistence/storeconformance/upgrade_test.go`
- Reuse helpers from `workspace_orphan_postgres_test.go` and `internal/testutil`.

## Risks

An existing PostgreSQL initialization failure can block the matrix. Isolate it against the base;
fix narrow prerequisites with regression evidence, and document any larger required design change.

## Dependencies

None. Run sequentially in the recommended order.

## Parallelism

`sequential`

## Inputs

- [Host requirements](../../specs/plugins/requirements/prompt-history-extraction-host.md) and [source reconciliation design](../../specs/plugins/system-design/conversation-source-reconciliation.md).
- [Architecture decision](../../decisions/2026-09-16-conversation-source-reconciliation.md).
- Implementation commit `c0a048bc128f7ef9a1051caf95ed442627faf9df`; comparison base `88c6c0fe0a6ae5d25332070d603b99f7d9241645`.
- [Original package](../conversation-storage-replacement/plan.md) and its recorded evidence.

## Results

Pending. Record exact commands, tested commit, environment, results, and skipped tests.
Update this package and the original package together; preserve historical results.
