---
status: active
system: tasks
created: 2026-08-02
owners:
  - kandev
---
# Cancelled Turn Completion Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001: Cancelled Turn Completion

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

Decision: [ADR-2026-08-02-explicit-user-cancel-completion](../../../decisions/2026-08-02-explicit-user-cancel-completion.md)

## Why

Cancelling an active Kanban agent turn currently stops the agent and marks the runtime task state ready for review, but it leaves the task on its existing workflow step. Users who configure a step to move on turn completion need an explicit choice over whether their own cancellation should finish that step or merely pause it.

## What

- Each workflow step exposes `cancel_triggers_turn_complete`, a boolean setting controlling whether an explicit user cancellation evaluates that step's existing `on_turn_complete` actions.
- The setting defaults to `false` for newly created custom steps, imported files that omit it, and existing persisted workflow steps. Existing workflows are not backfilled.
- The embedded `simple` Kanban template enables the setting on `Backlog` and `In Progress`, the steps that already define `on_turn_complete` transitions. New workflows instantiated from that template inherit the enabled value.
- The policy applies only when a user invokes the visible task-session cancel action. Silent clarification pauses, peer-message interrupts, parent-task stops, task archival, runtime teardown, provider errors, crashes, and other system-owned cancellations never evaluate completion actions through this setting.
- A configured user cancellation reuses the ordinary `on_turn_complete` action pipeline. It runs source-step `on_exit`, applies at most one transition, and runs destination-step `on_enter` with the same first-transition-wins and side-effect behavior as a normal completed turn.
- A configured user cancellation is a human completion decision and does not require `step_complete_kandev`, even when `auto_advance_requires_signal=true`. A pending clarification remains a hard barrier and still blocks the transition.
- Cancellation keeps the current session and conversation context. It records the existing cancellation status message, durably confirms `WAITING_FOR_INPUT` and closes every active turn before evaluating completion, leaves queued user messages parked, and ignores the cancelled turn's later stale ready/completion events.
- Once the agent acknowledges cancellation and publishes its terminal turn frames, the visible cancel request completes without waiting for the lifecycle escalation timeout. In the deterministic local regression environment, the shared desktop/mobile cancel control leaves its cancelling state within two seconds of that acknowledgement.
- Terminal frames emitted while cancellation is pending retain their normal ordered persistence before cancellation reconciliation. Concurrent cancel requests for the same session join one backend-owned operation and cannot duplicate the lifecycle cancel, status message, turn closure, or workflow completion.
- Explicit user cancellation, silent clarification recovery, and peer-message interruption share that same per-session operation. Only its owner invokes lifecycle cancellation; joined callers wait and then re-evaluate their own queue or clarification action.
- Once authorization and admission succeed, the shared operation uses a bounded service-owned context. A caller that disconnects may stop waiting without aborting cancellation or causing a live joiner to inherit its context error.
- If completion actions do not produce a transition, cannot be evaluated safely, or are blocked by pending user input, the task remains on the current workflow step and retains the existing `WAITING_FOR_INPUT` / review-ready reconciliation.
- If completion reaches a terminal workflow step, the task receives the same completed state as an ordinary `on_turn_complete` transition without an intermediate `REVIEW` state write.
- The workflow settings UI shows the option only when the step has an `on_turn_complete` transition. Its help text explains that destination `on_enter` actions may immediately start another agent.
- Desktop and mobile workflow settings expose the same capability. On phones, the option remains an inline, touch-sized setting inside the existing focused step editor; it does not introduce a new drawer or navigation level.

## Data Model

`workflow_steps` gains:

| Field | Type | Default | Meaning |
|---|---|---|---|
| `cancel_triggers_turn_complete` | boolean, non-null | `false` | An explicit user cancellation evaluates this step's `on_turn_complete` actions. |

`WorkflowStep`, `StepDefinition`, and the portable workflow step representation expose the same field. The embedded workflow-template YAML accepts `cancel_triggers_turn_complete`; template instantiation copies it into the persisted workflow step.

The portable workflow import/export format always exports the boolean. Missing input imports as `false`. Workflow sync compares and applies it as part of the step contract.

## API Surface

The existing workflow-step create and update contracts add an optional boolean:

