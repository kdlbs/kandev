---
status: building
created: 2026-07-15
owner: kandev
---

# Workflow Cycle Guardrails

## Why

Workflow authors can unintentionally connect transition actions into a cycle
that re-enters an auto-start step. The repeated agent prompt looks like a user
message and may start work the author did not intend, while the current linear
pipeline view makes the complete trigger path easy to miss.

## What

- The workflow settings editor analyzes `on_turn_start` and
  `on_turn_complete` transition actions whenever it displays or proposes a
  workflow shape.
- The analysis resolves `move_to_next`, `move_to_previous`, and `move_to_step`
  against the workflow's ordered steps and reports only cycles whose
  `on_turn_complete` edge re-enters a step whose `on_enter` actions include
  `auto_start_agent`. An `on_turn_start` edge into that step does not replay
  automation because the runtime deliberately skips destination `on_enter`
  actions while dispatching the user's prompt.
- A cycle is **fully automatic** when every hop can occur without a user action:
  every transition starts from a step that auto-starts an agent on entry, and
  no hop requires approval. This includes `on_turn_start` transitions fired by
  an auto-started prompt. Creating or applying a newly introduced fully
  automatic cycle is blocked.
- A cycle is **user-mediated** when its `on_turn_complete` replay edge re-enters
  an auto-start step but at least one hop needs a user action: a transition
  from a step that does not auto-start its own turn, or a transition that
  requires approval. Creating or applying a newly introduced user-mediated
  cycle requires explicit confirmation.
- Transition guards are treated conservatively as possible paths. A guard does
  not make a cycle safe merely because its condition is not currently met.
- Intentional cycles that do not re-enter an `auto_start_agent` step remain
  valid and do not produce a warning.
- The diagnostic shows an ordered trace beginning and ending at the affected
  auto-start step. Every hop names the source step, trigger, transition action,
  and destination step, and indicates where a user action is required.
- The diagnostic identifies what the re-entered auto-start step sends:
  - an empty step prompt replays the task description;
  - a step prompt containing `{{task_prompt}}` sends the rendered step prompt
    including the task description;
  - any other non-empty step prompt replaces the task description.
- If several cycles re-enter the same auto-start step, the editor shows the
  highest-severity diagnostic first, then the shortest trace. Diagnostics for
  distinct auto-start steps remain separately identifiable.
- Existing persisted workflows with a replay cycle display the diagnostic as
  soon as their steps load. This visibility does not retroactively stop active
  tasks or prevent edits that remove the cycle.
- Existing workflows keep their current immediate step persistence. Before a
  step update, add, delete, or reorder request is sent, the editor analyzes the
  proposed complete shape:
  - a newly introduced fully automatic cycle cancels the request and offers no
    override;
  - a newly introduced user-mediated cycle holds the request until the user
    chooses **Apply anyway** or cancels it;
  - a change that removes a cycle proceeds without confirmation.
- A mutation is considered newly introduced only when its resulting diagnostic
  is absent from the currently persisted shape. Unrelated edits to a workflow
  that already has a diagnostic do not repeatedly ask for confirmation.
- A draft workflow is analyzed locally before its first persistence. Fully
  automatic cycles disable creation and explain the blocking trace;
  user-mediated cycles require **Create anyway** confirmation. Cancelling keeps
  the draft and sends no workflow or step request.
- The same diagnostic component is used for the inline workflow alert and the
  blocking/confirmation dialog. Affected steps are visually identified in the
  pipeline without relying on color alone.
- On mobile, the alert and trace stack vertically, dialog actions remain
  reachable by touch, long step names and prompt-source text wrap, and the page
  does not gain horizontal overflow. The existing pipeline may retain its own
  horizontal scrolling region.
- The guardrail is advisory for user-mediated cycles and blocking for fully
  automatic cycles only in the workflow settings authoring surface. It does
  not change workflow runtime execution or persisted workflow schemas.

## Analysis Contract

The analyzer accepts the complete proposed `WorkflowStep[]` and returns zero or
more diagnostics. Step order is ascending `position`, with input order as the
stable tie-breaker. A transition that cannot resolve to a step in the same
array is ignored by cycle analysis and remains subject to existing validation.

Each diagnostic contains:

- `severity`: `blocking` or `warning`;
- the re-entered auto-start step ID and name;
- the ordered affected step IDs;
- trace hops containing source ID/name, `on_turn_start` or
  `on_turn_complete`, action kind, destination ID/name, and whether that hop
  requires user involvement;
- prompt source: `task_description`, `step_prompt_with_task_description`, or
  `step_prompt`.

