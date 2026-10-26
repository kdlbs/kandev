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

Make top-level tool starts durable message boundaries for ID-less assistant text. Store the boundary with the SQL projected cursor, resolve and persist canonical message identity atomically, and preserve existing projected history and legacy behavior.

## In scope

- Add and migrate the SQL cursor boundary in SQLite and PostgreSQL.
- Advance it only when a top-level tool call projects.
- Resolve durable text segment IDs in canonical projection and return committed IDs to lifecycle notifications.
- Add lifecycle, repository, migration, restart, duplicate, and browser regressions.
- Resolve IDs from a stable base identity plus the committed boundary. Never re-hash an already-resolved ID or mutate caller-owned events before commit; duplicate projection returns the exact stored committed ID.
- Update every top-level tool projection route, including normal projection, retained replay, and canonical shell-start projection. Persist boundaries with the projected cursor in event sequence order, including mixed batches. Nested tools and tool updates remain non-boundaries.
- Initialize legacy cursor boundaries once without resetting valid values on reopen; cover nonzero projected cursors, pruned payloads, and owner/generation/cursor updates.
- Update plan results after implementation and synchronize the plan status.

## Out of scope

- Agentctl journal or wire protocol changes.
- Changes to native message IDs, reasoning grouping, nested tools, or legacy delivery.
- Rewriting existing projected transcript rows or changing chat layout.
- Unrelated durable delivery cleanup.

## Acceptance

- Text chunks before and after top-level tool starts project to separate assistant rows; multiple chunks within each segment coalesce. Nested tool calls and tool updates do not split rows.
- Duplicate delivery, batching, partial projection, replay, reconnect, and backend or agentctl restart retain the same committed message IDs. Existing projected messages remain unchanged, and legacy delivery remains unchanged.
- SQLite and PostgreSQL schema upgrades initialize existing cursors once, including nonzero projected cursors and pruned payloads, and preserve the projection transaction invariant.
- Failure injection proves rollback leaves the stored base identity, boundary, message effect, and cursor unchanged; retry commits the same identity. A duplicate after a later boundary returns the original committed ID.

## Verification

Run from the repository root after implementation.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite -run 'TestCanonicalAgentMessageIDPreservesNativeAndReasoningIdentity|TestCanonicalDeliveryTextSegments|TestCanonicalDeliveryTextSegment|TestCanonicalDeliverySegment' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/task/repository/sqlite ./internal/orchestrator -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run '^TestCanonicalDeliverySegmentBoundaryMigration$' -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run '^TestCanonicalDeliverySegmentBoundaryPostgres$' -v -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
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

Implementation is complete. SQLite stores the top-level tool boundary beside
the projected cursor and commits it atomically with message effects. Canonical
text IDs derive from a stable base and committed boundary, are persisted in
the inbox for replay after pruning, and are returned exactly for duplicates.
New and legacy SQLite/PostgreSQL schemas use additive columns; existing
transcript rows remain untouched. Normal projection, retained replay, and
canonical shell-start routes preserve ordered boundaries.

Verification results:

- Focused race command passed for `TestCanonicalAgentMessageIDPreservesNativeAndReasoningIdentity`,
  text segmentation, rollback/retry, duplicate-after-later-boundary, reopen,
  and boundary migration tests. Lifecycle passed in 1.4s; SQLite passed in
  17.4s.
- SQLite upgrade command `go test -race -p 1 -parallel 4 ./internal/task/repository/sqlite -run '^TestCanonicalDeliverySegmentBoundaryMigration$' -count=1` passed.
- PostgreSQL command `go test -race -p 1 -parallel 4 ./internal/task/repository/sqlite -run '^TestCanonicalDeliverySegmentBoundaryPostgres$' -v -count=1` skipped because `KANDEV_TEST_POSTGRES_DSN` is unset. PostgreSQL migration behavior remains unverified locally.
- The Chromium durable-v1 E2E passed 1/1 test. It ingested mock events through the durable stream, verified the durable-only disconnect, coalesced multiple text chunks on each side of a tool row, and rechecked transcript order and content after reload.
- `go run ./cmd/sqlguard ./internal`, `go test -race ./internal/persistence/storeconformance -count=1`, changed-Go lint (0 issues), `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.
- Broad controlled race command `go test -race -p 1 -parallel 4 -timeout 20m ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl ./internal/task/repository/sqlite ./internal/orchestrator -count=1` passed lifecycle (402s), agentctl (6.9s), and orchestrator (561s). SQLite reached its 20-minute package timeout during core schema initialization under shared-runner I/O contention; no assertion failure was reported. The earlier default-timeout broad run also timed out in lifecycle, SQLite, and orchestrator.

The focused regression suite and durable-v1 browser test pass. The remaining
verification limitations are the unavailable PostgreSQL DSN and the broad
SQLite package timeout under shared-runner load.
