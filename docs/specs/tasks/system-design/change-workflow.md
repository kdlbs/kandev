---
status: draft
system: tasks
requirements:
  - REQ-TASKS-CHANGE-WORKFLOW-001
  - REQ-TASKS-CHANGE-WORKFLOW-002
---

# Change workflow design

## Ownership and evidence

The task system owns this vertical capability, including its form and persistence.
`MoveTaskWithOptions` in `internal/task/service/service_workflow.go` already changes
workflow membership. The HTTP handler permits an active primary session. The
service still rejects other starting/running sessions and invalid workspaces.
This design preserves those checks; it does not require every session to be idle.

`Task.WorkflowAgentOverrides` stores one workflow ID and fixed-step bindings.
`normalizeWorkflowAgentOverrides` validates grouped input during creation.
`buildWorkflowAgentOverrideRows` derives grouped rows in the task-create form.
`PreviewWorkflowMove` predicts existing routing, and the move hook reconciles
committed versus failed moves. Extend these boundaries instead of creating a
second workflow transition engine.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-TASKS-CHANGE-WORKFLOW-001 | Shared form, Preview, Mobile contract, Failure handling |
| REQ-TASKS-CHANGE-WORKFLOW-002 | Request contract, Validation and persistence, Routing and compatibility |

## Request contract

Extend the existing HTTP move and move-preview request with an optional
`workflow_change` object. Proposed shape:

```json
{
  "workflow_id": "destination-workflow",
  "workflow_step_id": "analysis-step",
  "position": 0,
  "workflow_change": {
    "expected_workflow_id": "source-workflow",
    "expected_step_id": "source-step",
    "expected_updated_at": "source-task-version",
    "agent_overrides": {"fixed-source-profile": "replacement-profile"}
  }
}
```

These are proposed fields, not existing APIs. The timestamp uses the task DTO's
RFC3339 representation and a typed timestamp comparison at the write boundary.
The existing position field retains its current server-owned placement semantics.

Absence or null retains the legacy move contract. A present object requires all
expected-source fields and an `agent_overrides` object; `{}` explicitly selects
destination defaults. Reject malformed or missing fields. A present object is
valid only for a different workflow in the same workspace, on a normal task.
Reject Office/ephemeral conversion and archived tasks. Office ownership is
identified by the task's `is_from_office` projection and by a non-empty
`project_id`; the service checks both so a project-owned Office task remains
excluded even when a caller's task projection is incomplete. Keep hidden
managed workflows out of the picker and reject them as explicit change
destinations.

Carry the typed option through `MoveTaskOptions`, HTTP, and the existing WS move
adapter. Both adapters must validate it consistently. MCP/plugin/bulk callers
omit it and retain their schemas and behavior. Do not use generic task updates
to edit this record. The frontend always sends the explicit object from this form.

Return existing move response fields, including entry identity and the task DTO.
Use a stable conflict code with HTTP 409 for a stale expected source/version.
Use structured validation codes and a source-profile ID for row errors. Localize
code-based messages in the client; do not display raw internal errors as new copy.

## Validation and persistence

Extract reusable validation from `workflow_agent_overrides.go` without constructing
a synthetic create request. Resolve the task's actual executor/profile and
workspace, rather than a new-task workspace default. Validate fixed source
membership, replacement visibility, enabled state, and executor compatibility.
Missing source definitions remain identifiable by ID so a valid replacement can
repair them. A fixed profile that remains selected as its default must also pass
eligibility checks. Explicit session targets are not fixed-profile sources.

Normalize once into destination step bindings using the existing model. Build a
candidate task copy with the destination workflow and new map for profile checks.
Retain the real source workflow/step and source session separately for exit policy.
Preflight must see the same candidate map that execution will read after commit.
Extend the existing preflight interface where it currently reloads the old task.
Never temporarily persist the candidate merely to run a preview or preflight.

Within the existing admission transaction, check expected workflow, step, and
task timestamp against the locked current row. Apply the candidate override map
in the same write as workflow membership, step admission/queue state, lifecycle
markers, and transition identity. Preserve the repository's workspace, step,
then task lock order. A new guarded admission option must not reuse the existing
step-CAS rebase blindly: that helper currently copies only a restricted field set
and would drop the replacement map and manual-move markers.

The existing nullable `tasks.workflow_agent_overrides` column is sufficient.
No new storage table or migration is planned. Audit both admitted and WIP-queued
write paths in `internal/task/repository/sqlite/task.go`; the shared repository
must retain SQLite/PostgreSQL SQL portability. Rejected CAS or transaction failure
must publish no successful move event. Extend repository tests with an injected
write failure and a write-time concurrent edit/move, not only a service pre-read.

After commit, existing task events carry the complete task projection. Routing,
future queued promotion, and restart load the new map. Never implement this as a
move followed by an independent override update.

## Routing and compatibility

Use existing task-aware fixed-profile resolution. Initial and earlier-step
bindings stay authoritative. A replacement does not mutate an old session's
profile, override a `new` start policy, or create a missing prior-step binding.
Keep source retirement, workflow completion, WIP, and dependency gates intact.

The explicit change replaces the single stored map. Returning through this form
starts with destination defaults; previous maps are not restored. Disclose this
in the form. Legacy moves without `workflow_change` keep the existing dormant-map
behavior: leaving its workflow makes the map inactive, and returning can reactivate
it. This distinction avoids changing bulk, plugin, or MCP semantics silently.

