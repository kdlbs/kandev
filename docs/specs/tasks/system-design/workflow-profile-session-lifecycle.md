---
status: draft
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
---

# Workflow Step Profile Session Lifecycle System Design

## Purpose and boundaries

The task and workflow system owns fixed-profile step routing and task-session
lifecycle. This design makes the destination workflow step own both its agent
profile override and its profile-session entry policy. The agent runtime still
owns process launch, resume, and stop. The existing task environment remains
shared across sessions.

This changes the data supplied to workflow step entry. It does not change the
workflow engine's transition graph, action vocabulary, or transition evaluation.
The orchestrator already receives the destination step before it prepares a
session, so it can read the policy directly from that step.

Conditional original-session settings remain separate. They mutate one
session's model-adjacent configuration without switching profiles.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Combined step agent selector](#combined-step-agent-selector) |

## Components and responsibilities

- `workflow/models.WorkflowStep` owns the normalized destination-step policy
  beside `AgentProfileID`.
- The workflow repository persists the policy with each `workflow_steps` row.
- `workflow/models.StepPortable` carries the policy through export, import,
  templates, and workflow sync.
- Workflow step request and response DTOs carry the policy through create,
  update, duplication, boot data, and WebSocket updates.
- `orchestrator.Service.prepareWorkflowStepSession` reads the normalized policy
  directly from the destination step and selects reuse or new-session behavior.
- The profile-switch stop path completes or parks the source session and records
  a stop-intent stamp before asking the agent runtime to stop it.
- Agent completion and stopped handlers consume matching stop-intent stamps and
  skip ordinary workflow advancement for that old execution.
- The workflow step draft and coordinated save path own both the selected profile
  and policy.
- A dedicated combined selector presents profile choice and session behavior in
  one configuration surface. It follows the interaction pattern of the chat
  model selector without making agent profiles a model-specific abstraction.

## Data and contracts

`workflow_steps.profile_session_policy` is a non-null text value with these
canonical values:

| Value | Source session | Destination entry |
| --- | --- | --- |
| `complete` | Set `COMPLETED`, then stop runtime | Reuse the newest independently eligible nonterminal matching session, or prepare a new one |
| `park_reuse` | Set `WAITING_FOR_INPUT`, then stop runtime | Reuse the newest eligible nonterminal matching session |
| `park_new` | Set `WAITING_FOR_INPUT`, then stop runtime | Prepare a new session |

The schema default is `complete`. Domain constructors, repository scans,
request updates, portable import validation, templates, and sync normalization
also map an empty or unknown value to `complete`.

The portable step field uses the same `profile_session_policy` name and values.
The workflow-level portable field is removed. The export version does not change
because the step field is optional and older readers already ignore unknown
fields. Omitted input preserves the compatibility default.

No workflow-level policy is retained in the domain, API, persistence, portable
format, or frontend state. The earlier workflow-level design was not released,
so its unshipped column and mappings are replaced instead of creating two
sources of truth.

A parked source session retains its session ID, task environment, executor
profile, messages, ACP resume token, and workflow-switch provenance. Its
`CompletedAt` remains unset. It is non-primary after the destination session is
promoted.

Before a parked runtime stop, session metadata stores a workflow-switch stop
intent containing the exact agent execution ID and a unique stamp. Matching
completion and stopped handlers mark the intent consumed with stamped
compare-and-set semantics and retain the durable tombstone. They do not use a
generic session-state check or an unscoped boolean cancellation marker.

## Control flow

1. Resolve the destination step's effective profile using the existing step
   override and workflow-default fallback.
2. Normalize `destinationStep.ProfileSessionPolicy`.
3. If the profile is empty or matches the active session, preserve the current
   session. The destination policy has no effect.
4. Preflight managed Git credentials before mutating session ownership.
5. For `park_reuse`, select the newest nonterminal matching session, excluding
   the source. For `complete`, retain today's nonterminal reuse behavior when
   an independently eligible matching session already exists. For `park_new`,
   do not perform a matching-session lookup.
6. Prepare a destination session before changing the source. Promote the
   destination and transfer pending queued state using the existing profile
   switch ordering and failure checks.
7. For `complete`, mark the source `COMPLETED` before runtime stop so the
   current terminal guard suppresses the stop event.
8. For either park policy, persist the execution-stamped stop intent and set
   the source to `WAITING_FOR_INPUT` while holding the source session's
   cancellation/teardown guard. Release that guard before asking the runtime
   to stop. If the caller already owns the guard, schedule the stop after the
   surrounding lifecycle operation returns. This allows a synchronous terminal
   callback from the runtime to consume the durable intent without deadlocking.
9. When the old execution emits completion or stopped, consume only its matching
   stamp and retire its runtime activity. Skip turn completion, workflow
   transition evaluation, and task-state reconciliation.
10. A later prompt to a parked session uses the existing cold-resume path and
    stored provider resume token. A new execution ID cannot match the consumed
   or stale old stop intent. A parked source has no `CompletedAt` value.

The workflow engine still decides which step follows from the current step and
actions. No new transition action, event, state, or evaluation branch is added.
The only engine-adjacent change is that session preparation consumes a field
already present on the resolved destination step instead of fetching workflow
metadata for the policy.

## Failure and recovery