```json
{
  "cancel_triggers_turn_complete": true
}
```

- Create with the field omitted persists `false`.
- Update with the field omitted leaves the current value unchanged.
- HTTP list/get responses and workflow-step WebSocket events return the effective boolean explicitly.
- `create_workflow_step_kandev` and `update_workflow_step_kandev` accept the same optional field in config mode; `list_workflow_steps_kandev` returns it.
- No new task-session cancel endpoint or WebSocket action is introduced. The existing authorized cancel action owns the behavior.

## State Machine

| Starting condition | Trigger | Result |
|---|---|---|
| Running Kanban turn; setting `false` | User cancels | Turn stops, session becomes input-ready, task remains on the current workflow step. |
| Running Kanban turn; setting `true` | User cancels | Cancellation bookkeeping authoritatively settles the session and turn, then the current step's `on_turn_complete` actions evaluate once. |
| Setting `true`; pending clarification | User cancels | Turn stops and remains on the current step; the clarification barrier blocks completion. |
| Setting `true`; transition succeeds | Cancellation settles | Source `on_exit`, workflow move, and destination `on_enter` run with ordinary completion semantics. |
| Setting `true`; transition reaches a terminal step | Cancellation settles | The workflow transition owns the final task state; no transient `REVIEW` state is published first. |
| Setting `true`; no eligible transition or evaluation failure | Cancellation settles | Session remains input-ready on the current workflow step. |
| Any setting | Internal or system-owned cancellation | No cancellation-driven completion evaluation occurs. |

## Permissions

No new permission is introduced. The caller must already be authorized to cancel the target task session. Workflow-step configuration continues to use the existing workflow mutation authorization and read-only synced-workflow rules.

## Failure Modes

- If the runtime cancel returns a non-recoverable error, the cancel request fails and Kandev does not reconcile the session or evaluate completion actions.
- If the agent acknowledges cancellation and emits terminal frames, those frames must be allowed to drain through the normal ordered stream path; cancellation must not hold a serialization boundary that the terminal path itself needs in order to finish.
- If the agent acknowledges the cancel but does not finish the turn, the existing bounded lifecycle timeout and escalation path still applies. Fast acknowledged cancellation does not shorten or remove that fallback.
- If no live execution exists or the runtime cancellation must be escalated, Kandev performs its existing safe reconciliation. Because the user cancellation was accepted, an enabled step may still evaluate completion actions.
- If the task is archived, ephemeral, Office-owned, missing its workflow step, or the current step cannot be loaded, Kandev does not move it through this policy.
- If the session cannot be authoritatively persisted as `WAITING_FOR_INPUT`, or any active turn cannot be closed and verified, Kandev returns the cancellation reconciliation error and does not evaluate `on_turn_complete`, move the workflow, or launch destination `on_enter` actions.
- If workflow evaluation or persistence fails, Kandev leaves the session input-ready on the current workflow step and surfaces the failure through existing logs and workflow state.
- A stale `agent.ready` or `agent.completed` event emitted after cancellation cannot run the transition a second time because the reconciled session no longer owns a running turn.
- While cancellation is admitted, non-stream prompt/state claimants defer or drop their mutations, and stream frames are accepted only when their execution and prompt generation match the captured cancellation identity. A successor prompt or stale completion therefore cannot be closed by the cancelled turn.
- Destination `on_enter` behavior is not suppressed. If it contains `auto_start_agent`, cancellation can intentionally start work on the destination step; the settings help text must disclose this consequence.

## Persistence Guarantees

The setting survives restart with the workflow step. No cancellation-specific session metadata or new transition record is added. Successful moves continue to use the existing workflow transition history with trigger `on_turn_complete`; the visible cancellation status message provides the user-facing cancellation record.

Template defaults apply when a workflow's steps are instantiated. Changing the embedded template does not rewrite existing persisted workflows, including workflows originally created from `simple`.

## Scenarios

