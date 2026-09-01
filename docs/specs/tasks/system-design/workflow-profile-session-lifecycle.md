---
status: draft
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
---

# Workflow Step Profile Session Lifecycle System Design

## Purpose and boundaries

The task and workflow system owns fixed-profile routing and task-session
lifecycle. Each workflow step owns its agent profile override and two lifecycle
settings.

The destination step owns session selection. The source step owns session
retirement. The agent runtime still owns process launch, resume, and stop. The
task environment remains shared across sessions.

The workflow engine continues to select transitions and steps. This change does
not add an action, event, or workflow state. The orchestrator integration must
carry both source and destination step settings into the session handoff.

Conditional original-session settings remain separate. They change one
session's model settings without switching profiles.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Combined step agent selector](#combined-step-agent-selector) |

## Components and responsibilities

- `workflow/models.WorkflowStep` owns `AgentProfileID`,
  `ProfileSessionStartPolicy`, and `ProfileSessionEndPolicy`.
- `workflow/models.StepDefinition` carries the same settings in templates.
- The workflow repository persists both settings on each `workflow_steps` row.
- `workflow/models.StepPortable` carries both settings through export, import,
  templates, and workflow sync.
- Workflow step request and response DTOs carry both settings through create,
  update, duplication, boot data, and WebSocket updates.
- The transition integration supplies the source and destination steps to the
  orchestrator session handoff.
- The destination start setting controls reusable-session lookup.
- The source end setting controls completion or parking.
- Completion and stopped handlers consume execution-stamped stop intents. They
  skip ordinary workflow advancement for the retired execution.
- The workflow step draft and coordinated save path own the profile and both
  lifecycle settings.
- `WorkflowStepAgentProfileSelector` shows the profile and lifecycle settings
  in one surface.

## Data and contracts

The step stores two independent enums:

| Field | Value | Behavior |
| --- | --- | --- |
| `profile_session_start_policy` | `reuse` | Reuse the newest eligible nonterminal session. Create one when none is available. |
| `profile_session_start_policy` | `new` | Always create a session with a fresh provider conversation. |
| `profile_session_end_policy` | `complete` | Complete the source session before runtime stop. |
| `profile_session_end_policy` | `park` | Keep the source session nonterminal and stop its runtime. |

The schema defaults are `reuse` and `complete`. Domain constructors,
repository scans, request updates, templates, portable import, and sync use the
same defaults for missing or unknown values.

These defaults preserve the actual compatibility behavior of the current
`complete` policy. That policy looks for an eligible nonterminal destination
session and completes the source session.

The unshipped `profile_session_policy` field and its three-value enum are
removed from persistence, domain models, APIs, portable formats, frontend state,
and tests. The implementation does not retain a precedence rule between the old
field and the new fields.

The portable step uses `profile_session_start_policy` and
`profile_session_end_policy`. The export version does not change because both
fields are optional. Older readers ignore unknown fields.

A parked source session retains its session ID, task environment, executor
profile, messages, ACP resume token, and workflow-switch provenance. Its
`CompletedAt` value remains empty. It becomes non-primary after destination
promotion.

Before runtime stop, session metadata stores a workflow-switch stop intent. The
intent contains the exact agent execution ID and a unique stamp. Matching
callbacks consume the intent with stamped compare-and-set semantics. The
consumed tombstone remains durable.

## Control flow

1. Obtain the source step and destination step for the transition.
2. Resolve the destination step's effective profile.
3. If the profile is empty or matches the active session, preserve the current
   session. Do not apply either lifecycle setting.
4. Normalize the destination start setting and the source end setting.
5. If the start setting is `reuse`, select the newest eligible nonterminal
   matching session. Exclude the source session.
6. If the start setting is `new`, do not perform a reusable-session lookup.
7. Preflight managed Git credentials for the selected or new destination.
8. Prepare and promote the destination session before source mutation.
9. If the end setting is `complete`, mark the source `COMPLETED`. Then stop
   its runtime.
10. If the end setting is `park`, persist the execution-stamped stop intent.
    Set the source to `WAITING_FOR_INPUT` and clear `CompletedAt`.
11. Release the source lifecycle guard before runtime stop. If the caller owns
    the guard, schedule the stop after the lifecycle operation returns.
12. When the old execution emits a callback, consume only its matching stamp.
    Skip turn completion, transition evaluation, and task-state reconciliation.

The existing helper that accepts one destination policy must split its inputs.
Reusable-session lookup accepts the destination start setting. Source cleanup
accepts the source end setting.

Legacy transitions and manual moves already resolve both steps. They pass both
steps to session preparation. Direct engine entry paths that currently retain
only the destination must carry the source step ID or normalized end setting
through the transition context or step-entry record.

The workflow engine core remains unchanged. The engine integration and
orchestrator handoff contract change because session routing now needs both sides
of the transition.

## Failure and recovery

Destination preparation and credential validation fail before source mutation.
The current session remains primary and recoverable.

If a park intent cannot be persisted, Kandev stops the switch before source
retirement. It does not change the end setting to `complete`.

