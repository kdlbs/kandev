---
status: draft
system: tasks
created: 2026-08-05
owners:
  - Kandev
---
# Workflow Step Agent Start Ownership Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-001: Workflow Step Agent Start Ownership

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

### REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002: Context reset turn quiescence

**Intent:** A workflow step can replace agent context without losing turn completion or blocking later session operations.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.1:** When a workflow context reset finds an active turn, the system shall stop and reconcile that turn before it replaces the provider context.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.2:** The system-owned stop shall not create a user-cancellation message or evaluate configured user-cancellation completion actions.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.3:** When the reset succeeds, the next automatic step prompt shall reach the new provider context without waiting for the replaced turn.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.4:** When turn quiescence or context replacement fails, the system shall not dispatch the automatic step prompt.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.5:** When a prompt waits for an unresolved dispatch-only completion, the wait shall end within a bounded period and release session admission.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.6:** A delayed completion from the replaced turn shall not complete or release a later prompt generation.

## Migrated source detail

## Why

A workflow step whose `on_enter` combines `reset_agent_context` and
`auto_start_agent` can have two independent paths start the same agent
execution. The second start is rejected, the task is marked `FAILED`, the agent
is force-killed, and the step prompt that was already written to chat history is
never delivered. Recovery boots a fresh agent, so the user is left looking at a
healthy "Resumed agent" banner above a session that will never do anything.

Observed in a local run, where the whole sequence — step move, double start,
force-kill, idle reboot — completed in about 13 seconds.

## Broken behavior

`resetAgentContext` decides whether to restart purely on the presence of an
agent execution ID. A `CREATED` session whose execution is workspace-only —
prepared but never started — therefore has its subprocess *started* by the reset
path, which reaches agentctl as a restart.

`markIdleAfterReset` only flips sessions in `RUNNING`/`STARTING`, so the session
stays `CREATED`. Its own comment documents the opposite invariant: that
`resetAgentContext` early-returns for `CREATED` "without restarting".

`autoStartStepPrompt` then reads `CREATED`, concludes the agent was never
started, and calls `StartCreatedSession` → `StartAgentProcess` against the
now-running execution. agentctl's `Manager.Configure` rejects it with
`cannot configure while agent is running`; the executor marks the session
`FAILED` and force-stops the agent.

The prompt recorded by `recordAutoStartMessage` is not requeued on that failure
— only a taken handoff message is — so it is lost. Recovery's `session/load`
then fails with `-32002 Resource not found` because the ACP session died with
the killed process, and the fallback `session/new` boots an agent that is idle
and unprompted.

## What

- On a step entry, exactly one path starts the agent subprocess for a session.
- Context reset performs no start or restart for a session that has never been
  prompted (`CREATED`). Such a session has no agent conversation to clear, and
  the process the auto-start path launches already begins on a new ACP session.
- Context reset for a session that has been prompted continues to restart the
  subprocess and clear `acp_session_id` exactly as it does today.
- When a `CREATED` launch fails *synchronously* with a busy or
  already-running condition, the recorded prompt is queued for the session so
  the existing boot-ready drain delivers it once the session is promptable
  again. Permanent rejections (Office scheduler guard, missing agent profile)
  are not queued — nothing would ever drain them.
- A failed start still marks the session `FAILED` and surfaces the error.

## Failure modes

- Reset is skipped for a `CREATED` session and auto-start then fails for an
  unrelated reason: the session is `FAILED` with the error surfaced, and the
  prompt is queued rather than orphaned.
- The step has `reset_agent_context` but no `auto_start_agent` and the session
  is `CREATED`: no restart occurs and the session remains promptable. No agent
  context is lost, because none exists.
- The session is passthrough and `CREATED`: the same skip applies; the CLI has
  no conversation to reset before its first prompt.
- Execution lookup fails or returns an empty ID: unchanged — reset is skipped,
  as today.
- Queueing the prompt after a failed start itself fails: the failure is logged
  and the session's `FAILED` state is unchanged.

## Scenarios

- **GIVEN** a session in `CREATED` with a prepared, not-yet-started agent
  execution, **WHEN** the task enters a step whose `on_enter` has both
  `reset_agent_context` and `auto_start_agent`, **THEN** the reset performs no
  subprocess restart, auto-start performs the single start, and the step prompt
  reaches the agent.
- **GIVEN** a session in `CREATED` with a prepared execution, **WHEN** the task
  enters a step with `reset_agent_context` and no `auto_start_agent`, **THEN**
  no restart occurs and the session remains promptable.
- **GIVEN** a session in `RUNNING` with a started agent execution, **WHEN** the
  task enters a step with `reset_agent_context`, **THEN** the subprocess is
  restarted and `acp_session_id` is cleared, unchanged from today.
- **GIVEN** a passthrough session in `CREATED`, **WHEN** the task enters a step
  with `reset_agent_context`, **THEN** no restart occurs.
- **GIVEN** a `CREATED` session with no in-memory execution (the shape after a
  backend restart), **WHEN** its auto-start launch fails synchronously because
  a concurrent path already owns the start, **THEN** the recorded prompt is
  queued for the session rather than dropped, and the session's later
  promptable transition delivers it.
- **GIVEN** a `CREATED` launch rejected permanently (Office scheduler guard),
  **WHEN** the failure is handled, **THEN** the error is returned and no
  message is queued.

## Known gap

Prompt preservation across an **asynchronous** start failure is not covered
here. `startAgentOnExistingWorkspace` ends with `startAgentProcessAsync` and
returns `nil`, so a session with a prepared execution — the incident's own shape
— never surfaces a synchronous error for the clause above to catch. That failure
is handled entirely by `handleAgentProcessStartFailure` →
`Service.handleAgentStartFailed`, which has no access to the prompt the launch
was carrying and therefore cannot re-queue it.

Task 01 removes the only known trigger for that path on a first-turn launch, so
the incident cannot recur through it. The residual gap — any *other* async start
failure on a first-turn launch drops the step prompt — is a distinct pre-existing
defect. Closing it needs a pending-prompt handle the async failure path can
read, which is a design change beyond this repair.

## Out of scope

- Relaxing agentctl's `Manager.Configure` running-process guard. Reconfiguring
  command or environment under a live subprocess is unsafe; the guard is
  correct and stays as-is.
- Native ACP `session/reset` support in the adapter. The
  reset-unsupported-fallback-to-restart path is unchanged.
- The `-32002 Resource not found` resume failure, which is a consequence of the
  force-kill rather than an independent defect.
- Changing task `FAILED` semantics, the reconciliation path, or the
  lazy-recovery boot that follows a backend restart.