Diagnostics have stable identities derived from the auto-start step and the
ordered trace. The authoring guard compares these identities between current
and proposed shapes so it can distinguish an introduced cycle from an existing
one. The analysis is deterministic and has no persisted state.

## Failure Modes

- If workflow steps fail to load, the editor keeps its existing load error and
  does not claim that the workflow is safe.
- If a confirmed remote mutation fails, the existing error toast remains the
  source of truth and the editor refreshes or retains the server-backed shape
  according to the existing mutation behavior. Confirmation never implies
  persistence succeeded.
- If a workflow already contains a fully automatic cycle, opening settings
  surfaces the blocking diagnostic but does not interrupt running tasks. The
  author must remove the cycle to prevent future re-entry.
- Invalid or dangling transition targets do not crash the editor and do not
  create a false replay trace.

## Scenarios

- **GIVEN** `Build` and `Review` both auto-start on entry and each moves to the
  other on turn completion, **WHEN** an author attempts to save the second
  transition, **THEN** the request is blocked and the trace shows
  `Build --on_turn_complete--> Review --on_turn_complete--> Build`.
- **GIVEN** `In Progress` auto-starts, a path includes an `on_turn_start`
  transition from a step that does not auto-start, and its final
  `on_turn_complete` edge returns to `In Progress`, **WHEN** an author applies
  the final edit, **THEN** the editor requires **Apply anyway** and sends no
  mutation before confirmation.
- **GIVEN** every source in the same path auto-starts, **WHEN** an
  `on_turn_start` transition is fired by an auto-started prompt, **THEN** that
  hop is automatic and a newly introduced fully automatic replay cycle is
  blocked.
- **GIVEN** `Review` moves to auto-starting `In Progress` on `on_turn_start`,
  **WHEN** that feedback cycle is analyzed, **THEN** the editor shows no replay
  warning because the runtime sends the user's prompt without executing
  `In Progress.on_enter`.
- **GIVEN** a cycle contains only `on_turn_complete` transitions but one source
  step does not auto-start, **WHEN** the cycle is introduced, **THEN** it is a
  user-mediated warning because a user must start that step's turn.
- **GIVEN** any transition in the cycle, including the replay edge, requires
  approval, **WHEN** the cycle is introduced, **THEN** it is a user-mediated
  warning rather than a fully automatic block.
- **GIVEN** a draft workflow has a user-mediated replay cycle, **WHEN** the user
  clicks Save and then cancels the confirmation, **THEN** the draft remains and
  no workflow or step creation request is sent.
- **GIVEN** a draft workflow has a user-mediated replay cycle, **WHEN** the user
  confirms **Create anyway**, **THEN** the existing workflow creation sequence
  proceeds once.
- **GIVEN** an existing workflow already has a replay-cycle diagnostic, **WHEN**
  the settings page loads, **THEN** the inline alert shows the exact trace and
  prompt source before the author edits it.
- **GIVEN** an existing workflow already has a replay-cycle diagnostic, **WHEN**
  the author changes only its name or prompt without changing the diagnostic
  identity, **THEN** the mutation is not reconfirmed and the inline alert
  remains accurate.
- **GIVEN** an existing replay cycle, **WHEN** an edit removes the return path or
  removes `auto_start_agent`, **THEN** the edit persists without confirmation
  and the diagnostic disappears after refresh.
- **GIVEN** a workflow cycle that never re-enters an auto-start step, **WHEN**
  the author creates or edits it, **THEN** the editor shows no replay warning and
  does not block the mutation.
- **GIVEN** a re-entered auto-start step has no prompt, **WHEN** its diagnostic
  renders, **THEN** the prompt source states that the task description will be
  replayed.
- **GIVEN** a re-entered auto-start step prompt contains `{{task_prompt}}`,
  **WHEN** its diagnostic renders, **THEN** the prompt source states that the
  rendered step prompt includes the task description.
- **GIVEN** the warning is displayed at a mobile viewport, **WHEN** the user
  reviews and confirms a user-mediated cycle, **THEN** the full trace and both
  dialog actions are usable without horizontal page scrolling or hover.

## Out of Scope

- Changing workflow runtime transition or auto-start behavior.
- Stopping tasks already cycling when the settings page opens.
- Rejecting workflow writes made directly through HTTP, WebSocket, YAML import,
  seed configuration, or another client.
- Analyzing generic event-driven transitions such as
  `on_children_completed`, `on_heartbeat`, or `on_agent_error` in this first
  iteration.
- Warning about cycles that contain no auto-start replay.
- Displaying the full task description or fully rendered prompt in settings.

## Implementation Plan

[Implementation plan](../../plans/workflow-cycle-guardrails/plan.md)
