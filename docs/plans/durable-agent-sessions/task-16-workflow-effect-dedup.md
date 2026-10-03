---
id: "16-workflow-effect-dedup"
title: "Deduplicate projected workflow effects"
status: complete
wave: 16
depends_on: ["15-inbox-projection"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
---

# Task 16: Deduplicate projected workflow effects

## Summary

Consume durable effect intents once at each authoritative workflow transition.

## In scope

- Claim inbox projection intents through existing turn, queue, and environment-generation guards.
- Persist unique effect identity with authoritative state transitions. Keep unrelated bus consumers unchanged.
- Reconcile uncertain external actions instead of replaying MCP, Git, or provider calls.
- Preserve completed-task follow-up behavior: no repeated final-step action and no implicit task reopening.
- Add crash fixtures around intent claim, state transition, notification delivery, and consumer restart.

## Out of scope

Inbox storage implementation and a replacement task queue.

## Acceptance

- Repeated terminal intents advance a turn or workflow once.
- A stale owner cannot complete a newer turn or release its queue.
- A crash after a state transition does not repeat an external side effect.

## Verification

Run from the repository root. New test names describe required evidence, not existing passing tests.
Use TDD for implementation. Record the failing assertion before the implementation result.

```bash
(cd apps/backend && rtk go test -tags fts5 -race ./internal/orchestrator/... ./internal/workflow/... ./internal/task/repository/... -count=1)
(cd apps/backend && rtk go run ./cmd/sqlguard ./internal)
(cd apps/backend && rtk go test -tags fts5 -race ./internal/persistence ./internal/persistence/storeconformance -count=1)
rtk make -C apps/backend lint
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Target evidence in `apps/backend/internal/orchestrator/agent_delivery_projection_test.go`:

- `TestReplayedTerminalIntentAdvancesOnce`: `AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2`.

Run conformance with a provisioned PostgreSQL test database and `KANDEV_TEST_POSTGRES_DSN` set.
A skipped PostgreSQL suite does not satisfy acceptance.
Include fresh, migration replay, and previous-stable upgrade results.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/agent_delivery_projection_test.go`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/workflow/`
- `apps/backend/internal/orchestrator/agent_delivery_projection_test.go` (new tests or extensions).

## Dependencies

[Task 15](task-15-inbox-projection.md).

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

Implemented durable effect-key storage and attached authoritative keys to workflow transition and
compare-and-swap admission boundaries. Replayed terminal intents are suppressed without repeating
the transition or external side effect. Orchestrator/repository race tests, SQL guard, persistence
conformance, and lint pass.
