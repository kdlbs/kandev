---
id: "01-preserve-async-start-prompt"
title: "Preserve the step prompt across an asynchronous start failure"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.1
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.2
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.3
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.4
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.5
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.6
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 01: Preserve the Step Prompt Across an Asynchronous Start Failure

## Summary

Give the asynchronous start-failure path access to the step prompt the launch
was carrying, through a single-use per-session handle, and queue that prompt for
the session's next promptable transition.

## In scope

- Add a pending step-prompt registry to `orchestrator.Service`, holding the
  composed prompt, plan mode, attachments, entity references, step handoff text,
  workflow message origin, task ID, and the recorded-user-message flag. The take
  operation removes the entry and returns it, so exactly one consumer wins.
- Arm the handle in the `CREATED` branch of `autoStartStepPrompt`, immediately
  before `startCreatedSessionWithComposedPrompt`.
- Take and discard the handle on a synchronous launch error, leaving
  `handleCreatedAutoStartLaunchFailure` as the sole preserver on that path.
- Take and discard the handle when the agent process start succeeds.
- Take the handle in `Service.handleAgentStartFailed`, after its cancel-in-flight,
  stale-resume-attempt and drop checks, and queue the prompt without scheduling
  an automatic resume.
- Log a queue-write failure and leave the start-failure projection unchanged.

## Out of scope

- Durable storage of the handle across a backend restart.
- Any change to the start-failure projection, task or session failure states, or
  the recovery surfaces from PRs #3315, #3610 and #3669.
- Passthrough sessions, the non-`CREATED` retry loop, and issue #3336.

## Files

- `apps/backend/internal/orchestrator/` — new file for the pending step-prompt
  registry and its take-once semantics.
- `apps/backend/internal/orchestrator/event_handlers_workflow.go` — arm the
  handle in the `CREATED` branch (around line 5768), disarm on synchronous
  error (around line 5804), and add the queue variant that does not schedule an
  automatic resume (around line 6177).
- `apps/backend/internal/orchestrator/event_handlers_agent.go` — consume the
  handle in `handleAgentStartFailed` (around line 3389) after its existing
  ownership checks.
- `apps/backend/internal/orchestrator/dynamic_launch.go` — discard the handle in
  `handleAgentProcessStarted` (around line 298), before its ceiling-ownership
  early return.

## Acceptance

- A `CREATED` session entering a step with `reset_agent_context` and
  `auto_start_agent`, whose agent process start fails asynchronously, has its
  composed step prompt queued exactly once, with attachments, references and
  handoff text intact.
- The queued entry carries the recorded-user-message flag, so the drain writes
  no second chat-history row.
- No agent process is started by the preservation path, and no automatic resume
  is scheduled from it.
- A successful start leaves nothing queued, and a later start failure on the
  same session queues nothing.
- The synchronous failure path behaves exactly as PR #2263 left it and queues
  the prompt once, not twice.
- A start failure from a superseded execution, a stale resume attempt, or a
  session with cancellation in progress leaves the handle armed and queues
  nothing.

## Validation

- `make -C apps/backend test`
- `make -C apps/backend lint`
