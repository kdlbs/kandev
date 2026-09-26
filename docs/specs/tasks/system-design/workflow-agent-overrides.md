---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-AGENT-OVERRIDES-001
---

# Task workflow agent overrides design

## Ownership and current evidence

This design extends task creation and workflow routing. It does not change agent
profile ownership. The inspected Feature workflow has one fixed profile:
Implement uses 5.6 Luna Max. PR has a step recipient that references Implement.
Analysis and Review have initial-session recipients.

`TaskCreateAdvancedSettings` currently contains dependency and priority controls.
`WorkflowSection` reads fixed step profiles from workflow snapshots.
`resolveStepAgentProfile` currently resolves a step profile, then a workflow
default. Explicit session targets bypass this profile resolver.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKFLOW-AGENT-OVERRIDES-001` | Data and persistence, Creation validation, Runtime routing, Create dialog, Verification |

## Data and persistence

Proposed public create field: `workflow_agent_overrides`, an optional object
whose keys are source profile IDs and values are replacement profile IDs.
Omission, null, and an empty object mean no overrides. A self-mapping normalizes
to absence. Empty keys or values are invalid. Substitution is a single lookup:
A-to-B and B-to-C means that an A step uses B, not C.

At creation, expand each source-profile choice into the matching fixed step IDs.
Add a typed task-owned record with `workflow_id` and `steps` fields. Each step
entry contains `source_profile_id` for provenance and `replacement_profile_id`
for routing. These bindings freeze the affected step identities, not the workflow.
The source-to-replacement map remains the create-input and grouped UI contract. Persist
it in a new nullable TEXT column, `tasks.workflow_agent_overrides`, as JSON.
The stored workflow ID comes from the resolved task workflow, never a second
caller-supplied workflow identity. Existing rows read as empty.

Use the existing replayable SQLite/PostgreSQL migration path. Include the
record in task insert, scan, get, list, and update projections. Unrelated task
updates preserve it. Generic task updates do not accept replacement changes.
Expose the effective step bindings in task DTOs only when their stored workflow matches.
Do not reconstruct them from the workflow's current source profiles.
Malformed persisted records return a routing error instead of an empty map.

This typed column avoids an unvalidated generic metadata write path. A profile
map expresses the user's single replacement choice in the form. Durable step
bindings preserve that choice when shared profile defaults change. A live
source-profile lookup would lose the replacement after such an edit.
No separate ADR is needed: this feature design preserves the local rationale.

## Creation validation

Extend `httpCreateTaskRequest`, service `CreateTaskRequest`, the existing WS
create adapter, and frontend `CreateTaskParams`. Keep old callers compatible.
Validation belongs in the task service after workspace authorization and
workflow resolution, before task persistence. Reuse the service's injected
profile and workflow lookups rather than calling HTTP internally.

Valid keys are nonempty `WorkflowStep.AgentProfileID` values in the selected
workflow. A step with `SessionTarget` is not a source. Deduplicate by ID, not
name. Missing source definitions remain visible by ID so a user can repair the
choice with a valid replacement. Source membership uses workflow references.

Replacement eligibility uses the existing enabled-profile, authorization, and
agent/executor compatibility rules. Dynamic profiles remain valid only where
existing selectors and launch validation already permit them. Their candidates
are not recursively replaced. Credential preflight uses the effective profile.
Reject stale source keys rather than silently discarding a submitted choice.

Validate both create-only and create-and-start. Persist the map with the task,
so deferred execution reads the same choice. Preserve existing external-ID
idempotency: a replay returns the existing task without mutating its overrides.
Do not inherit a parent's map in MCP or ordinary child creation.

## Runtime routing

Introduce one task-aware resolver used by every existing
`resolveStepAgentProfile` call site. Pass an already-loaded task where possible.
Resolve from the current task ID only. Do not cache the result by workflow or step alone.
Never modify a shared workflow object or another task record during resolution.
Tasks in the same workflow can concurrently use different replacements. A task
without a replacement continues to use the workflow profile. Separate the template profile
from the effective profile for diagnostics.

Resolution order:

1. Resolve explicit session targets through existing recipient bindings.
2. For a fixed-profile step, look up its step ID in the task bindings when the stored workflow ID matches.
3. Otherwise preserve the step profile, workflow default, and current-session fallback.

Only fixed step references participate in substitution. The main Initial Agent
selection and workflow-level defaults retain their existing behavior, even when
they happen to use the same profile ID as a source.

Audit `event_handlers_workflow.go`, `session_ensure.go`, and
`task_operations.go`. Cover initial launch into a fixed step, ensure/restart,
credential preflight, automatic transitions, manual transitions, and previews.
Both preflight and session preparation must use the same effective profile.

For the reference workflow, Implement creates the replacement session using its
existing `new` policy. PR resolves the Implement session binding with its
existing `reuse` policy. It does not perform another profile substitution.
Same-profile replacements do not bypass a `new` policy. Existing dynamic-profile
routing, terminal-session exclusion, park/complete behavior, and conversation
identity remain authoritative.

Revalidate a replacement at launch. If it becomes unavailable, return the
existing actionable transition/launch error and preserve the source session.
Never retry with the original profile. Changing a bound step's fixed profile
from Luna to Sol does not replace this task's Terra choice. New steps have no
binding, even when their profile matches an original source. Deleted step
bindings remain inert. Recreated steps have new IDs and do not inherit them.
Changing a step to an explicit session target retains that target's existing
authority; this feature does not freeze workflow topology or session policies. Moving to another workflow disables this map; moving back restores its
applicability. Existing session bindings remain authoritative.

## Create dialog

Store overrides in task-create form state. Derive rows from the effective
workflow snapshot in workflow order. Group fixed references by profile ID.
Show direct step names and earlier-step recipient consumers where derivable.
For example, the Luna row shows Implement and PR. Guard traversal against
cycles and unresolved step references. Do not infer a new profile from a
historical session while constructing a new-task form.

Add a full-width Workflow agents region after the existing advanced option grid.
Each row shows the source name, affected steps, replacement picker, and reset.
Visible helper text states that changes apply only to this task. Use the
existing agent selector's labels, logos, eligibility, and search behavior.
Update the workflow summary to display the replacement consistently.

Collapse preserves state. Workflow/workspace changes and completed or abandoned
new-task forms clear it. Network errors preserve it. Do not add overrides to
last-used preferences. Carry the map through every normal create submission
path and retry payload. Session-create and task-edit modes have no new fields.

While the selected snapshot loads, show loading in the region. On failure,
show retry and block submission until the workflow can be validated. Profiles
that become invalid remain visible with a row error until reset or replaced.
No fixed profiles means no region after a successful load.

## Step previews in task surfaces

The task topbar uses `WorkflowStepper`. Workflow disclosures and move controls
share `useWorkflowMovePreview` and the workflow-move preview components.
Both the topbar and above-chat step controls must consume the task-aware
read-only move projection. Do not substitute profile labels only in the browser.

Extend `workflow_move_preview.go` and its routing inputs to use the same
step-bound effective profile resolution as execution. Preserve the existing
[move-preview contract](workflow-move-preview.md): reused recipients use their
effective runtime/session configuration, and new recipients use projected launch
configuration. Applicable step configuration rules still affect the projected
model. An unresolved dynamic model remains unknown. Preview never starts a
session or selects a provider through execution.

Show the replacement profile/model for Implement. For PR, resolve the Implement
recipient binding and show that session's effective model when available.
Before a binding exists, derive the expected model from the referenced step's
task-specific profile and projected launch configuration. Mark it as planned.
Keep the actual recipient unresolved and omit a fabricated session ID. Add a
separate optional planned recipient/model projection to the preview DTO, so
existing actual-recipient and move-outcome semantics remain intact. Cycle,
missing-reference, or unresolved dynamic-model cases remain unknown. Follow
references read-only with a visited-step guard. Do not create sessions.
Once the binding exists, actual effective session configuration supersedes the
planned projection. Both disclosures use the same provenance label.
Analysis and Review continue to show the initial recipient's effective model.
The current-step running-agent label uses the actual session profile/configuration.

Keep preview request and cache identity task-scoped. Preserve revision
invalidation on task, session configuration, binding, and workflow changes.
Switching between tasks must not show a previous task's replacement. Loading
and errors must not fall back to the original workflow model as authoritative.

## Mobile and accessibility

Reuse the full-screen task-create form and its single body scroll owner.
The existing Advanced settings disclosure is the nearest shipped exemplar.
Rows stack labels, affected steps, and full-width selectors on phones. Desktop
rows place the source and selector side by side. The picker uses the existing
responsive agent-selection surface, with its own option-list scroll region.

Keep the task form footer and safe-area behavior. Use dynamic viewport sizing,
28-pixel desktop controls, and at least 44-pixel phone/coarse-pointer targets.
Do not add a separate settings screen or nested flow. Share state and derivation
across viewports. Long labels wrap without document-level horizontal scrolling.
Label each selector with its source profile. Restore focus when the picker
closes. Localize all copy in English, Portuguese, and the three Chinese catalogs.
Generate Traditional Chinese values with the existing command.

## Related contracts

- [Fixed profile routing](workflow-step-fixed-profile-routing.md)
- [Recipient bindings and session lifecycle](workflow-profile-session-lifecycle.md)
- [Agent/executor compatibility](task-create-agent-executor-compatibility.md)
- [Advanced disclosure](../requirements/task-dependencies-create-dialog-advanced-settings.md)

Those routing designs describe the existing no-override path. This draft adds
an optional task substitution before fixed-profile session selection.
Implementation must reconcile their resolution summaries with this extension.

## Observability and verification

Use existing structured routing logs with source and effective profile IDs.
Use existing transition errors. No new metric or high-cardinality labels.

Repository tests prove migration and reload persistence. Service tests prove
validation and task isolation. Orchestrator tests prove all routing entry paths,
restart, explicit-target bindings, and unavailable-profile failures. Frontend
unit tests prove grouping, reset, workflow changes, and payload preservation.
Desktop and mobile E2E reproduce Analysis, Implement, Review, and PR with mock
agents and prove profile identity plus session reuse.

## Implementation plans

- [Delivery package](../../../plans/task-workflow-agent-overrides/plan.md)
- [Change workflow design](change-workflow.md) proposes explicit destination-map
  replacement during a move. The legacy dormant-map behavior above remains the
  contract for moves that omit that new option.