- **GIVEN** a custom workflow step with an `on_turn_complete` move and `cancel_triggers_turn_complete=false`, **WHEN** the user cancels its active turn, **THEN** the session becomes input-ready and the task remains on that step.
- **GIVEN** the same step with `cancel_triggers_turn_complete=true`, **WHEN** the user cancels its active turn, **THEN** the move runs exactly once and the task appears on the configured destination step.
- **GIVEN** a signal-gated step with both `auto_advance_requires_signal=true` and `cancel_triggers_turn_complete=true`, **WHEN** the user cancels without an agent completion signal, **THEN** the configured completion transition still runs.
- **GIVEN** an enabled step with an unanswered clarification, **WHEN** the user cancels, **THEN** the turn stops but the task remains on the current step.
- **GIVEN** an enabled step whose destination auto-starts an agent, **WHEN** the user cancels, **THEN** the workflow moves and the destination's normal auto-start behavior runs.
- **GIVEN** an enabled step whose session-state write or turn closure fails, **WHEN** the user cancels, **THEN** no completion action, workflow move, or destination auto-start runs and the failure remains retryable.
- **GIVEN** an enabled step whose transition target is terminal, **WHEN** the user cancels, **THEN** observers see the task move directly to `COMPLETED` without an intermediate `REVIEW` state.
- **GIVEN** an enabled step whose transition target is nonterminal and does not immediately restart work, **WHEN** the user cancels, **THEN** the destination step is persisted and the task settles to `REVIEW` after the transition.
- **GIVEN** an enabled step with a queued user message, **WHEN** the user cancels, **THEN** the workflow transition may run but the queued message remains parked until an existing explicit drain or destination behavior consumes work.
- **GIVEN** an internal clarification timeout, peer interrupt, parent stop, archive, runtime failure, or provider error, **WHEN** that path cancels or stops an agent turn, **THEN** it does not run cancellation-driven completion actions.
- **GIVEN** a runtime cancel that is escalated or finds no live execution, **WHEN** Kandev safely reconciles the user's cancel request, **THEN** an enabled step evaluates completion once and the session does not remain stuck running.
- **GIVEN** an agent that acknowledges cancellation and immediately emits its terminal stream frames, **WHEN** the user cancels the active turn, **THEN** the cancel request settles without reaching the lifecycle escalation timeout and the shared desktop/mobile control leaves its cancelling state within two seconds in the deterministic local regression environment.
- **GIVEN** terminal usage, session-status, interruption-message, and completion frames emitted for the cancelled turn, **WHEN** cancellation is settling, **THEN** the frames drain in order before reconciliation and no frame can close or mutate a later successor turn.
- **GIVEN** multiple cancel requests for the same active turn, **WHEN** the first request is still waiting for terminal frames, **THEN** all requests observe the same operation result while the lifecycle cancel, visible cancellation message, turn closure, and completion policy run exactly once.
- **GIVEN** explicit and silent cancellation sources race for one active turn, **WHEN** both arrive before lifecycle cancellation settles, **THEN** exactly one lifecycle cancel runs and each source re-evaluates its own follow-up action after the shared result.
- **GIVEN** the explicit cancel caller disconnects while a second caller remains connected, **WHEN** lifecycle cancellation is still in progress, **THEN** the service-owned operation continues and the connected caller receives its final result.
- **GIVEN** the standard Kanban template, **WHEN** a new workflow is created from it, **THEN** `Backlog` and `In Progress` have `cancel_triggers_turn_complete=true` and cancellation moves work to `Review` through their existing actions.
- **GIVEN** an existing persisted standard Kanban workflow, **WHEN** Kandev upgrades, **THEN** its stored setting remains `false` until a user enables it.
- **GIVEN** an exported workflow with the setting enabled, **WHEN** it is imported or synchronized, **THEN** the imported step retains the enabled value.
- **GIVEN** a user edits a step on desktop or mobile, **WHEN** they enable the cancellation option and save, **THEN** the value persists after reload, the associated label provides a 44px touch target on phones, and the page has no horizontal overflow or desktop-only interaction.

## Out of Scope

- A separate `on_turn_cancelled` event or cancellation-specific action list.
- Backfilling or rewriting existing workflows created from the standard Kanban template.
- Changing queued-message drain policy after cancellation.
- Treating agent errors, provider failures, crashes, task stops, or clarification pauses as successful completion.
- Adding cancellation-cause metadata to workflow transition history.
- Reducing the existing lifecycle timeout for agents that acknowledge cancellation but never finish their prompt, or synthesizing success before their terminal path settles.