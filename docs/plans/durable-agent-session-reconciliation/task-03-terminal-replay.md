---
id: "03-terminal-replay"
title: "Reconcile ordered replay and terminal recovery"
status: complete
wave: 3
depends_on:
  - 02-adoption-identity
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/agents/system-design/harness-session-continuity.md
---

# Task 03: Reconcile ordered replay and terminal recovery

## Summary

Use one ordered durable recovery path for compatible adopted instances.

## In scope

Use one ordered durable recovery path for compatible adopted instances.
- Commit normalized events once before any live or retained publication. Audit adapter output and direct error/exit producers.
- Use the durable journal path for compatible peers. Do not call `applyRecoveredTurnOutcome` directly for them.
- Preserve `ControlTurnID` only for legacy retention. Keep durable turn/submission/event identities immutable.
- Replay to a captured high-water mark through SQL inbox, canonical projection, lifecycle effects, and ACK.
- Drain more than one replay page with bounded memory. Never jump from the first 1000 events to live delivery.
- Keep startup guards until identity, recovered state, and required replay are reconciled. Surface progress during the bounded deadline.
- Ensure duplicate/stale terminal events cannot mutate a successor or release queued work twice.
- Keep legacy retained outcomes and their current deduplication for positively identified legacy peers.
- A durable replay or journal error leaves recovery blocked. Never fall back to terminal-only recovery.
- Separate backend detach from instance stop: detach leaves the journal and harness alive; actual stop releases blocked writers and storage owners.

## Out of scope

New survival runtimes, new flags, automatic context continuation, and unrelated CI fixes.

## Acceptance

1. A terminal outcome follows preceding canonical transcript effects and advances its workflow once across ACK loss and backend restart.
2. Every durable event producer commits once before publication, with bounded buffering and prompt cancellation preserved.
3. Legacy recovery still works. Durable failures cannot bypass SQL projection or silently downgrade.

## Test cases

Add `durable_adoption_replay_test.go` in lifecycle:
- `TestDurableAdoptionReplaysBeforeTerminal`: delayed canonical write must prevent completion and queue release.
- `TestDurableAdoptionTerminalCrashBoundaries`: fail after inbox, canonical write, effect commit, and before ACK.
- `TestDurableAdoptionReplayMultiplePages`: at least 1001 events, concurrent live tail, exact order, no gap.
- `TestDurableAdoptionLateTerminalCannotCompleteSuccessor`: duplicate and delayed old outcomes after a new turn starts.
- `TestDurableAdoptionLegacyTerminalFallback`: absence of protocol versus errors from a v1 peer.
Add direct producer tests in process for journal failure, one commit, full buffer, detach, and actual stop.
Reuse `TestReplayedTerminalIntentAdvancesOnce` and journal cursor tests. Add a real SQLite projection test for crash recovery.

## Verification

Run from the repository root. Proposed test files must exist before these commands pass.

```bash
(cd apps/backend && go test -race ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/lifecycle -run 'TestDurableAdoption|Test.*Delivery|Test.*TurnOutcome|TestJournal|Test.*Blocking|Test.*Survival' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator ./internal/task/repository/sqlite -run 'TestReplayedTerminal|Test.*AgentDelivery|Test.*DurableAdoption' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && golangci-lint run ./... --new-from-rev=origin/main --timeout=5m)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/process/blocking_send.go`
- `apps/backend/internal/agentctl/server/process/turn_outcome.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/api/durable_delivery.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_recovery_turn_outcome.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go`
- `apps/backend/internal/agent/runtime/lifecycle/streams.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_stream.go`
- `apps/backend/internal/orchestrator/agent_delivery_projection_test.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go`

## Dependencies

Task 02 must pass its acceptance before this work begins.

## Inputs

- [Plan and baseline](plan.md).
- [Adoption contract](../../specs/platform/system-design/durable-agent-delivery.md#surviving-process-adoption).
- Read the scoped AGENTS.md files and the tests next to every changed implementation.

## Risks

Direct lifecycle calls can bypass the SQL effect guard even when the two event types compile together.
A missing terminal slot does not prove a durable stream is idle or caught up.
PostgreSQL behavior requires a real DSN if any SQL method changes. A skipped test is not acceptance.

## Parallelism

`sequential`

## Results

Completed 2026-09-14. Durable recovery replays bounded pages through the
existing inbox, canonical projection, lifecycle, and ACK pipeline. Canonical
events now retain the stream coalescer, and journal, projection, effect, owner,
sequence, and terminal paths fail closed before acknowledgement or lifecycle
advancement.

The multi-page replay, gap, effect-read, canonical-projection, journal,
process, API, store-conformance, SQL guard, exact Task 03 backend, and changed
package Go lint checks passed. Legacy terminal recovery remains limited to
positively identified legacy peers.
