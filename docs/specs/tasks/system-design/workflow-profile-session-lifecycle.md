---
status: draft
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
---

# Workflow Step Profile Session Lifecycle System Design

## Purpose and boundaries

The task and workflow system owns session recipients and task-session lifecycle.
Each step owns its recipient and two lifecycle settings. The explicit-target
extension below is a design for implementation, dated 2026-09-09. Existing
profile-only behavior remains the compatibility path.

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
| `REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002` | [Explicit recipient contract](#explicit-recipient-contract), [Recipient bindings](#recipient-bindings), [Explicit routing flow](#explicit-routing-flow), [Combined step agent selector](#combined-step-agent-selector) |

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

The current implementation uses the two independent lifecycle fields. The
earlier combined `profile_session_policy` contract was replaced; removing it
is not work in the explicit-recipient package. There is no precedence rule
between the retired contract and these fields.

The portable step uses `profile_session_start_policy` and
`profile_session_end_policy`. These optional lifecycle fields use export
version 1. Explicit recipient targets use version 2, as specified below;
the lifecycle-field compatibility rule does not apply to target routing.

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
3. For a profile-only step, if the normalized start policy is `reuse` and the
   profile is empty or matches the active session, preserve the current
   session. If the policy is `new`, create a fresh conversation even when the
   profile matches the active session. For an explicit target, use the explicit
   routing flow below, including same-profile fresh sessions.
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

The existing handoff accepts separate policies. Reusable-session lookup accepts
the destination start setting. Source cleanup accepts the source end setting.

Legacy transitions and manual moves resolve both steps and pass them to session
preparation. Every entry path, including direct engine entry, must retain the
source step ID or normalized end setting for explicit recipient changes too.

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

The two lifecycle columns and their replayable SQLite/PostgreSQL migrations
already exist. Create, read, list, and update queries include both fields.
The new step-column work is the nullable explicit recipient target below.

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

1. A fixed **Session lifecycle** row shows the compact start and end summary
   above search and recipient choices. It is outside search filtering and the
   scrolling list, so many profiles cannot hide it.
2. The list groups **Workflow sessions** (initial, then earlier steps) and
   **Agent profiles** (existing default and explicit profile choices).
   Earlier-step rows use `Step name · Profile label`; the initial row uses a
   generic icon and explains that task creation determines the agent.
3. The lifecycle view explains that the start setting selects the target's
   conversation and the end setting applies when leaving that conversation.
   Profile-only choices retain their existing same-profile behavior.
4. **When this step starts** offers:
   - **Reuse an available session.** Continue the most recent available session
     for this target. If none is available, start a new session. For an explicit
     target, do not imply that any session with the same profile qualifies.
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

A synchronized workflow permits opening and navigating the selector but disables
mutations. When a step has a conditional `configure_session` action, recipient
changes remain disabled with visible guidance to remove the rules first.
Initial targeting does not mean restoring original ACP model settings.

The closed trigger shows the explicit target's label, including source step
name, rather than displaying an empty profile as **No profile override**.
Draft derivation uses the full edited step list supplied through
`workflow-pipeline-editor-panels.tsx`. Rename and profile edits update labels
immediately; invalid references remain visible until repaired. The save
coordinator owns save/discard, including changes across multiple steps.

When a draft reorder, source removal, or override removal invalidates a target,
keep the selected source visible with an error and identify each dependent step.
Provide **Choose another target**, **Use no profile override**, and the existing
discard/undo path. Disable Save until the final draft is valid. A replacement
target preserves the start/end policies. Undo restores the previous reference.
Use target repair updates before the source mutation when each intermediate
state is valid; if no safe save order exists, ask the author to repair/save the
dependent steps first. Do not silently clear references or invent an atomic
multi-step API. Server rejection preserves the remaining draft and names the
affected steps, including edits made by another client. Synced workflows show
the error and direct repair to the source file, with no local mutation controls.

## Mobile design contract

- **Desktop outcome:** The step header opens a popover for the profile and both
  lifecycle settings.
- **Mobile entry point:** The same trigger appears in the step card.
- **Nearest shipped exemplar:** The new-task profile selector supplies
  `AgentLogo` treatment. `ModelConfigSelector` supplies nested navigation.
- **Hierarchy:** A fixed lifecycle row precedes search and grouped recipient
  choices. **Session lifecycle** opens one focused view with both groups.
- **Presentation:** A phone uses an inset bottom drawer. A desktop uses a
  popover.
- **Geometry:** Each phone row has a 44 px active dimension. The drawer uses
  `100dvh` constraints, one internal scroll region, and safe-area padding.
- **Surface rationale:** This is a temporary configuration choice, so the
  existing inset drawer fits. Reuse `MobilePickerSheet`, which already has
  settings consumers, and add a fixed-content slot for lifecycle/search above
  its scrolling children. Do not copy its shell. Its implementation is currently
  `components/task/mobile/mobile-picker-sheet.tsx`; moving it to a shared path
  is optional and must preserve every consumer. Keep desktop density separate
  from coarse-pointer hit areas. The search keyboard reduces the list viewport;
  lifecycle navigation stays outside that list.
- **Responsive selection:** Retain `useResponsiveBreakpoint` for this
  click/search selector's phone drawer and wider popover. Test coarse-pointer
  tablet containment and touch targets. `useTouchDrawer` is the existing choice
  when a disclosure requires a pointer-driven drawer alternative; it is not a
  required second responsive path for this selector. Do not add a parallel hook.
- **Navigation:** Back returns to the profile list. Close returns focus to the
  trigger. The keyboard does not cover profile search.
- **Shared logic:** Both viewports share filtering, normalization, draft updates,
  dirty tracking, and save behavior.
- **Mobile evidence:** Playwright selects both lifecycle settings, saves,
  reloads, and checks containment and horizontal overflow.

## Explicit recipient contract

Add optional `session_target` to `WorkflowStep`, `StepDefinition`, step DTOs,
frontend HTTP/WS types, and workflow drafts:

```json
{"session_target": {"kind": "initial"}}
{"session_target": {"kind": "step", "step_id": "implement-step-id"}}
```

Absence means the existing `agent_profile_id` resolution path. A nonempty
`agent_profile_id` and `session_target` are mutually exclusive. Target kinds
are a closed union. `initial` forbids `step_id`; `step` requires it. Unknown or
malformed values are errors, never default routing. Clearing the field with
explicit null returns to legacy routing; PATCH omission preserves it.

Both REST and MCP use the same controller target validation. Add target schema
and forwarding support in `internal/mcp/server/config_handlers.go`, then request
decoding/mapping in `internal/mcp/handlers/config_workflow_handlers.go`. Extend
`workflow_step_parity_test.go` with explicit target create/update/clear cases;
its existing payloads do not exercise the new field. Preserve omission versus
null through each layer, including emitted `workflow_step.*` payloads.

A `step` target must name an earlier position in the same workflow, whose
configuration has a direct profile override and no target of its own. This
bounded source rule avoids recursive references. Workflow transition cycles
remain supported; source availability is conditional on the path taken.
Deletion, reorder, and profile removal must validate incoming references.
Coordinated saves repair dependents before removing their source. A failed or
partial save must preserve unsaved drafts and expose the validation error.

Persist the target as nullable JSON in `workflow_steps.session_target` using
the repository's existing JSON conventions and replayable migrations on both
databases. Do not overload event actions or `agent_profile_id` with sentinels.
Keep the full workflow schema and the task repository's stub `workflow_steps`
projection in `base_schema.go` aligned. Audit lifecycle-column insert paths in
`defaults.go` and `workspace_bootstrap.go`, and the mapping in
`config/workflows/loader.go`. An intentionally omitted target in built-in rows
must yield the documented legacy null default, with a regression test. Inspect
task `workflow.go` for workflow-deletion cleanup; `builtin_workflow_step_rows.go`
only finds IDs and is not a target-column mapping that needs an automatic edit.
Portable steps represent a source with `step_position`, following
`pull_from_step_position`; two-pass import allocates IDs, then resolves targets.
Duplicate positions and unresolved references are errors. Duplication and
template instantiation remap source IDs through the existing `RemapStepID` map.
Sync equality and remapping include targets.

Export workflows containing explicit targets with portable version 2. Continue
reading version 1 and emitting it for workflows without targets. Update the
single version gate, `workflow/models.WorkflowExport.Validate` in `export.go`;
`workflowsync/service.go:parseExport` already calls it. Keep decoding and schema
docs aligned. A reader that cannot
interpret routing must reject the version rather than ignore the target and
send prompts to the wrong agent. A mixed export uses version 2 for its envelope.
Reject non-null `session_target` fields in a version 1 document. Recognizing
version 2 never bypasses closed-kind or source-reference validation.

## Recipient bindings

### Initial provenance and entry routing

Deliver initial targeting end to end before source-step targeting. Both remain
in scope; the plan owns the implementation milestones. The first slice adds
no per-step binding table. It accepts only `kind: initial`; unknown kinds are
rejected. The second slice extends the closed union with `kind: step` and
validates it. Both use portable version 2: an initial-only reader must reject
the unknown step kind even though it recognizes the envelope version.

The original-session marker consumed by `originalTaskSession` remains the
authority for discovering initial identity. Persist a write-once
`workflow_initial_session` object in task metadata with `session_id` and logical
`agent_profile_id`, atomically with first-session creation. This snapshot
survives deletion of the original session and a task changing workflows.
The ID remains historical when its session row is absent; never resolve it
to another task's session. A fresh replacement does not change the snapshot
or acquire the original marker. Do not copy this metadata into duplicated tasks.

For older tasks without a snapshot, use the existing conservative marker and
timestamp resolver and persist a snapshot only for an unambiguous result.
An absent or ambiguous original is a visible resolution error, not permission
to select the current primary or infer an agent from mutable task assignment.
Snapshot existing original identity before session deletion; if it cannot be
resolved, do not claim that later fresh fallback can recover the missing profile.

The first slice also adds a bounded `workflow_session_route` metadata object
for the current/latest entry: operation identity, destination step, resolved
target/profile snapshot, source session ID, destination session ID, and phase
(`prepared` or `committed`). It is a retry record, not recipient provenance.
Use conditional task-repository operations within the existing transition and
lifecycle serialization. Fresh session insertion and prepared destination
recording commit together; promotion and committed phase advance together.
Preflight external credentials before that transaction; do not hold a database
transaction while stopping or launching a runtime. Retry continues the recorded
destination and stamped source cleanup rather than allocating another session.

Reject an obsolete entry against the current transition/entry identity before
replacing this record. A newer entry cannot overtake an unfinished route without
reconciliation. Keep the committed record until a newer valid entry replaces it;
task deletion removes it and task duplication strips it. A deleted recorded
destination fails that entry visibly instead of creating a second destination
under the same operation. Test prepared/committed restart and rollback on both
databases. Extend existing task metadata transaction helpers; do not assume
unconditional `SetTaskMetadataKey` provides compare-and-set or write-once safety.

### Source-step bindings

Profile identity alone cannot identify the session selected by a prior step.
The second slice adds task-owned `task_workflow_session_bindings`, keyed by
`(task_id, target_key)`, with `workflow_id`, logical `agent_profile_id`, nullable
`session_id`, and the step-entry operation identity. Target keys are
`step:<workflow-step-id>`; they are internal and never portable. Initial
provenance stays in its existing task snapshot, with no table migration or
duplicate initial row in this slice.

Each successful entry to a direct-profile step records its selected session
against that step. This includes keeping the current session and reusing a
session originally created elsewhere. Re-entry replaces that step's binding.
A Review entry targeting Implement reads Implement's binding; Review does not
rewrite it. If reuse falls back to a new conversation, later Review entries
still resolve against the source step's binding. This makes the source reference
literal, not a moving alias for every conversation using its profile.

Persist bindings with the routing commit before any entry prompt or source
retirement. Reuse the existing task transition and step-entry serialization and
operation IDs for idempotence. Retried delivery of one entry reads its committed
destination; it must not create another session for `new`. Stale operations
cannot overwrite a later step binding. Asynchronous `session_step_history` is
audit data and must not become routing authority.

Session deletion clears the binding's session pointer but retains profile
identity for the documented fresh-session fallback. Task deletion removes its
bindings. Workflow/source-step deletion removes affected step bindings after
reference validation. The initial snapshot survives a task changing workflows.
Bindings never copy into duplicated tasks or workflow exports. Extend the
existing `task` required-store descriptor and fixed conformance adapter;
do not add another schema owner.

## Explicit routing flow

1. Resolve a typed destination containing target kind, source identity, logical
   profile, and an optional exact session ID. Initial uses its immutable snapshot;
   step uses its latest binding and the source step's current direct profile.
2. If no source binding exists because the step was skipped, use its configured
   profile with no candidate. A profile change invalidates the old candidate.
   Do not run the source step's entry actions.
3. For `reuse`, validate the exact candidate's task, profile, nonterminal state,
   and existing lifecycle eligibility. Never call newest-by-profile lookup for
   explicit targets. Missing or terminal candidates mean fresh creation;
   lookup errors or unresolvable profiles abort rather than masquerading as absence.
4. For `new`, discard the candidate even when its profile matches the active
   profile. Reusing the exact active session performs no retirement. Any change
   of session uses the source step's end setting, including same-profile changes.
5. Preflight the selected session's executor and credentials. Fresh creation
   retains the existing task environment and executor inheritance. Resolve
   dynamic logical profiles through the existing dynamic-launch path, never
   snapshotting a transient execution candidate as the logical profile.
6. Prepare/promote and persist the destination with the entry identity and any
   source-step binding update. On failure, retain the source and previous
   binding. Recheck eligibility at promotion; a candidate that became terminal
   uses the existing fresh-creation fallback.
7. Transfer queued prompts and pending moves, retire the departing session with
   existing stamped stop intent and rollback behavior, then execute entry actions
   with the destination ID. Preserve completion-signal and question barriers.

Use the same target resolver for `preflightWorkflowStepCredentials`,
`prepareWorkflowStepSession`, manual moves, queued moves, automatic advancement,
direct engine entry, and no-session task start. Carry the resolved selection
through preflight and preparation, then revalidate under the lifecycle guard.
Do not independently choose different candidates on either side of preflight.
Transport checks apply after recipient resolution for ACP and passthrough.

Initial task start binds the first session once; a start step targeting initial
must not create a throwaway session and immediately replace it. Subsequent
entries honor `new`. The source step is required whenever an existing session
is retired. Initial targeting with conditional `configure_session` remains
invalid; that feature still acts only on the immutable original session.

Explicit resolution failures use existing workflow/session error surfaces and
leave prompts undelivered. Log target kind, source step, destination session,
logical profile, and reuse/fresh reason without prompt content or credentials.
Validate source workflow membership on writes and reads, and task/session pairs
before promotion. A corrupt binding cannot route across tasks or users.

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
