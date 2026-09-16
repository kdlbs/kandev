---
id: "04-retire-journal"
title: "Remove legacy journal storage safely"
status: in_progress
wave: 4
depends_on:
  - "03-core-reconciliation"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-007
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-006.8
  - AC-PLUGINS-PROMPT-HISTORY-HOST-007.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-007.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-007.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-007.4
system_design:
  - "../../specs/plugins/system-design/conversation-source-reconciliation.md"
---

# Task 04: Remove legacy journal storage safely

## Summary

Remove the obsolete runtime and add the forward cleanup migration.
This work order establishes the final single-source storage contract for both fresh and upgraded installations.

## In scope

- Own atomic trigger/table cutover, PostgreSQL function removal, startup ordering, migration cancellation, and failure propagation.
- Remove journal readers, mirrors, durable ACK/poison APIs, maintenance workers, composition wiring, and retired wire types.
- Remove only the legacy Host event database and its SQLite sidecars after primary cleanup commits.
- Add pre-PR-schema fixtures, partial-backfill fixtures, failpoints, and source-preservation sentinels.
- Integrate new coverage with required-store conformance and the previous-Stable upgrade harness. Preserve tagged fixture provenance.
- Add a disposable benchmark for 10k/100k 1 KiB messages and record storage, write, and startup measurements.

## Out of scope

- Automatic VACUUM, deletion of backups or plugin storage, rolling mixed-version deployment, and unrelated database cleanup.

## Acceptance

- Fresh and legacy upgrades preserve source records byte-for-byte and leave no legacy writer or payload table.
- DDL failure rollback, repeated startup, direct writes, FK cascades, and real PostgreSQL pass targeted tests.
- Successful file cleanup removes only expected regular files. Failed cleanup is observable and retryable without reopening the old store.

## Verification

Run from the repository root. Obtain behavioral RED evidence before production edits.

```bash
(cd apps/backend && go test -race ./internal/task/repository/sqlite ./internal/plugins ./internal/backendapp ./internal/persistence/storeconformance -count=1)
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test -race ./internal/task/repository/sqlite ./internal/plugins ./internal/persistence/storeconformance -run 'Conversation|PreviousStableUpgrade|UpgradeFixtureManifest' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test ./internal/task/repository/sqlite -run '^$' -bench '^BenchmarkConversationSourceStorage$' -benchtime=1x -count=1)
rg -n 'conversation_message_versions|conversation_turn_versions|conversation_session_events|session-events.sqlite|SessionDeliveryDispatcher' apps/backend apps/web
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/conversation_journal.go`
- `apps/backend/internal/task/repository/sqlite/conversation_journal_cleanup.go (new)`
- `apps/backend/internal/task/repository/sqlite/conversation_journal_cleanup_test.go (new)`
- `apps/backend/internal/task/repository/sqlite/conversation_source_bench_test.go (new)`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/plugins/conversation_journal.go`
- `apps/backend/internal/plugins/conversation_stream.go`
- `apps/backend/internal/plugins/conversation_stream_service.go`
- `apps/backend/internal/plugins/service.go`
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/types.go`
- `apps/backend/internal/persistence/requiredstores/`
- `apps/backend/internal/persistence/storeconformance/upgrade_test.go`
- `apps/backend/internal/persistence/storeconformance/testdata/`
- `apps/backend/pkg/websocket/actions.go`
- `apps/web/lib/ws/ordered-session-events.ts`

## Dependencies

03-core-reconciliation.

## Risks

- A bare code revert leaves copy triggers installed.
- The legacy SQLite file is outside the primary database transaction. Retry cleanup without touching keys or plugin data.
- DROP frees logical storage but can cause one-time I/O and does not shrink the SQLite file.
- The rg audit permits only cleanup code and historical fixtures/tests. Inspect each match, not only its exit code.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/prompt-history-extraction-host.md) and the frontmatter criteria.
- [System design](../../specs/plugins/system-design/conversation-source-reconciliation.md).
- [ADR](../../decisions/2026-09-16-conversation-source-reconciliation.md).
- Existing conversation handler, journal, Host facade, and recovery tests provide the regression patterns.

## Results

- Removed legacy journal readers, writers, ordered delivery runtime, ACK/poison persistence, and startup backfill composition.
- Added idempotent SQLite/PostgreSQL cleanup for legacy conversation tables and triggers, with source-preservation and safe Host-file sidecar cleanup tests.
- Added the `BenchmarkConversationSourceStorage` benchmark and recorded 10k/100k-row runs in the implementation evidence.
- `go run ./cmd/sqlguard ./internal`: passed.
- Targeted cleanup, repository, plugin, gateway, and test-harness race checks: passed.
- PostgreSQL integration variants remain unrun because `KANDEV_TEST_POSTGRES_DSN` is unavailable in this workspace.
- Status remains in progress until the required PostgreSQL migration and cleanup matrix passes.


## Large legacy SQLite verification

The requested large-database verification passed on a 2.95 GB synthetic legacy
database. Backup, cleanup, task-repository initialization, and HTTP readiness
were measured separately. Both 60-second streaming runs completed without
write or readiness/probe failures. Active SQLite triggers, production workers,
and retired host files were audited. Manual compaction and subsequent readiness
passed; no automatic startup VACUUM was added.

See [the verification report](verification/large-sqlite-upgrade.md) for workload,
commands, measurements, evidence, and limits. The public operations guide now
includes explicit post-upgrade compaction. PostgreSQL and other existing plan
gates remain open. All changes remain uncommitted.
