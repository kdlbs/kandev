---
id: "01-persist-text-segment-boundaries"
title: "Persist and project durable text segment boundaries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-SEGMENTATION-001
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-SEGMENTATION-001.1
  - AC-PLATFORM-DURABLE-AGENT-SEGMENTATION-001.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/platform/system-design/durable-agent-stream-processing.md
---

# Task 01: Persist and project durable text segment boundaries

## Summary

Make top-level tool starts durable message boundaries for ID-less assistant text. Store the boundary with the SQL projected cursor, persist the authoritative committed canonical ID in the inbox atomically, and preserve existing projected history and legacy behavior.

## In scope

- Add and migrate the SQL cursor boundary and additive inbox canonical-ID column in SQLite and PostgreSQL.
- Advance it only when a top-level tool call projects.
- Resolve durable text segment IDs in canonical projection and return committed IDs to lifecycle notifications.
- Add lifecycle, repository, migration, restart, duplicate, and browser regressions.
- Resolve IDs from a stable base identity plus the committed boundary. Never re-hash an already-resolved ID or mutate caller-owned events before commit; duplicate projection returns the exact stored committed ID.
- Update every top-level tool projection route, including normal projection, retained replay, and canonical shell-start projection. Persist boundaries with the projected cursor in event sequence order, including mixed batches. Nested tools and tool updates remain non-boundaries.
- Initialize legacy cursor boundaries once without resetting valid values on reopen; cover nonzero projected cursors, pruned payloads, and owner/generation/cursor updates.
- Treat `canonical_message_id` as the authoritative committed projection ID. Payload compaction preserves it; whole-row retirement deletes it with the receipt and keeps expired-replay rejection.
- For old projected rows with an empty column, use a retained payload ID only when its payload is still available. A pruned payload yields no ID and no notification; do not infer one from incoming metadata or the current boundary.
- Keep projection-only canonical IDs out of transport identity/hash checks, and ensure duplicate admission preserves the stored column.
- Update plan results after implementation and synchronize the plan status.

## Out of scope

- Agentctl journal or wire protocol changes.
- Changes to native message IDs, reasoning grouping, nested tools, or legacy delivery.
- Rewriting existing projected transcript rows or changing chat layout.
- Unrelated durable delivery cleanup.

## Acceptance

- Text chunks before and after top-level tool starts project to separate assistant rows; multiple chunks within each segment coalesce. Nested tool calls and tool updates do not split rows.
- Duplicate delivery, batching, partial projection, replay, reconnect, and backend or agentctl restart retain the same committed message IDs. Existing projected messages remain unchanged, and legacy delivery remains unchanged.
- Projection results expose only committed IDs. When an old pruned receipt has no stored ID, duplicate projection leaves it unchanged and sends no notification. Payload pruning preserves the ID; whole-row expiration retains the existing typed replay rejection.
- SQLite and PostgreSQL schema upgrades initialize existing cursors once, including nonzero projected cursors and pruned payloads, and preserve the projection transaction invariant.
- Failure injection proves rollback leaves the stored base identity, boundary, message effect, and cursor unchanged; retry commits the same identity. A duplicate after a later boundary returns the original committed ID.

## Verification

Run from the repository root after implementation.

```bash
(cd apps/backend && go test -race -trimpath -p 1 -parallel 4 ./internal/agent/runtime/lifecycle -run 'TestCanonicalAgentMessageIDPreservesNativeAndReasoningIdentity|TestCanonicalProjectionDoesNotNotifyWithoutCommittedMessageID|TestCanonicalProjectionBatchSkipsRowsWithoutCommittedMessageID' -count=1)
(cd apps/backend && go test -race -trimpath -p 1 -parallel 4 ./internal/task/repository/sqlite -run 'TestCanonicalDeliveryTextSegments|TestCanonicalDeliveryTextSegment|TestCanonicalDeliverySegment|TestCanonicalDeliveryRetentionPruneRestartAndDuplicatePreserveCommittedID|TestCanonicalDeliveryPreUpgradePrunedReceiptDoesNotInventIdentity|TestDeliveryRetentionIntegration$|TestDeliveryRetentionSchemaUpgradePreservesPreRetentionEvidence' -count=1)
(cd apps/backend && go test -race -trimpath -p 1 -parallel 4 -v ./internal/task/repository/sqlite -run '^(TestDeliveryRetentionSchemaUpgradePreservesPreRetentionEvidence|TestCanonicalDeliveryPreUpgradePrunedReceiptDoesNotInventIdentity)$/^Postgres$' -count=1)
(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race -trimpath -p 1 -parallel 4 ./internal/persistence/storeconformance -count=1)
(cd apps/web && pnpm e2e:run tests/chat/durable-agent-message-segments.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The focused E2E command must report discovery and execution of the intended Chromium test. It must ingest events through durable-v1 using the mock/retained-delivery fixture, assert durable mode by disconnecting the live stream successfully, verify multi-chunk text/tool/text row ordering, then reload and verify again. A single tool call renders as a standalone row; do not require a tool group. Record unavailable PostgreSQL-backed cases as an evidence gap; do not report them as passed. Run `bash scripts/lint-go-changed <changed-go-files>` for changed Go files. Run `pnpm install --frozen-lockfile` from `apps/` once if this worktree lacks dependencies before invoking pnpm.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`
- `apps/backend/internal/agent/runtime/lifecycle/stream_event_processor.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream_test.go`
- `apps/backend/internal/task/models/agent_delivery.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_schema.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_segments_test.go`
- `apps/web/e2e/tests/chat/durable-agent-message-segments.spec.ts`

