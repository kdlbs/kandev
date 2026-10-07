---
status: current
system: tasks
created: 2026-10-07
requirements:
  - REQ-TASKS-WORKFLOW-START-SELECTION-001
---

# Workflow start selection system design

## Purpose and mapping

The task system owns persisted workflow selection and task placement. This
design specifies a presence-aware seam for one flag in ordinary step updates.
ROOT accepted the revised package and released implementation in the same primary.
Implementation and focused verification are in progress; hosted evidence is pending.

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKFLOW-START-SELECTION-001` | Caller inventory, Presence seam, Transaction and projections, Failure and compatibility, Verification |

## Caller inventory

Source inventory at `a458174b023b33fe16c755bfed762025bf3706a5`:

| Entry / writer | Current boundary | Treatment |
| --- | --- | --- |
| REST `PUT /api/v1/workflow/steps/:id` | `workflow/handlers.httpUpdateStep` -> shared `controller.Controller.UpdateStep` | Preserve request presence; keep JSON and route shape |
| WS/MCP `ws.ActionMCPUpdateWorkflowStep` | `mcp/handlers.registerWorkflowHandlers` -> `handleUpdateWorkflowStep` -> same controller | Preserve its `*bool`; keep guarded registration and response envelope |
| Domain settings patch | `backendapp.settingsOperations.updateWorkflowStep` -> controller through `settingsWorkflowController` | Inherits correction with unchanged interface |
| Direct `service.Service.UpdateStep` / `UpdateStepWithStartStepUpdates` | Complete model -> concrete workflow repository | Always explicit full-model flag |
| Sync reconciliation | `service/sync_apply.go` -> `Repository.UpdateStep` | Full definition remains replacement |
| Deleted-step reference cleanup | `Repository.ClearStepReferences` -> `UpdateStep` | Retain full-model contract; no writer migration |
| Exact Host workspace administration | `backendapp.pluginsWorkspaceAdminAdapter.updateWorkflowStep` -> `UpdateStepWithStartStepUpdatesIfUnchanged` | Preserve both version predicates and full configuration |
| Step create / import | Existing create methods and demotion transaction | Unchanged |

`workflow/handlers.registerWS` registers step list/get/template creation, not
an ordinary step-update action. Do not invent a new update transport there.
The MCP handler is the existing registered WS update path, also reached by MCP
tool dispatch. REST and MCP use the same `stepevents.Publisher`, with
`workflow-handlers` and `mcp-handlers` provenance respectively.

`apps/web/app/actions/workspaces.ts:updateWorkflowStepAction` already accepts
`Partial<Pick<WorkflowStep, ...>>` and filters `undefined`; name-only requests
are real supported input. No browser change is needed.

## Root cause and accepted evidence

`Controller.UpdateStep` reads a model before `EnsureWorkflowMutable`, overlays
supplied pointers, then loses flag presence at
`Service.UpdateStepWithStartStepUpdates`. The repository's
`updateStepWithDemotedStartSteps` demotes on the model's stale `true` and writes
the stale flag even when the request omitted it.

ROOT's read-only protected candidate and joined receipt prove both directions
with the real controller, two real Services, and one actual SQLite repository.
The existing mutable-workflow provider callback performs a second-service
explicit promotion after the snapshot and a real SQL workflow lookup. Initial
A -> selected B restores A and demotes B; initial B -> selected A clears A.
Responses are stale, and the first case resolves the wrong task start. Name-only
nonoverlap and explicit promote/clear controls pass.

Evidence: `/tmp/kandev-root-step-start-omission-candidate_test.go`, mode `0400`,
SHA256 `37751033fc06567c23a2e7c6a960800f6b9682cdf1ca9291c6b4a9a861f7a406`;
receipts under `/tmp/kandev-root-step-start-omission-discovery-20261007`.
Original native session `22270`, terminal chunk `1898ae`, exit 1, Go 0.208s;
ROOT qualified process group `3328882` gone. This evidence does not establish
registered HTTP, independent SQLite pools, or PostgreSQL behavior. Do not copy,
import, replay, modify, or remove the protected proof.

## Presence seam

The narrow service and repository entry points are
`UpdateStepWithStartStepIntent` and `UpdateStepWithDemotedStartStepsIntent`.
Both accept `(ctx, step, isStartStep *bool)` and retain the existing demoted-step
return shape. The ordinary controller passes `req.IsStartStep` unchanged after
its existing authorization, mutability, reference, and model validation.
The service validates and delegates just as its existing full-model method does.

The private repository update helper accepts this intent. Existing public
full-model and exact wrappers supply the model flag as explicit intent and
retain their signatures. Do not infer intent from whether the new value differs
from the observed value. Explicit false/true must remain explicit even when the
snapshot carries that same value.

The service holds concrete `*repository.Repository`; the controller holds
concrete `*service.Service`. There is no workflow step CRUD repository interface
to expand. `adapters.WorkflowRepo` contains participant/decision methods only.
`backendapp.settingsWorkflowController` names the unchanged controller request
signature. `WorkflowProvider`, MCP wiring, bootstrap injection, exact Host
adapter, and test doubles keep their public signatures. Audit references before
implementation; no optional interface assertion or fallback to the old method.

## Transaction and projections

Keep one existing `sqlx.Tx` for demotion, target update, flag capture, and commit.
Demote other rows only for non-nil true intent. Nil intent never invokes either
demotion helper, even if the observed model flag is true.

Use a fixed, rebound SQL assignment for this flag:
`is_start_step = CASE WHEN ? = 1 THEN ? ELSE is_start_step END`, binding integer
presence and value through the existing dialect helpers. All other assignments
retain current full-model behavior. An alternative with only this assignment
omitted is acceptable if equally small, but never reread a flag outside the
transaction and turn it into explicit intent.

After the target UPDATE succeeds and affected-row checks pass, capture the
target's saved `is_start_step` on the same Tx, scoped by step and workflow ID.
Use an integer scan matching this store's integer boolean columns. Keep that
value local until commit succeeds, then refresh `step.IsStartStep` for the
existing response and publisher. No post-commit reader query: a later writer
could otherwise replace the result with another request's selection. Other
model fields and timestamps keep their existing handling; this is not a
universal projection or patch rewrite. Close all query rows before subsequent
writes/commit and use Tx rebinding, never the separate reader pool.

SQLite factory writers already acquire `_txlock=immediate` at BEGIN; independent
store tests use that actual factory. On PostgreSQL, the target UPDATE holds its
row lock until commit, and READ COMMITTED evaluates the preserved-column value
on the current row after a competing writer releases it. This seam adds no
read-modify-write predicate for omission and needs no new advisory lock. Keep
existing explicit demotion atomicity, unique constraint, and error behavior;
do not expand into an all-writer serialization framework.

`idx_workflow_steps_single_start` already uniquely constrains selected rows per
workflow. Duplicate unconstrained starts are not the defect and are not an
asserted new invariant of this repair. Competing explicit promotions may retain
existing constraint/conflict failure behavior; failed demotions must roll back.

REST/MCP handlers publish the returned demotions and edited step only after
success. `gateway/websocket.RegisterTaskNotifications` maps the bus subject
`workflow_step.updated` to `ws.ActionWorkflowStepUpdated` (`workflow.step.updated`).
The frontend workflow WS handler replaces flags in active Kanban and multi-board
snapshots. Orchestrator subscribers invalidate step caches and reconsider queue
admission. Preserve those consumers and payloads, with no cross-request ordering
claim or new telemetry.

`Service.ResolveStartStep` reads `GetStartStep`, then first positional step when
no flag exists. The task service's ordinary `resolveWorkflowStep` uses it;
immediate agent starts and plan-mode first-step routing remain separate.

## Failure and compatibility

Retain `AuthorizeStep`, `EnsureWorkflowMutable`, `ValidateStepReferences`, and
validation errors. Missing or denied resources keep sanitized classifications.
Tx admission, UPDATE, scan, affected-row, or commit errors propagate without a
successful response/event; deferred rollback settles proposed demotions. Do not
add retries or hide native failures.

Retain exact workflow/step predicates and stale-conflict errors, including
demotion rollback when the target exact update is rejected. Full snapshots,
sync, imports, creation, and cleanup remain explicit. REST rejects null for
`complete_task_on_enter` and `disable_unclassified_fallback`; both REST and MCP
currently decode `is_start_step: null` as nil. Preserve those existing decoding
contracts rather than adding validation for this flag.

## Verification

The [single work order](../../../plans/workflow-start-selection/task-01-preserve-selection.md)
defines fresh permanent caller regressions, registered REST and guarded WS/MCP
dispatch through the actual `stepevents.Publisher`, exact event payload/count
assertions, resolver controls, exact/full-model compatibility, rollback and
access cases. Add small file-backed independent SQLite-store and env-gated real
PostgreSQL cases for changed SQL and its target-row lock boundary.
Both omission directions assert the actual selected ID from `ResolveStartStep`;
explicit-clear controls assert its actual first-positional fallback ID. The
unchanged task-creation call into that resolver and gateway payload mapping are
covered by the source consumer audit above. No new gateway hub or task-creation
fixture or execution is planned, and those consumers receive no production edit.
This bounds verification without changing the task-placement criterion.
No browser or mobile test is needed: persistence and projection data change with
no layout, touch, scrolling, navigation, or viewport-dependent behavior.

## Related sources

- [SQLite writer transaction admission](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md)
- [Workflow step ordering design](workflow-step-ordering.md)
- [Implementation plan](../../../plans/workflow-start-selection/plan.md)

The narrow repair and its compatibility rationale fit this design and work
order; `/record` criteria do not require an additional ADR or a new repository
convention. Public portable-format omission rules remain unchanged. This design
package has no public-documentation edit.
