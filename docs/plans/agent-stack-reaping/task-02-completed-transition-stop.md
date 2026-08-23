---
id: "02-completed-transition-stop"
title: "Guarded agent stack stop on task COMPLETED"
status: completed
wave: 1
depends_on: ["01-agent-stack-reaping-flag"]
plan: "plan.md"
spec: "../../../specs/platform/agent-stack-reaping.md"
---

# Task 02: Guarded agent stack stop on task COMPLETED

## Acceptance

- A task transition to COMPLETED schedules a detached sweep that stops the
  task's idle sessions' stacks. This covers the sessions `StopByTaskID` does
  not list (IDLE, COMPLETED) and `markTaskCompletedForTerminalStep`, which
  never stopped agents at all.
- The REVIEW transition schedules nothing. `writeTaskReviewState` and
  `writeTaskReviewStateOnCancel` run after every turn, so reaping there would
  remove warm-stack reuse rather than reclaim idle memory.
- `StopTask` and `CompleteTask` keep their original stop-then-persist ordering:
  a failed task-state write must not skip the agent teardown the user asked
  for.
- The stop primitive is fail-closed: flag off, working session state, active
  turn, prompt inside its admission window, turn service unavailable, or no
  live execution all mean skip.
- Sweeps run on the service-owned `agentStackSweeper`, whose context is
  cancelled and whose workers are joined by `Service.Stop`.
- `promptTask` holds `beginPromptAdmission` from before `ensureSessionRunning`
  until the prompt finishes, so a sweep cannot stop the execution a prompt is
  about to use.
- Stops are graceful (`force=false`) with reason
  `agent stack reaping: task completed`, executed under the per-session
  lifecycle lock.

## Verification

- `cd apps/backend && go test ./internal/orchestrator -run 'TestAgentStackReaping|TestAgentStackSweeper|TestStopTask_StopsAgents'`

## Files

- `apps/backend/internal/orchestrator/agent_stack_reaper.go` (new)
- `apps/backend/internal/orchestrator/event_handlers_children_completed.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/service.go`