## Dependencies

None.

## Risks

- Updating the SQL boundary without the message and projected cursor in the same transaction can make replay assign a different segment.
- Existing projected cursor values are a conservative upgrade boundary when older tool payloads are pruned; this can start one new message after upgrade but must not modify existing rows.
- Notification must use the canonical ID persisted by projection, including when an unprojected retained event predates this change.

## Parallelism

`sequential`

## Inputs

- [Plan and source evidence](plan.md).
- [Durable delivery requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Durable text segmentation requirements](../../specs/platform/requirements/durable-agent-text-segmentation.md).
- [Durable delivery design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Durable stream processing design](../../specs/platform/system-design/durable-agent-stream-processing.md).
- [Text segment identity ADR](../../decisions/2026-10-26-durable-agent-text-segment-identity.md).
- Existing lifecycle and SQLite projection tests.

## Results

The implementation changes are complete, but this work order remains in
progress because broad SQLite and PostgreSQL verification are incomplete.
SQLite stores the top-level tool boundary beside the projected cursor and
commits it atomically with message effects. Canonical text IDs derive from a
stable base and committed boundary. The inbox `canonical_message_id` column
is the authoritative committed ID; payload compaction retains it, and
duplicates return the exact stored value. Fresh schemas include the columns;
idempotent SQLite/PostgreSQL `ADD COLUMN` migrations run in `runMigrations()`
after schema initialization. Existing transcript rows remain untouched.
Normal projection, retained replay, and canonical shell-start routes preserve
ordered boundaries.

Verification results:

- Change-aware race command `go test -race -trimpath -p 1 -parallel 4 ./internal/agent/runtime/lifecycle -run 'TestCanonicalAgentMessageIDPreservesNativeAndReasoningIdentity|TestCanonicalProjection' -count=1` passed (1.26s).
- The migration-order test `TestAgentDeliverySegmentationColumnsAreAddedByRunMigrations` failed before the implementation change because schema initialization added the columns early, then passed after moving the additive migrations to `runMigrations()`.
- After that fix, the focused race command `go test -race -trimpath -p 1 -parallel 4 -v ./internal/task/repository/sqlite -run 'TestCanonicalDeliveryTextSegments|TestCanonicalDeliveryTextSegment|TestCanonicalDeliverySegmentBoundaryMigration|TestCanonicalDeliverySegmentBoundaryPostgres|TestCanonicalDeliveryRetentionPruneRestartAndDuplicatePreserveCommittedID|TestCanonicalDeliveryPreUpgradePrunedReceiptDoesNotInventIdentity|TestDeliveryRetentionIntegration$|TestAgentDeliverySegmentationColumnsAreAddedByRunMigrations|TestDeliveryRetentionSchemaUpgradePreservesPreRetentionEvidence|TestDeliveryRetentionInterruptedMigrationResumes' -count=1` passed (15.0s). It covers SQLite segmentation, rollback/retry, real payload pruning/reopen/duplicate, pre-upgrade fallback, interrupted and schema upgrades, and whole-row expiry. PostgreSQL subtests skipped because `KANDEV_TEST_POSTGRES_DSN` is unset.
- The implicated test `go test -race -p 1 -parallel 4 -timeout 20m ./internal/task/repository/sqlite -run '^TestCompleteActiveClarificationBundlePersistsRecoverableDeliveryIntent$' -count=1` passed again after the migration-path change (2.103s).
- `pnpm e2e:run tests/chat/durable-agent-message-segments.spec.ts` passed 1/1 Chromium test (10.6s) after rebuilding backend and Vite assets. The test ingested through durable-v1, verified disconnect, text/tool/text order and multi-chunk coalescing, then reloaded and checked the transcript.
- `go run -trimpath ./cmd/sqlguard ./internal` passed. After the migration-path change, `go test -race -trimpath -p 1 -parallel 4 ./internal/persistence/storeconformance -count=1` passed (84.7s). Changed-Go lint reported 0 issues. `python3 scripts/list-docs.py validate` validated 404 decisions and 1741 specifications; `python3 scripts/lint-spec-files.py --all` and `git diff --check` passed.
- PostgreSQL schema-upgrade and fallback tests were attempted through the `Postgres` subtests and skipped because `KANDEV_TEST_POSTGRES_DSN` is unset. PostgreSQL behavior remains unverified locally.