If runtime stop fails after parking is committed, Kandev reports the error and
retains the stamped intent. A retry can stop the same execution. A delayed
callback cannot advance the destination step.

A reuse candidate that becomes terminal before promotion is not revived. The
orchestrator creates a new destination when the start setting is `reuse`.
Terminal sessions remain historical endpoints.

Changing a step setting affects later transitions only.

## Persistence

The workflow repository replaces the unshipped step policy column with two
replayable columns for SQLite and PostgreSQL. Create, read, list, and update
queries include both fields.

Step export, import, templates, duplication, and synchronized YAML preserve both
enums. Sync equality includes both fields.

The stop-intent metadata remains on the session. Matching callbacks keep the
consumed tombstone across delayed delivery and restart. A newer park operation
writes a new execution ID and stamp.

## Combined step agent selector

The selector keeps one entry point for the agent profile and session lifecycle.
The closed trigger shows:

- The selected agent logo and profile label.
- `Reuse on start · Complete on end`, or the applicable compact summary.

When the step uses the workflow default, the trigger shows the generic agent
icon. A selected profile uses `AgentLogo` with `profile.agent_name`. Profile
rows use the same logo and label treatment as the new-task selector.

The desktop popover has this hierarchy:

1. The primary view contains the searchable profile list.
2. A **Session lifecycle** row shows the compact start and end summary.
3. The lifecycle view starts with this helper: **These settings apply when the
   workflow changes agent profiles.**
4. **When this step starts** offers:
   - **Reuse an available session.** Continue the most recent available session
     for this agent profile. If none is available, start a new session.
   - **Start a new session.** Always start a new conversation for this step.
5. **When this step ends** offers:
   - **Complete the session.** Close this session. The workflow cannot reuse it
     later.
   - **Park the session.** Stop the agent but keep the conversation available
     for reuse or manual follow-up.
6. The Back control returns to the profile list.
7. The existing **Save changes** action persists all three step settings.

The component does not use the old combined labels, such as **Park and reuse the
previous session**. Those labels mix source and destination behavior.

Profile health, workflow-default fallback, conditional-session incompatibility,
and dirty tracking remain workflow-specific. The selector can reuse
hierarchical picker primitives without using model-specific data types.

A synchronized workflow shows all settings and disables changes. When a step has
an incompatible conditional `configure_session` action, the existing profile
restriction remains visible.

## Mobile design contract

- **Desktop outcome:** The step header opens a popover for the profile and both
  lifecycle settings.
- **Mobile entry point:** The same trigger appears in the step card.
- **Nearest shipped exemplar:** The new-task profile selector supplies
  `AgentLogo` treatment. `ModelConfigSelector` supplies nested navigation.
- **Hierarchy:** The profile list is the first view. **Session lifecycle** opens
  one focused view with the start and end groups.
- **Presentation:** A phone uses an inset bottom drawer. A desktop uses a
  popover.
- **Geometry:** Each phone row has a 44 px active dimension. The drawer uses
  `100dvh` constraints, one internal scroll region, and safe-area padding.
- **Navigation:** Back returns to the profile list. Close returns focus to the
  trigger. The keyboard does not cover profile search.
- **Shared logic:** Both viewports share filtering, normalization, draft updates,
  dirty tracking, and save behavior.
- **Mobile evidence:** Playwright selects both lifecycle settings, saves,
  reloads, and checks containment and horizontal overflow.

## Verification design

Backend tests prove these four combinations:

| Start | End | Expected result |
| --- | --- | --- |
| `reuse` | `complete` | Reuse an eligible destination and complete the source |
| `new` | `complete` | Create a destination and complete the source |
| `reuse` | `park` | Reuse an eligible destination and park the source |
| `new` | `park` | Create a destination and park the source |

Focused persistence tests prove separate defaults and round-trip behavior.
Portable, template, duplication, and sync tests prove that each field remains on
its step.

Transition tests prove that the source end setting and destination start setting
come from different steps. They cover legacy, manual, queued, and direct engine
entry paths. Existing tests retain delayed callback, durable intent, stop error,
promotion race, and queue rollback coverage.

Frontend tests prove logos, profile search, separate start and end choices,
compact summaries, dirty state, read-only behavior, and desktop/mobile parity.

Desktop E2E saves different lifecycle combinations on multiple steps. Runtime
identity proves reuse versus new session and complete versus park. Mobile E2E
uses both choice groups and checks focus, touch size, safe areas, and overflow.

## Security

Existing workflow authorization and sync read-only guards protect both fields.
Session selection stays task-scoped and profile-matched. Stop-intent metadata
contains internal identifiers but no credentials or prompt content.

## Observability

Profile-switch logs include source step ID, destination step ID, start setting,
end setting, source outcome, and destination outcome. Stop-event suppression
logs include session ID, execution ID, and intent stamp.

## Related decisions

- [Make workflow step profile-session switching explicit](../../../decisions/2026-08-31-workflow-profile-session-switch-policy.md)
- [Task model unification](../../../decisions/0004-task-model-unification.md)
- [Agent model unification](../../../decisions/0005-agent-model-unification.md)
