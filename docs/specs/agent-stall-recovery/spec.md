---
status: draft
created: 2026-07-29
owner: Kandev
---

# Agent Stall Recovery

Decision:
[ADR-2026-07-29-agent-stall-user-controlled-recovery](../../decisions/2026-07-29-agent-stall-user-controlled-recovery.md)

## Why

An agent turn can remain `RUNNING` forever after its active tool stops emitting
events. Users currently see only “Agent is running,” while Resume is correctly
blocked by the active-session guard and the backend repeats a warning that the
UI never receives.

## Broken behavior

`waitForPromptDone` detects five minutes without events but only logs the
condition every 30 seconds. A top-level shell tool that remains `in_progress`
can therefore hold the prompt waiter and session state indefinitely even
though Kandev already has a bounded cancel-escalation path capable of releasing
the turn.

## What

- Kandev checks running prompts for inactivity once per 60 seconds.
- After five minutes without agent events, Kandev creates at most one
  user-visible warning for that prompt generation.
- The warning says the agent may be stuck and includes the active top-level
  tool's display title or name when available. It does not include raw command
  arguments.
- The warning provides a prominent **Cancel turn** action on desktop and
  mobile. Activating it uses the existing `agent.cancel` request.
- The warning remains visible and actionable while the affected session is
  `RUNNING`, including after a page reload, and is hidden when the session
  leaves `RUNNING`.
- Detection alone does not change task state, session state, prompt admission,
  or process liveness.
- The backend logs the first stall detected for a prompt generation and does
  not emit another warning or log entry on every watchdog check.

## Failure modes

- If active tool identity is unavailable, the generic warning and cancel action
  still appear.
- If the agent acknowledges cancellation, normal cancellation reconciliation
  makes the session input-ready.
- If the agent does not acknowledge cancellation, the existing bounded
  cancel-escalation path releases the prompt and reconciles the session.
- If warning-message persistence fails, the failure is logged without changing
  or terminating the running session.

## Scenarios

- **GIVEN** a running prompt whose top-level shell tool is `in_progress`,
  **WHEN** no agent event arrives for five minutes, **THEN** one warning names
  that tool and offers **Cancel turn** while the session remains `RUNNING`.
- **GIVEN** a running prompt with no known active tool, **WHEN** no agent event
  arrives for five minutes, **THEN** one generic warning offers **Cancel turn**
  while the session remains `RUNNING`.
- **GIVEN** a warning already exists for the active prompt generation,
  **WHEN** subsequent watchdog checks observe the same stall, **THEN** Kandev
  creates no duplicate warning and emits no repeated stall log.
- **GIVEN** a stalled warning is visible, **WHEN** the user activates
  **Cancel turn**, **THEN** the existing cancellation path settles the turn and
  the session becomes available for new input without a backend restart.
- **GIVEN** a stalled warning is visible on a phone viewport, **WHEN** the user
  taps **Cancel turn**, **THEN** the same cancellation outcome is reachable
  through a full-width touch target of at least 44px.
- **GIVEN** a quiet but legitimate long-running turn, **WHEN** the inactivity
  threshold passes and the user does not cancel, **THEN** Kandev leaves the
  turn and process running.

## Out of scope

- Automatically timing out, failing, cancelling, or killing quiet turns.
- Making the inactivity threshold user-configurable.
- Provider-specific repair of shell commands that hold child-process streams
  open.
- Treating event silence as proof that an agent process crashed.
- Allowing Resume or direct prompt admission while the session remains
  `RUNNING`.