Broad SQLite diagnostic, recorded separately from the focused passes:

- Default-timeout command `go test -race -p 1 -parallel 4 ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/task/repository/sqlite ./internal/orchestrator -count=1` timed out in lifecycle, SQLite, and orchestrator; agentctl passed. This command failed overall.
- Controlled command `go test -race -p 1 -parallel 4 -timeout 20m ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/task/repository/sqlite ./internal/orchestrator -count=1` passed lifecycle (402.4s), agentctl (6.9s), and orchestrator (561.1s), but timed out in SQLite at 20m during core schema initialization. This command failed overall.
- Available timeout output identifies `TestCompleteActiveClarificationBundlePersistsRecoverableDeliveryIntent` in the `newRepoForSessionTests` core schema initialization path. The full goroutine stacks and lock/connection owners are not available because the original output was truncated and not saved; the cause is unresolved.
- A recovery audit searched all 2,534 available session records and task-specific `/tmp` artifacts; no broad-run stack was present. The only retained `/tmp/kandev-4788-base-test.log` and `/tmp/kandev-4788-implementation-test.log` files are 69-byte outputs from the earlier isolated comparison (2.502s on base, 2.558s on the initial implementation). The blocked goroutine chain and lock/connection owners therefore cannot be named.
- The isolated comparison command `go test -race -p 1 -parallel 4 -timeout 20m ./internal/task/repository/sqlite -run '^TestCompleteActiveClarificationBundlePersistsRecoverableDeliveryIntent$' -count=1` passed sequentially with no concurrent Go tests on unchanged base `7fb28fc022d5320922608bf57cc9fd9351a8b8da` (2.841s) and implementation `8bafa71d33be8d20029699202605f136dae41bc3` (2.103s), using matching race, package, parallelism, and timeout settings. This exact test passes in isolation on both revisions; it does not reproduce or explain the broad SQLite timeout. Earlier comparisons on pre-migration-path implementation revisions also passed (2.502s base, 2.558s and 2.170s implementation).

The earlier incomplete verification is superseded by the parent takeover
evidence below: complete SQLite CI cohorts passed, and PostgreSQL cases now
execute locally with the corrected portable queries.


## Parent takeover verification (2026-10-26)

The parent session owns PR #4819 from head `e9a20fbcb650` onward.
Backend CI run `40385254227` passed all three complete SQLite repository
cohorts and all four ordinary backend shards on that head. The earlier local
SQLite timeout remains historical evidence with an unknown cause. Complete
repository CI coverage now supplements those local attempts.

The PostgreSQL CI cohort exposed two migration-regression queries that used
SQLite placeholders without `Rebind`. Both queries now use the database dialect.
The failure reproduced locally before correction with SQLSTATE 42601.
A disposable local PostgreSQL 16 instance then ran the segment and migration
regressions, including previously skipped PostgreSQL cases:

```bash
(cd apps/backend && go test -trimpath -race -parallel 4 ./internal/task/repository/sqlite -run 'TestCanonicalDelivery(TextSegments|TextSegment|Segment)|TestAgentDeliverySegmentationColumnsAreAddedByRunMigrations|TestAgentDeliveryRetentionMigratesPreRetentionSchema' -count=1 -timeout 5m)
```

With `KANDEV_TEST_POSTGRES_DSN` set to the local instance, this passed in 15.068s.
The DSN is runtime-only and contains no production credentials.

The Windows native CI job also stalled in the supervised-descriptor test while
closing a synchronous pipe reader with its writer still open. The fixture now
releases the writer first, matching supervised child exit. Cleanup uses the
same order so a failed assertion cannot hang cleanup. No launcher behavior changes.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/agentctl/launcher -run 'TestPipeOutput' -count=1 -timeout 2m)
```

This passed locally in 3.174s. The Windows rerun remains a CI check.
The PR will be marked ready after pushing these corrections, as the user requested.


## Post-hold PR fixup (2026-10-26)

After the requested 15-minute hold, the current-head CodeRabbit review identified
an E2E scheduling gap and stale plan wording. The browser fixture now blocks
its tool on an `e2e:await_file` marker. It asserts no tool row exists before the
forced disconnect, then releases the marker after the disconnect call. Backend
transcript polling precedes rendered-row waits and chronological DOM checks.
The updated managed Chromium test rebuilt production assets and passed 1/1
in 10.9s. Targeted ESLint passed. No product behavior changed in this fixup.
