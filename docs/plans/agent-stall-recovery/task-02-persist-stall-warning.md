---
id: "02-persist-stall-warning"
title: "Persist an actionable stall notice"
status: done
wave: 2
depends_on: ["01-detect-stalled-prompt"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-stall-recovery.md"
---

# Task 02: Persist an actionable stall notice

## Acceptance

- The orchestrator consumes `agent.stalled` and creates one neutral status
  message without changing task, session, or process state.
- Copy says Kandev is still waiting, uses only the tool display title/name, and
  falls back to a generic notice without asserting failure.
- Metadata exposes a running-only **Cancel turn** `agent.cancel` action with the
  affected `session_id`, neutral styling, and stable test ID.

## Verification

- `make -C apps/backend test` (regressions: `TestHandleAgentStalled` in `internal/orchestrator` and `TestWatcher.*AgentStalled` in `internal/orchestrator/watcher`)

The handler test must first fail because no notice is persisted, then pass
with exact metadata assertions and unchanged session state.

## Files likely touched

- `apps/backend/internal/orchestrator/watcher/watcher.go`
- `apps/backend/internal/orchestrator/watcher/watcher_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_stall.go`
- `apps/backend/internal/orchestrator/event_handlers_stall_test.go`

## Dependencies

Task 01.

## Parallelism

Sequential. It consumes Task 01's event contract and defines Task 03's message
metadata contract.

## Inputs

- Spec scenarios for tool-specific and generic notices
- Plan section `Persist the actionable notice`
- Existing transient-retry and recoverable-failure action-message builders

## Output contract

Report the RED assertion, notice copy and metadata, proof that no state
transition occurred, exact test results, files changed, blockers, and risks.
Mark this task `done` and update its plan checkbox in the same conversation.
