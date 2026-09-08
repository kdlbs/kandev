---
id: "01-detect-stalled-prompt"
title: "Detect and publish stalled prompts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-stall-recovery.md"
---

# Task 01: Detect and publish stalled prompts

## Acceptance

- A prompt with no agent events for five minutes publishes one
  generation-scoped `agent.stalled` event and remains in progress.
- The event includes active top-level tool display identity when available;
  subagent tools do not replace it and terminal tool updates clear it.
- The watchdog checks every 60 seconds and does not repeat the notice or log
  for the same prompt generation.

## Verification

- `make -C apps/backend test` (regressions: `TestHandleAgentEvent_TracksActiveTopLevelTool` and `TestWaitForPromptDone_PublishesSingleStall` in `internal/agent/runtime/lifecycle`)

The `testing/synctest` regression must first fail because no `agent.stalled`
event is published, then pass without shortening production durations.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/events/types.go`

## Dependencies

None.

## Parallelism

Sequential. Task 02 consumes the event and payload contract defined here.

## Inputs

- Spec sections `What`, `Failure modes`, and the first three scenarios
- ADR decision bullets for advisory detection and once-per-generation logging
- Existing `recordActivity`, `handleToolCallEvent`, `handleToolUpdateEvent`, and
  `waitForPromptDone` patterns

## Output contract

Report the RED assertion, event payload shape, tool-tracking behavior, exact
test result, files changed, blockers, and risks. Mark this task `done` and
update its plan checkbox in the same conversation.
