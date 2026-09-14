---
status: approved
created: 2026-08-13
owner: kandev
---

# Workflow Step Move Overrides

## Why

Kandev can move a task to another workflow step, but every move uses the target step's durable defaults. Operators and agents need a one-time exception for a particular hand-off without editing workflow policy or creating a second task configuration.

## What

- A human or agent can move a task to any target exposed by the existing manual-move policy, including explicitly allowed non-adjacent steps. The request does not bypass current workspace, archive, active-session, WIP, or target-membership validation.
- One move can carry these optional overrides:
  - reset_context: clear the current session's context before the target prompt;
  - instructions: append a custom hand-off message after the normal target workflow prompt;
  - skip_step_prompt: suppress the target step's durable prompt (and its task-description fallback) for this one entry.
- A move applies its overrides to the task's current primary session in place. It never changes the primary session, its agent profile, or its resolved model. Selecting a different agent profile or model for one entry is out of scope.
- The override object is consumed by this transition. It never changes the target workflow step, workflow default, or later task defaults.
- The target step's existing on_exit and on_enter actions still run once against the task's current session. The effective order is explicit reset when requested or configured, session configuration, normal workflow prompt construction (suppressed when skip_step_prompt is set), and appended one-time instructions delivery.
- If the target auto-starts an agent, the one-time instructions are appended to the auto-start prompt. If the current session exists but the target does not auto-start, the instructions are queued for that session's next prompt exactly once. A move that supplies agent-facing entry options is rejected before changing the task when there is neither a session nor target auto-start.
- When skip_step_prompt is set, the target step's durable prompt and its task-description fallback are suppressed for this one entry. With instructions present, the auto-started turn carries only those instructions (plus any workflow-level instructions block). With no instructions, the entry's auto_start_agent action is dropped so no agent turn starts and the session parks for input (WAITING_FOR_INPUT). skip_step_prompt combines independently with reset_context and instructions.
- The HTTP move endpoint, WebSocket task move action, and move_task_kandev accept the same nested entry_options object. The existing top-level MCP prompt remains accepted as a compatibility alias and is normalized into entry_options.instructions; conflicting prompt values are rejected.
- The active-agent move path stores the complete typed entry options in PendingMove, so a move requested during a RUNNING or STARTING turn applies after that turn rather than racing the current prompt. Direct moves carry the typed value on a transient task-metadata marker and publish only a move_id in task.moved.
- The stepper's target surface carries the direct Move here action and the one-shot options fields together: the fields are shown directly, and an untouched form submits the same destination-only move as a direct action. Chat and passthrough next-step controls keep their direct action and use the same shared form for fine-pointer options.
- On touch devices, target-step options open the existing task/session Drawer or Sheet rather than depending on hover. The form remains one-column, scrollable within the surface, safe-area aware, and usable with touch targets of at least 44 pixels. Desktop and mobile use the same form state, validation, and request payload.
- The agent-facing architecture is ready for the same contract to be used by move_task_kandev. Version one does not add pull-request draft/readiness controls; a future move-specific field can extend the typed object.

Decision: [ADR-2026-08-13-workflow-move-overrides](../../decisions/2026-08-13-workflow-move-overrides.md).

## Data model

The shared move override value is an optional object with this shape:

    {
      "reset_context": true,
      "instructions": "Start QA with the failing checkout test reproduced locally.",
      "skip_step_prompt": true
    }

Empty strings and false booleans are omitted. reset_context is opt-in and does not suppress a reset already configured on the target step.

The backend owns one typed EntryOptions definition in the workflow move package. HTTP and WebSocket request structs, task move options, the transient direct-move marker, watcher data, MCP decoding, and PendingMove persistence use that shape rather than independent maps. Decoding rejects unknown fields so a misspelled option fails closed rather than being silently ignored. The public task.moved event carries only move_id; entry instructions and toggles stay on the transient marker (direct moves) or the durable PendingMove record (deferred moves) and are never exposed in the event.

