---
status: draft
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
---

# Workflow Profile Session Lifecycle System Design

## Purpose and boundaries

The task and workflow system owns fixed-profile step routing and task-session
lifecycle. This design extends that routing with a workflow-level policy. The
agent runtime still owns process launch, resume, and stop. The existing task
environment remains shared across sessions.

Conditional original-session settings remain separate. They mutate one
session's model-adjacent configuration without switching profiles.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery), [Frontend and mobile behavior](#frontend-and-mobile-behavior) |

## Components and responsibilities

- `task/models.Workflow` owns the normalized workflow policy.
- The task workflow repository persists the policy with the workflow row.
- `workflow/models.WorkflowPortable` carries the policy through export, import,
  and workflow sync.
- `workflow.Service.GetWorkflowMeta` projects the policy with the workflow
  profile and prompt so one request-scoped read serves step entry.
- `orchestrator.Service.prepareWorkflowStepSession` selects reuse or new-session
  behavior from the destination workflow's policy.
- The profile-switch stop path completes or parks the source session and records
  a stop-intent stamp before asking the agent runtime to stop it.
- Agent completion and stopped handlers consume matching stop-intent stamps and
  skip ordinary workflow advancement for that old execution.
- The workflow settings draft, save coordinator, and workflow card expose the
  policy without a separate save path.

## Data and contracts

`workflows.profile_session_policy` is a non-null text value with these canonical
values:

| Value | Source session | Profile re-entry |
| --- | --- | --- |
| `complete` | Set `COMPLETED`, then stop runtime | Prepare a new session |
| `park_reuse` | Set `WAITING_FOR_INPUT`, then stop runtime | Reuse newest eligible nonterminal session |
| `park_new` | Set `WAITING_FOR_INPUT`, then stop runtime | Prepare a new session |

The schema default is `complete`. Domain constructors, repository scans,
request updates, portable import validation, and sync normalization also map an
empty or unknown value to `complete`.

The portable workflow field uses the same `profile_session_policy` name and
values. The export version does not change because the field is optional and
older readers already ignore unknown fields. Omitted input preserves the
compatibility default.

A parked source session retains its session ID, task environment, executor
profile, messages, ACP resume token, and workflow-switch provenance. Its
`CompletedAt` remains unset. It is non-primary after the destination session is
promoted.

Before a parked runtime stop, session metadata stores a workflow-switch stop
intent containing the exact agent execution ID and a unique stamp. Matching
completion/stopped handlers remove the stamp with compare-and-remove semantics.
They do not use a generic session-state check or a boolean cancellation marker.

## Control flow

1. Resolve the destination step's effective profile and the workflow policy
   through the existing request-scoped workflow metadata cache.
2. If the profile is empty or matches the active session, preserve the current
   session. The policy has no effect.
3. Preflight managed Git credentials before mutating session ownership.
4. For `park_reuse`, select the newest nonterminal matching session, excluding
   the source. For `complete`, retain today's nonterminal reuse behavior when an
   independently eligible matching session already exists. For `park_new`, do
   not perform a matching-session lookup.
5. Prepare a destination session before changing the source. Promote the
   destination and transfer pending queued state using the existing profile
   switch ordering and failure checks.
6. For `complete`, mark the source `COMPLETED` before runtime stop so the current
   terminal guard suppresses the stop event.
7. For either park policy, persist the execution-stamped stop intent, set the
   source to `WAITING_FOR_INPUT`, clear `CompletedAt`, and stop the runtime.
8. When the old execution emits completion or stopped, consume only its matching
   stamp and retire its runtime activity. Skip turn completion, workflow
   transition evaluation, and task-state reconciliation.
9. A later prompt to a parked session uses the existing cold-resume path and
   stored provider resume token. A new execution ID cannot match the consumed or
   stale old stop intent.

## Failure and recovery

Destination preparation and credential validation fail before source-session
mutation. The current session remains primary and recoverable.

If a park stop intent cannot be persisted, Kandev aborts the switch before
stopping or completing the source session. It does not silently downgrade to
`complete`. Any prepared but unpromoted destination remains eligible for
ordinary session cleanup.

If runtime stop fails after the parked state is committed, Kandev reports the
failure and retains the execution-stamped intent. A retry can address the same
execution without allowing its delayed completion to advance the destination
step.

A candidate that becomes terminal between lookup and primary promotion is not
revived. `park_reuse` prepares a new destination, preserving the existing
conditional-promotion guard.

Terminal sessions remain immutable historical endpoints for every policy.
Changing a workflow policy affects later profile switches only.

## Persistence

The task repository adds the replayable workflow column migration for SQLite
and PostgreSQL and includes the field in create, read, list, and update queries.
Workflow export/import and synced YAML preserve the enum.

The stop-intent metadata is transient coordination state stored on the session
so delayed callbacks can be matched safely. It is removed by the matching
callback. A stale stamp after restart is harmless because a resumed execution
has a different ID. Resume can remove stale stamps when practical.

## Frontend and mobile behavior

The Workflow details area adds a labeled **Profile session behavior** select
next to the workflow's default agent profile. It shows all three choices and a
visible explanation below the control. Synced read-only workflows render the
value with the select disabled.

The workflow draft and saved baseline treat `profile_session_policy` as an
editable field. The existing floating **Save changes** coordinator persists it,
and reload hydration does not overwrite an unsaved local choice.

Mobile design contract:

- **Desktop outcome:** authors choose one workflow-wide policy in Workflow
  details and save it with the rest of the workflow.
- **Mobile entry point:** the same labeled select appears in the existing
  single-column Workflow details flow.
- **Nearest shipped exemplar:** `WorkflowCardBody` and
  `mobile-workflow-settings.spec.ts`. Reuse the responsive Radix select and the
  page's existing document scroll owner.
- **Hierarchy and primary action:** the policy follows the default profile.
  The existing floating **Save changes** action remains the only persistence
  action.
- **Presentation rationale:** this is one short workflow-wide choice, so an
  inline select with the existing phone bottom-sheet treatment is clearer than
  a new drawer or route.
- **Geometry:** the phone trigger and option rows have touch-sized hit areas,
  the explanation wraps, and no document-level horizontal scrolling is added.
- **Shared logic:** all viewports share draft state, normalization, dirty
  tracking, validation, and save behavior.

Desktop and mobile Playwright scenarios change the policy through the UI,
save, reload, and verify the selected value. The desktop runtime scenario also
proves `A -> B -> A` session identity for both park policies.

## Security

The existing workflow authorization and read-only sync guard protect the new
field. Session selection stays task-scoped and profile-matched. Stop-intent
metadata contains internal session and execution identifiers but no credentials
or prompt content.

## Observability

Profile-switch logs add the normalized policy, source outcome (`completed` or
`parked`), and destination outcome (`reused` or `created`). Stop-event
suppression logs include the session and execution IDs and the stop-intent stamp.
Failures use the existing workflow transition and session-state error surfaces.

## Related decisions

- [Make workflow profile-session switching explicit](../../../decisions/2026-08-31-workflow-profile-session-switch-policy.md)
- [Task model unification](../../../decisions/0004-task-model-unification.md)
- [Agent model unification](../../../decisions/0005-agent-model-unification.md)
