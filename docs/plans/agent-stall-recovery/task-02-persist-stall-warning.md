---
id: "02-persist-stall-warning"
title: "Persist an actionable stall warning"
status: pending
wave: 2
depends_on: ["01-detect-stalled-prompt"]
plan: "plan.md"
spec: "../../specs/agent-stall-recovery/spec.md"
---

# Task 02: Persist an actionable stall warning

## Acceptance

- The orchestrator consumes `agent.stalled` and creates one warning status
  message without changing task, session, or process state.
- Copy uses only the tool display title/name, falling back to a generic warning.
- Metadata exposes a running-only **Cancel turn** `agent.cancel` action with the
  affected `session_id` and stable test ID.

## Verification

- `cd apps/backend && go test -race -run 'TestHandleAgentStalled' ./internal/orchestrator`
- `cd apps/backend && go test -race -run 'TestWatcher.*AgentStalled' ./internal/orchestrator/watcher`

The handler test must first fail because no warning is persisted, then pass
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

- Spec scenarios for tool-specific and generic warnings
- Plan section `Persist the actionable warning`
- Existing transient-retry and recoverable-failure action-message builders

## Output contract

Report the RED assertion, warning copy and metadata, proof that no state
transition occurred, exact test results, files changed, blockers, and risks.
Mark this task `done` and update its plan checkbox in the same conversation.