PendingMove persistence retains the serialized move override through its existing durable format and restart/reload path. Existing rows decode as an empty override. The in-memory repository follows the same value semantics; this remediation adds no new storage field or migration.

## API surface

The existing POST task move endpoint adds an optional entry_options object. The existing WebSocket task move action adds the same object. The existing move_task_kandev tool adds an optional entry_options object containing reset_context, instructions, and skip_step_prompt.

The legacy MCP prompt argument remains accepted for compatibility. A non-empty legacy prompt is copied to entry_options.instructions only when the nested field is empty; providing both values is a validation error. Responses include the normalized entry_options when supplied and identify immediate versus deferred acceptance with disposition. A deferred MCP response returns the authoritative source task until turn-end applies the move.

The task.moved event includes move_id when a direct move carries options; it never exposes instructions or option values. Deferred moves include the complete typed value in PendingMove. No new task or workflow endpoint is introduced.

## Precedence and state machine

| Stage | Normal source | One-move override | Result |
| --- | --- | --- | --- |
| Session | Task's current primary session | None | The move applies in place; the primary session, its profile, and its resolved model are unchanged. |
| Context | Target step reset_agent_context action | reset_context true | Reset when either source requests it. |
| Prompt | Workflow instructions, step prompt, and task prompt | instructions, skip_step_prompt | Instructions are appended once and never replace the normal workflow prompt; skip_step_prompt suppresses the step prompt and its task-description fallback for this entry. |

At target-step entry the orchestrator applies these precedences by building a transient copy of the target step with the move's reset, skip, and instructions baked in and running the ordinary on_enter path against that copy; the durable step is never mutated. A sentinel in the copied prompt marks that move instructions are already present so a re-dispatch never appends them twice. When skip_step_prompt is set with no instructions, the copy's auto_start_agent action is dropped so no turn starts. This overlay replaces the earlier private multi-phase move-entry state machine. Interrupted mid-application of a committed move is not recovered across a backend restart; an unconsumed deferred PendingMove is still honored through the normal queue path, but a move interrupted while entering the target requires a fresh move.

The transition follows this sequence:

1. Authenticate the caller and validate the target using existing move rules.
2. Normalize and validate overrides, including the requirement for a session or auto-start path when agent-facing fields are present.
3. Apply the move immediately when safe, or persist PendingMove when the source agent is active.
4. Persist the task step change and publish or process the transition through the existing event path.
5. Apply the reset override to the current session and dispatch or queue the effective prompt, or start no turn when skip_step_prompt is set with no instructions.
6. Consume the private entry object. A failed pre-transition validation leaves the task and entry untouched; a failed target-entry operation does not retry a partially applied prompt as a second move.

## Permissions

The existing task move authorization and MCP session authorization remain unchanged. Human HTTP and WebSocket callers use the authenticated owner boundary. Agent move_task_kandev calls retain their session-scoped task authorization. The overrides add no access to profiles, workspaces, or tasks that the caller could not already use.

## Failure modes

- Invalid target, cross-workspace target, archived task, active-session conflict, or invalid override leaves the task in its original step. A full limited destination accepts the move into its destination queue and defers destination entry and option consumption until promotion.
- A pending move survives backend restart with its override object. Invalid target data discovered at turn end is dropped using the existing pending-move cleanup path and cannot deliver its prompt to the source session.
- If prompt queueing fails after a direct move has been persisted, the task remains at the target and the backend records a sanitized warning; the prompt is not silently redelivered.
- A target without auto-start and without an existing session rejects agent-facing overrides rather than accepting an option that has no recipient.
- Repeated task.moved or agent-ready signals do not apply the same override twice. Existing transition and queue idempotence remains the guard.
- A move waiting for WIP capacity retains its private options unconsumed; promotion applies them once after admission.

## Persistence guarantees

