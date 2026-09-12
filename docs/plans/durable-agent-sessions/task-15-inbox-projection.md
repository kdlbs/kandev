---
id: "15-inbox-projection"
title: "Project replayed conversation events"
status: complete
wave: 15
depends_on: ["14-event-replay"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
---

# Task 15: Project replayed conversation events

## Summary

Persist received events before acknowledgment and apply one canonical effect across replay.

## In scope

- Add SQL inbox, received/projected cursors, and deterministic message effect keys.
- Commit canonical message updates with the projected cursor and retain received data until projection succeeds.
- Define durable effect intents for the next work order without dispatching them through the legacy bus.
- Own agent_delivery_inbox_lag_events and agent_delivery_projection_lag_events. Unknown remote bounds remain unavailable, not zero.
- Cover crash boundaries before/after inbox commit, projection commit, and acknowledgment.

## Out of scope

Workflow intent consumption and repeating external MCP actions.

## Acceptance

- Lost acknowledgments and duplicate events produce one canonical message effect.
- Projection crash recovery retains message order, tool identity, and unprocessed intent records.
- Stale-generation events remain auditable without changing the current conversation.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/task/repository/... ./internal/agent/runtime/lifecycle ./internal/orchestrator/... -count=1)
(cd apps/backend && rtk go run ./cmd/sqlguard ./internal)
(cd apps/backend && rtk go test -tags fts5 -race ./internal/persistence ./internal/persistence/storeconformance -count=1)
rtk make -C apps/backend lint
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/backend/internal/task/repository/sqlite/agent_delivery_inbox_test.go`:

- `TestInboxAckAfterCommitDeduplicatesMessages`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1`.

Run conformance with a provisioned PostgreSQL test database and `KANDEV_TEST_POSTGRES_DSN` set.
A skipped PostgreSQL suite does not satisfy acceptance.
Include fresh, migration replay, and previous-stable upgrade results.

## Files likely touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/persistence/storeconformance/`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_inbox_test.go` (new tests or extensions).

## Dependencies

[Task 14](task-14-event-replay.md).

## Risks

- Workflow effects cross repository boundaries. Each consumer needs a durable effect key at its authoritative transition.

## Parallelism

`sequential`

The primary session owns integration. This work order does not authorize subagents.
Preserve existing user edits and unrelated changes.

## Inputs

- [Owned system design](../../specs/platform/system-design/durable-agent-delivery.md).
- [Package manifest](plan.md), including shared regression gates and test prerequisites.
- Existing source and adjacent tests in the listed files.
- [Boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md).

## Results

Implemented backend inbox receipt, duplicate conflict checks, received/projected cursors, lifecycle
projection, durable effect intents, and acknowledgment only after canonical message projection.
Stable message identities coalesce newline-free chunks, and canonical message mutation plus the
projected cursor commit in one SQL transaction. Projection retains message ordering and
stale-generation auditability. Focused lifecycle/repository race tests, SQL guard, persistence
conformance, and lint pass; PostgreSQL execution remains environment-dependent.
