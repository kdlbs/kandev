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
blocked by the active-session guard and the backend repeats a diagnostic that
the UI never receives.

## Broken behavior

`waitForPromptDone` detects five minutes without events but only logs the
condition every 30 seconds. A top-level shell tool that remains `in_progress`
can therefore hold the prompt waiter and session state indefinitely even
though Kandev already has a bounded cancel-escalation path capable of releasing
the turn.

## What

- Kandev checks running prompts for inactivity once per 60 seconds.
- After five minutes without agent events, Kandev creates at most one
  user-visible notice for that prompt generation.
- The notice says Kandev is still waiting and includes the active top-level
  tool's display title or name when available. It does not assert that the tool
  failed and does not include raw command arguments.
- The notice is a single compact inline row with muted neutral copy and a
  neutral **Cancel turn** action. It has no warning/error colors, alert icon,
  tinted background, or alert-card treatment.
- On phones, **Cancel turn** remains inline and content-width rather than
  becoming a full-width row, while retaining a minimum 44px touch height.
  Activating it uses the existing `agent.cancel` request.
- The notice remains visible and actionable while the affected prompt's
  `turn_id` is the active turn in a `RUNNING` session, including after a page
  reload. It is hidden when that turn settles or a later turn becomes active.
- Detection alone does not change task state, session state, prompt admission,
  or process liveness.
- The backend logs the first stall detected for a prompt generation and does
  not emit another notice or log entry on every watchdog check.

## Failure modes

- If active tool identity is unavailable, the generic notice and cancel action
  still appear.
- If the agent acknowledges cancellation, normal cancellation reconciliation
  makes the session input-ready.
- If the agent does not acknowledge cancellation, the existing bounded
  cancel-escalation path releases the prompt and reconciles the session.
- If notice-message persistence fails, the failure is logged without changing
  or terminating the running session.

## Scenarios

- **GIVEN** a running prompt whose top-level shell tool is `in_progress`,
  **WHEN** no agent event arrives for five minutes, **THEN** one compact neutral
  notice names that tool and offers **Cancel turn** while the session remains
  `RUNNING`.
- **GIVEN** a running prompt with no known active tool, **WHEN** no agent event
  arrives for five minutes, **THEN** one generic notice offers **Cancel turn**
  while the session remains `RUNNING`.
- **GIVEN** a notice already exists for the active prompt generation,
  **WHEN** subsequent watchdog checks observe the same stall, **THEN** Kandev
  creates no duplicate notice and emits no repeated stall log.
- **GIVEN** a persisted notice belongs to an earlier prompt generation,
  **WHEN** a later prompt is `RUNNING`, **THEN** the earlier notice is not
  rendered and a delayed stall event cannot create a new notice for it.
- **GIVEN** a stall notice is visible, **WHEN** the user activates
  **Cancel turn**, **THEN** the existing cancellation path settles the turn and
  the session becomes available for new input without a backend restart.
- **GIVEN** a stall notice is visible on a phone viewport, **WHEN** the user taps
  **Cancel turn**, **THEN** the same cancellation outcome is reachable through
  an inline, content-width touch target of at least 44px.
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