- Workflow defaults never change as a side effect of a move override.
- Pending active-turn moves and their complete override object survive the message-queue repository's normal restart/reload path.
- Direct moves use the existing task.moved delivery path; the event includes only move_id and the orchestrator reads the transient move marker and overlays the target step before applying it at entry.
- A queued custom prompt is tagged as a move hand-off and is consumed at most once by the target session.
- The transient move marker is cleared after target entry is dispatched.

## Scenarios

- **GIVEN** a task is in Spec and Work is reachable, **WHEN** the operator chooses Work with reset_context enabled, **THEN** the task moves once and the target session is reset before Work's auto-start prompt.
- **GIVEN** a target step auto-starts with its own durable prompt, **WHEN** a move enables skip_step_prompt and supplies instructions, **THEN** the auto-started turn carries only the instructions and the step's durable prompt is suppressed for this entry.
- **GIVEN** a target step auto-starts, **WHEN** a move enables skip_step_prompt with no instructions, **THEN** no agent turn starts and the session parks for input.
- **GIVEN** a target step has its own workflow prompt, **WHEN** a move includes a custom prompt, **THEN** the normal workflow prompt remains intact and the custom text is appended exactly once.
- **GIVEN** the source agent is RUNNING, **WHEN** it calls move_task_kandev with reset and instructions overrides, **THEN** the tool returns a successful deferred move with disposition `deferred` and the complete override is applied after the current turn ends.
- **GIVEN** a deferred move is persisted and the backend restarts, **WHEN** the source session becomes ready, **THEN** the target and all override fields are restored and applied once.
- **GIVEN** the task has no session and the target step auto-starts, **WHEN** a human move includes instructions, **THEN** the first session and prompt include those instructions once.
- **GIVEN** the task has no session and the target step does not auto-start, **WHEN** a move includes an agent-facing override, **THEN** the request is rejected without changing the task.
- **GIVEN** an existing target session is waiting on a step without auto-start, **WHEN** a move includes a custom prompt, **THEN** the prompt is queued for that session's next input and is not duplicated by on_enter.
- **GIVEN** the target step normally resets context, **WHEN** reset_context is omitted, **THEN** the normal reset still happens; the override does not disable workflow policy.
- **GIVEN** a user opens a stepper target on desktop, **WHEN** they choose options, **THEN** one anchored form-capable action surface can set reset, skip step prompt, and instructions before submitting the move.
- **GIVEN** a user taps a target on a touch device, **WHEN** they open options, **THEN** the existing Drawer or Sheet exposes the same controls without requiring hover or horizontal scrolling.
- **GIVEN** a user proceeds from chat or passthrough with a fine pointer, **WHEN** they open options, **THEN** the anchored form targets the same next step as the direct proceed action and submits the same move contract; the direct action remains available without opening it.
- **GIVEN** the legacy MCP prompt is supplied without entry_options, **WHEN** move_task_kandev runs, **THEN** it behaves as entry_options.instructions and existing agents remain compatible.
- **GIVEN** both the legacy prompt and entry_options.instructions are non-empty, **WHEN** move_task_kandev runs, **THEN** it returns validation error and does not move the task.
- **GIVEN** an invalid target, **WHEN** a move is submitted, **THEN** the task remains in its source step and no override is queued; a full limited destination instead queues the task and retains the override until promotion.
- **GIVEN** a move completes, **WHEN** a later task enters the same target step without overrides, **THEN** it uses the target workflow defaults rather than the exceptional move values.

## Out of scope

- Pull-request creation mode such as draft versus ready for review.
- Persisting move overrides as workflow defaults or user preferences.
- Bypassing existing target reachability, authorization, WIP, archive, workspace, or active-session checks.
- A new transition endpoint separate from the existing move APIs.
- Selecting a different agent profile or model for one entry.
- Arbitrary executor, credential, MCP, permission-mode, or provider configuration overrides.
