---
id: "01-complete-agent-started-turns"
title: "Complete agent-started turns"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SESSION-TURN-SETTLEMENT-002
acceptance_criteria:
  - AC-TASKS-SESSION-TURN-SETTLEMENT-002.1
  - AC-TASKS-SESSION-TURN-SETTLEMENT-002.2
  - AC-TASKS-SESSION-TURN-SETTLEMENT-002.3
  - AC-TASKS-SESSION-TURN-SETTLEMENT-002.4
  - AC-TASKS-SESSION-TURN-SETTLEMENT-002.5
system_design:
  - ../../specs/tasks/system-design/session-turn-settlement.md
---

# Task 01: Complete agent-started turns

## Summary

A status-less tool update stops blocking the idle finalizer, and the provider end-of-turn frame closes an agent-started turn. A message queued during that turn is then delivered.

## In scope

- Ignore empty-status updates in `trackTool` and in the content check.
- Treat an origin-bearing `usage_update` as the end-of-turn boundary when no prompt turn owns the session.
- Add a mock-agent scenario that emits the recovered frame shape, then a follow-up turn.
- Write the regression test first and confirm it fails with `activeTools` holding the launch ID.

## Out of scope

- Workload settlement (Tasks 04 and 05).
- Prompt-owned turns.

## Acceptance

- The recovered seq 1173 shape followed by a tool cycle produces exactly one synthetic completion.
- A foreground tool with an active status still holds the turn open during silence, and existing tests in `async_complete_test.go` pass unchanged.
- The E2E flow delivers the queued message without user action.

## Verification

```bash
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp -run 'AsyncTurn|HandleACPUpdate' -count=1)
(cd apps/backend && make build-mock-agent)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/queue-after-background-followup.spec.ts
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_async_complete.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/async_complete_status_test.go`
- `apps/backend/cmd/mock-agent/detached_background.go`
- `apps/backend/cmd/mock-agent/scenarios.go`
- `apps/backend/cmd/mock-agent/async_followup.go`
- `apps/backend/cmd/mock-agent/handler.go`
- `apps/backend/cmd/mock-agent/main.go`
- `apps/web/e2e/tests/chat/queue-after-background-followup.spec.ts`

## Dependencies

None

## Risks

- The provider boundary could arrive before trailing frames. The idle timer stays armed so those frames extend the window.

## Parallelism

`sequential`

## Inputs

- [Agent-started turn finalization](../../specs/tasks/system-design/session-turn-settlement.md#agent-started-turn-finalization)
- `async_complete_test.go` patterns and `cmd/mock-agent/AGENTS.md`

## Results

Done.

- `trackTool` ignores a status-less `tool_call_update`, and
  `maybeScheduleAsyncTurnComplete` never creates or extends an agent-started turn
  for one. `isAsyncTurnContentEvent` keeps treating it as prompt content, so a
  status-less update during a live prompt still receives its `PromptGeneration`
  (pinned by `TestHandleACPUpdate_StatuslessToolUpdateKeepsPromptGeneration`).
- `noteAsyncTurnProviderBoundary` clears the finalizer's active tools and arms
  the ordinary idle timer when an origin-bearing `usage_update` arrives and no
  prompt turn owns the session.
- `async_complete_status_test.go` covers the recovered seq 1173 shape followed
  by a tool cycle, a status-less update alone, a status-less update for a live
  foreground tool, prompt attribution, and the provider boundary. The first,
  third, and fourth cases fail before the change.
- `cmd/mock-agent/async_followup.go` adds `/async-followup` (and the
  `/e2e:async-followup` registry entry) for the E2E flow, which passes on
  chromium.
- Review rerun: the managed chromium command covering this flow, Quick Chat
  cancellation, and turn-history conflict passed all 5 tests.
