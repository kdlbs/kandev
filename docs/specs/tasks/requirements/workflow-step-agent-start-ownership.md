---
status: draft
system: tasks
created: 2026-08-05
updated: 2026-09-18
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
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.7:** If cancellation ends without provider stop confirmation, workflow reset shall fail without replacing provider context or dispatching the automatic prompt.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.8:** If a provider receives a context-reset request but does not respond, that request shall fail within 10 seconds. An earlier caller deadline shall take precedence. The failed request shall release the session reset guard instead of waiting indefinitely for a response.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.9:** If workflow context reset fails, the session shall show a persistent error on desktop and mobile. The error shall identify the failed reset and explain that the step prompt did not start. After reset cleanup, session deletion shall remain available. A failed reset shall not appear as a successful context reset.

### REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003: Single-use task-description fallback

**Intent:** Start an unprompted workflow session from its task description without
repeating that description during later workflow entries.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.1:** When an unprompted
session enters an automatic-start step with no step prompt, the system shall send
the task description once. Admission of this fallback shall be atomic with the
session's first-prompt boundary.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.2:** When a prompted session
enters an automatic-start step with no step prompt, the system shall not send the
task description again. A concurrent direct user prompt and automatic fallback
shall not both qualify as the first prompt.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.3:** When an
automatic-start step has a step prompt, the system shall send the evaluated step
prompt regardless of earlier session prompts.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.4:** When fallback
suppression removes all textual prompt content and no queued attachment remains,
the system shall not create a user message or dispatch an empty agent turn. A
queued attachment-only handoff shall remain durable user input and shall be
dispatched with its attachment metadata.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.5:** The automatic
`on_enter` path and the explicit workflow-step launch path shall use the same
task-description fallback rule.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.6:** ACP and passthrough
sessions shall use the same task-description fallback rule.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.7:** If prompt-history
inspection fails, the system shall stop the automatic prompt and expose the
existing workflow-start error behavior.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-003.8:** When a workflow move
includes a textual handoff, the new session shall receive the handoff once in
its first agent message. When the target automatic-start step has a non-empty
prompt, the same message shall also contain the evaluated step prompt. The
stored user message and the dispatched message shall retain the same visible
content.

### REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004: Immediate-launch placement

**Intent:** Place a new task in the workflow step that owns its requested agent
start, regardless of the selected agent mode.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.1:** When task creation
starts an agent immediately, the system shall place the task in the first
positional step with an `auto_start_agent` entry action. Plan mode shall not
change this destination.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.2:** When no workflow step
has an `auto_start_agent` entry action, an immediate agent start shall use the
configured start step. If no start step exists, it shall use the first
positional step.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.3:** When a creator supplies
an explicit workflow step, the system shall use that step instead of an
intent-derived destination.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-004.4:** Desktop and mobile task
creation shall apply the same immediate-launch placement rule.

### REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005: Automatic prompt preservation after startup failure

**Intent:** Retain an automatic workflow prompt when its agent launch fails before prompt delivery.
This draft extension addresses [issue #3753](https://github.com/kdlbs/kandev/issues/3753).

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.1:** When an admitted automatic launch fails asynchronously before prompt delivery, the system shall preserve its prompt for recovery.
  The preserved input shall include handoff text, attachments, references, plan mode, and workflow origin.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.2:** After successful recovery, the system shall deliver the preserved prompt once through normal queue admission.
  It shall retain one existing user message and respect a paused queue.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.3:** Duplicate or superseded launch failures shall not duplicate queued input or affect a successor turn.
  Cancellation, completion, archive, deletion, and a superseding workflow entry shall prevent stale prompt recovery.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.4:** Prompt preservation shall retain the existing launch-error classification and recovery actions.
  It shall not initiate another launch from the failure callback or turn permanent rejection into an automatic retry loop.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.5:** After successful queue persistence, the preserved prompt shall survive a backend restart.
  If queue admission fails, the same launch attempt shall retry once while it
  still owns the preservation claim. After that retry fails, the launch error
  shall remain visible and diagnostics shall identify the preservation failure
  without exposing prompt content.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005.6:** Successful starts and synchronous permanent rejections shall not create asynchronous recovery entries.
  Existing synchronous busy-error recovery shall retain its current behavior.

This extension excludes a backend crash before the asynchronous failure callback persists the prompt.
It also excludes replay after ambiguous provider acceptance, automatic repair of historical orphaned messages, and new recovery controls.

### REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006: Initial user prompt on an explicit step

**Intent:** Apply the selected step's user-message transition before an immediate creation prompt reaches the agent.
This draft extension addresses [issue #3804](https://github.com/kdlbs/kandev/issues/3804).

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006.1:** When REST or MCP creation requests an immediate start with an explicit step and non-empty prompt, its `on_turn_start` transition shall precede prompt delivery.
  Initial placement shall still use the explicit step. The configured transition determines the subsequent step.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006.2:** The first prompt shall use the resulting step, session, profile, and applicable session settings.
  Destination automatic start shall not send a competing prompt. The initial input shall retain existing prompt-composition and attachment behavior.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006.3:** One initial user prompt shall evaluate the trigger once, including queue delivery and passthrough running notifications.
  A later user turn shall retain ordinary trigger behavior.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006.4:** When the transition queues the task for WIP admission, the initial prompt shall wait for admission.
  It shall remain available for delivery without a second user message or repeated transition.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006.5:** Creation without an explicit step shall retain automatic destination selection.
  Creation without immediate start or non-empty text shall not gain an additional trigger from this change.
  Workflow automatic starts and ordinary message submission shall not gain an additional trigger.
- **AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-006.6:** If turn-start processing or destination-session resolution fails, the creation prompt shall not launch against stale state.
  Existing task/session error handling shall expose the failed start. Existing terminal-state and creation-settlement guards shall remain effective.

The extension covers immediate REST and MCP dispatch. Dependency-deferred creation, historical stuck tasks, and changes to workflow action semantics are excluded.
The [initial creation prompt package](../../../plans/task-create-initial-turn-start/plan.md) owns implementation and regression evidence.

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

## Asynchronous preservation delivery status

The original [start-ownership package](../../../plans/workflow-step-agent-start-ownership/plan.md)
closed the synchronous failure path. Requirement
`REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-005` defines the asynchronous
recovery contract. The [asynchronous prompt preservation package](../../../plans/workflow-async-start-prompt-preservation/plan.md)
delivers it through its completed work order.

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
- Changing placement for a plan-only prepared session that does not request an
  immediate agent start. This path continues to use the first workflow step by
  position.