Retain existing lifecycle markers and event ordering. A committed transition
whose agent launch subsequently fails remains a committed move with a visible
launch error. Do not undo membership or submit a second move automatically.
Existing entry-identity fencing owns stale completion and transition deduplication.
Tests must prove that an old source turn cannot advance the destination workflow.

## Preview

Extend `WorkflowMovePreviewRequest` and the handler's service validation path with
the normalized candidate map. `resolveWorkflowMovePreviewInput` must receive the
candidate destination choices while reading source policy from the original task.
Task reloads inside helper calls must not discard the candidate map.

The preview remains read-only and advisory under
[the existing preview contract](workflow-move-preview.md). Project actual reused
session models, expected new-session models, and unresolved recipients honestly.
Use the existing `dispatch`, `source_disposition`, context-reset, and notice fields.
Do not label skip-step-prompt as a guarantee that no session will start.

Extend `useWorkflowMovePreview` request, in-flight key, and invalidation inputs
with a canonical sorted mapping and expected source identity. Retain cancellation,
bounded concurrency, and late-response guards. Draft changes, routing changes,
and reconnect invalidate the preview. Unrelated activity must not cause a request
loop. Preview infrastructure failure remains nonblocking; authoritative data or
selection validation failures prevent submission.

## Shared form and entry points

Add a proposed `ChangeWorkflowDialog` and a domain hook for task-scoped draft state.
The hook owns loading, selectors, validation, preview, submit, and reconciliation.
Mount the surface outside the menu that triggers it, so closing the menu cannot
destroy its draft. Freeze the subject task ID when opening; changing active tabs
must not retarget a pending submission.

Replace single-task workflow submenus with one `Change workflow...` item in:

- `kanban-card-menu-items.tsx` and `kanban-card-menu.tsx`.
- `task-move-context-menu.tsx` and `task-switcher-context-menu-move-items.tsx`.
- `use-task-actions-menu.ts`, task action dialogs, and task management surfaces.
- `task-command-items.tsx` and `task-command-choices.tsx`.

Keep `Move to` for same-workflow steps. Bulk selection keeps its existing direct
move flow, labelled `Change workflow for selected tasks`; no grouped agent form
is implied. Keep action IDs/test IDs compatible where useful while updating tests
to the new visible flow. Internal identifiers do not need cosmetic renames.

Load destination details on selection even when no multi-board snapshot exists.
Use existing workflow/profile APIs and access checks. Do not treat an uncached
workflow as empty. Sort by workflow position and retain personally hidden steps.
Require a destination step rather than guessing from matching names or ordinals.

The form order is source summary, destination workflow, destination step,
Workflow agents, conversation relationships, entry preview, and actions.
Reuse or extract the create-form row derivation and profile picker. Do not import
the entire task-create form. Explain task-only scope and replacement of the old
map. Show defaults/reset explicitly. No fixed profiles means an informative
empty region after successful loading, not a hidden loading failure.

Existing one-shot move options may remain in a collapsed section, with their
current defaults. Mapping is not a one-shot option and never travels in the
pending one-shot marker. Reset-context remains off by default.

## Mobile contract

Use `useResponsiveBreakpoint` for a phone form and wider dialog. The nearest
shipped exemplar is `components/kanban/mobile-menu-sheet.tsx`: an inset full-height
Drawer with fixed header and one internal scroll body. The task-create agent
override form supplies stacked labelled selectors and the responsive agent picker.

Phone entry is a visible task overflow action, not long press. This occasional
multi-field operation needs a full-height form because workflow mappings can be
long. A short nested menu or narrow table cannot show relationships and errors.
Use a fixed title/close header, vertically stacked fields, and a fixed bottom
`Change workflow` action above the safe area. The body owns vertical scrolling.
Use dynamic viewport units and no document horizontal overflow. Pickers own only
their temporary option-list scrolling and restore focus on close.

Desktop uses ordinary 28px controls. Phone/coarse-pointer hit targets are at least
44px; use 48px nominal primary-action sizing to tolerate fractional bounds.
Task state and mutations are shared; only presentation differs. Returning focus
targets the original trigger, or the nearest surviving task/board control after
the card leaves its source board. Do not change saved desktop preferences.

## Failure handling and observability

Validation failures retain the draft with row errors. Stale task conflicts reload
the task and require another explicit submit. If the task already moved elsewhere,
discard the stale source identity and explain the change. Revalidate retained
destination choices instead of silently overwriting the other operation.

On a transport timeout, refresh the task before retry. If its destination and
normalized map match, present the observed result without replaying entry effects.
Otherwise offer refresh/retry with the current source version. Disable duplicate
submit locally; the write-time source/version check protects repeated requests.

Reuse current structured move/route logs and transition history. Log profile IDs
only where existing routing diagnostics do; add no prompts, secrets, or new metric
labels. Board/source removal, destination insertion, sidebar, and open task stepper
must reconcile from the committed task using existing event/store paths.

## Related contracts and delivery

- [Override routing](workflow-agent-overrides.md)
- [Session lifecycle](workflow-profile-session-lifecycle.md)
- [Session policy decision](../../../decisions/2026-08-31-workflow-profile-session-switch-policy.md)
- [Implementation package](../../../plans/change-workflow/plan.md)

No new ADR is needed. This feature reuses the existing storage owner and routing
policy; the request opt-in and legacy compatibility rationale are captured here.
Reconcile creation-only override and single-task submenu wording during delivery.
Do not rewrite completed companion plan results or publish draft behavior as shipped.
