---
id: "01-statusless-turn-start"
title: "Ignore status-less updates at turn start"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SESSION-TURN-SETTLEMENT-002
acceptance_criteria:
  - AC-TASKS-SESSION-TURN-SETTLEMENT-002.6
system_design:
  - ../../specs/tasks/system-design/session-turn-settlement.md
---

# Task 01: Ignore status-less updates at turn start

## Summary

A `tool_update` with an empty status no longer flips a Ready execution to
Running, so the backend cannot mark a session busy for a turn the agentctl
finalizer will never close.

## In scope

- In `recordActivity`, skip the Ready-to-Running flip for a `tool_update` whose
  `ToolStatus` is empty, next to the existing `isTerminalToolUpdate` check.
- Keep the activity timestamp update for that event.

## Out of scope

- The agentctl finalizer and provider boundary, delivered by #4988.
- Prompt-owned turns, which never reach the flip.

## Acceptance

- A Ready execution stays Ready after a status-less `tool_update`, and publishes
  no `AgentRunning`.
- A `tool_update` with `in_progress`, a `tool_call`, or a `message_chunk` still
  flips it, and the existing wakeup tests still pass.
- Write the regression first and confirm it fails on `main` for the expected
  reason: the execution becomes Running.

## Verification

```bash
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath ./internal/agent/runtime/lifecycle -run 'Wakeup' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/manager_events.go apps/backend/internal/agent/runtime/lifecycle/wakeup_statusless_test.go
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/wakeup_statusless_test.go` (new;
  follow `wakeup_simulated_test.go`, especially
  `TestWakeup_LateTerminalToolUpdateDoesNotFlipToRunning`)

## Dependencies

None

## Risks

- An update with real progress but no status would not start a turn. The ACP
  adapter assigns `in_progress` whenever an update carries a title, input,
  content, locations, or recognized shell output, so this applies to metadata
  only.

## Parallelism

`sequential`

## Inputs

- [Backend turn start](../../specs/tasks/system-design/session-turn-settlement.md#backend-turn-start)
- `wakeup_simulated_test.go`

## Results

Implemented. `recordActivity` now treats a `tool_update` with an empty
`ToolStatus` like a terminal update: it refreshes activity timestamps but does
not flip a Ready execution to Running or publish `agent.running`.
`wakeup_statusless_test.go` covers the status-less case and the positive cases
(message chunk, tool call, in-progress tool update).

Verified:

```bash
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath ./internal/agent/runtime/lifecycle -run 'Wakeup' -count=1)
```