Destination preparation and credential validation fail before source-session
mutation. The current session remains primary and recoverable.

If a park stop intent cannot be persisted, Kandev aborts the switch before
stopping or completing the source session. It does not silently downgrade to
`complete`. Any prepared but unpromoted destination remains eligible for
ordinary session cleanup.

If runtime stop fails after the parked state is committed, Kandev reports the
failure and retains the execution-stamped intent. The switch still succeeds,
and a retry can address the same execution without allowing its delayed
completion to advance the destination step.

A candidate that becomes terminal between lookup and primary promotion is not
revived. `park_reuse` prepares a new destination, preserving the existing
conditional-promotion guard.

Terminal sessions remain immutable historical endpoints for every policy.
Changing a step policy affects later entries to that step only.

## Persistence

The workflow repository adds the replayable `workflow_steps` column migration
for SQLite and PostgreSQL and includes the field in create, read, list, and
update queries. Step export/import, templates, and synchronized YAML preserve
the enum.

The stop-intent metadata is coordination state stored on the session so delayed
callbacks can be matched safely. The matching callback marks it consumed and
keeps the tombstone durable across delayed delivery and restart. A newer parked
switch overwrites it with a new execution ID and stamp. A stale stamp after
restart is harmless because a resumed execution has a different ID.

## Combined step agent selector

The simple step agent profile select becomes a dedicated
`WorkflowStepAgentProfileSelector`. Its closed trigger shows the robot icon,
the selected profile or workflow-default label, and a compact session-behavior
summary. Dirty state is true when either the profile or the policy differs from
the saved step.

Desktop interaction uses a field-style popover patterned after
`ModelConfigSelector`:

1. The primary view contains a searchable profile list, including the workflow
   default option.
2. A separated **Session behavior** row shows the current value and opens a
   nested view.
3. The nested view has a back control and the three policy choices with visible
   descriptions.
4. Selecting a profile or policy updates the same step draft. The existing
   workflow **Save changes** action persists both fields.

Use the model selector as an interaction exemplar, not as the data model.
Profile health, workflow-default fallback, conditional-session incompatibility,
and step dirty tracking remain workflow-specific. Shared hierarchical selector
primitives may be extracted only if they reduce duplication without coupling
agent profiles to model configuration.

When a step has an incompatible conditional `configure_session` action, the
profile choice retains the existing disabled behavior and explanation. The
session-policy value remains visible. Synced workflows render both values in the
same selector but disable mutations.

## Mobile design contract

- **Desktop outcome:** authors choose a step profile and configure its session
  behavior from one popover in the step header.
- **Mobile entry point:** the same combined trigger appears in the step card.
- **Nearest shipped exemplar:** `ModelConfigSelector` supplies the nested
  selection hierarchy. Existing responsive drawer-based selectors supply the
  phone presentation.
- **Hierarchy and primary action:** profile selection is the first view, and
  **Session behavior** is a nested setting. The workflow's existing
  **Save changes** action remains the only persistence action.
- **Presentation rationale:** the control combines search and a nested setting,
  so a phone inset bottom drawer is more usable than a short select menu.
- **Geometry:** the trigger and rows have at least a 44 px active dimension on
  touch viewports. The drawer uses `100dvh` constraints, one internal scroll
  owner, safe-area bottom padding, wrapped descriptions, and no horizontal page
  overflow.
- **Navigation:** Back returns from policy choices to the profile list and
  restores focus. Close returns focus to the trigger. The software keyboard does
  not hide the active search or selected row.
- **Shared logic:** desktop and mobile share profile filtering, normalization,
  step-draft updates, dirty tracking, validation, and save behavior. Only the
  presentation shell differs.

## Verification design

Focused backend tests prove per-step persistence, portable import/export,
template and sync round-trip, default normalization, and that two steps in one
workflow retain different policies.

Orchestrator tests pass destination steps with each policy directly. They prove
`A -> B -> A` identity, same-profile continuity, delayed callback suppression,
durable consumed intent, stop failure, promotion races, and queue rollback. A
focused assertion verifies that no workflow metadata lookup is required to
resolve the policy.

Frontend tests prove profile search, nested policy navigation, trigger summary,
combined dirty state, read-only behavior, conditional-session behavior, and
desktop/popover versus mobile/drawer parity.

Desktop Playwright coverage saves different policies on different steps,
reloads, and proves runtime identity. Mobile Playwright coverage uses the
combined drawer, saves and reloads, checks 44 px touch targets, focus return,
safe-area containment, and absence of document horizontal overflow.

## Security

The existing workflow authorization and read-only sync guard protect the new
step field. Session selection stays task-scoped and profile-matched. Stop-intent
metadata contains internal session and execution identifiers but no credentials
or prompt content.

## Observability

Profile-switch logs add the normalized destination-step policy, step ID, source
outcome (`completed` or `parked`), and destination outcome (`reused` or
`created`). Stop-event suppression logs include the session and execution IDs
and the stop-intent stamp. Failures use the existing workflow transition and
session-state error surfaces.

## Related decisions

- [Make workflow step profile-session switching explicit](../../../decisions/2026-08-31-workflow-profile-session-switch-policy.md)
- [Task model unification](../../../decisions/0004-task-model-unification.md)
- [Agent model unification](../../../decisions/0005-agent-model-unification.md)
