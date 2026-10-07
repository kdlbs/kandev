---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-SETTINGS-UPDATES-001
---

# Workspace settings update system design

## Purpose and ownership

The existing workspace owner remains the settings persistence boundary. Its
service admits requests; its repository writes their field intent. This is a
bounded repair to request-driven persistence, not a general revision framework.
The [requirement](../requirements/workspace-settings-updates.md) owns observable
behavior; org-unit, executor-policy, and settings presentation designs remain
authoritative for their independent contracts.

| Requirement | Design sections |
| --- | --- |
| `REQ-WORKSPACES-SETTINGS-UPDATES-001` | Admission and presence; persistence seam; observations; compatibility; verification |

## Verified baseline path

`buildWorkspaceUpdates` in
`apps/web/app/settings/workspace/workspace-edit-save.ts` compares the local draft
with its saved baseline and includes only changed name, default executor,
default agent profile, and idle policy fields. `updateWorkspaceAction` in
`apps/web/app/actions/workspaces.ts` forwards optional values by JSON to
`PATCH /api/v1/workspaces/:id`. Neither needs a change.

`RegisterWorkspaceRoutes` registers that REST route and
`ws.ActionWorkspaceUpdate` on the dispatcher. Both construct pointer-bearing
`service.UpdateWorkspaceRequest`; REST also supplies `UnitID`, whereas the
WebSocket request has no unit field. Neither exposes `ExpectedUpdatedAt`.
Both return `dto.FromWorkspace`. Registration and handler access wiring must
remain unchanged.

At the qualified baseline, `Service.UpdateWorkspace` in `service_resources.go` validates the timeout, reads
the workspace, checks manage authority and an optional exact timestamp, applies
request fields to the loaded object, and passes the entire object to
`UpdateWorkspace` or `UpdateWorkspaceIfUnchanged`. The SQL in
`repository/sqlite/workspace.go` assigns all settings and `unit_id` from that
snapshot. Successful unrelated requests can therefore overwrite one another.
`Visibility` on the service request is unused; it is not a persisted permission
field. `task_sequence` is absent from this UPDATE.

Two independent service instances can read the same workspace, then both
successfully persist disjoint settings changes. If either instance later
writes its stale whole-row snapshot, it overwrites the other instance's
successful change. This occurs in either ordering; sequential updates and
explicit-false values do not by themselves produce the lost update.

## Admission and presence

Preserve existing timeout validation, `GetWorkspace`,
`requireWorkspaceManage`, exact timestamp precheck, and `moveWorkspaceToUnit`.
Do not use the returned row to replace admission or widen authority. The initial
snapshot supports admission, not implicit write intent.

Introduce a workspace-specific `models.WorkspaceFieldUpdate` and a required
`WorkspaceRepository.UpdateWorkspaceFields(ctx, id, update, expectedUpdatedAt)`
method returning `(*models.Workspace, error)`. The expected argument is a
`*time.Time`; nil selects the ordinary path. No type-assertion fallback to a
whole-row write is allowed. Keep the existing direct whole-row methods intact.

| Field family | Domain representation | Service conversion | Persistence |
| --- | --- | --- | --- |
| Name, description | `*string` | Preserve supplied empty values | Assign only nonnil fields |
| Four default IDs | `**string` | Outer nil for omission; otherwise inner result of `normalizeOptionalID` | Inner nil binds SQL NULL; nonnil binds normalized ID |
| Idle enabled/timeout | `*bool`, `*int` | Preserve false; reject timeout <= 0 before persistence | Assign only supplied fields |
| Unit | `*string` | Only an admitted actual move | Assign admitted destination only |

The double pointer is local to this nullable domain contract: it retains
explicit clearing after `normalizeOptionalID` returns nil. It creates no
generic optional-value abstraction. JSON null decodes to nil before this
conversion and continues to mean omission. Nil request fields must never be
reconstructed from the loaded workspace.

For unit intent, capture the original unit, run `moveWorkspaceToUnit` when the
request supplies `UnitID`, then include the destination only when that helper
actually changes placement. It currently ignores empty and same-unit requests,
requires `unit.manage` for scoped actual moves, consults `UnitPlacer.UnitOrgID`,
and rejects another organization. Omitted or ignored placement intent never
adds a unit assignment. Preserve existing no-op admission without adding new
permission rules. This does not solve revocation or other admission races.

## Persistence seam

Implement the method in a focused `workspace_field_updates.go` alongside the
existing SQLite/ PostgreSQL-capable repository. Follow the established
`UpdateWorkflowFields` approach: build a fixed ordered allowlist of assignments,
bind every value, use `r.db.Rebind`, and perform one UPDATE with RETURNING of
the workspace projection. The allowlist contains name, description, unit,
four default IDs, enabled, timeout, and `updated_at`. Request input never names
a SQL column. Always write a newly generated UTC `updated_at`, including empty
updates, preserving their existing timestamp/event semantics.

When expectedUpdatedAt is supplied, add the existing exact
`optimisticUpdatedAtPredicate` (`AND updated_at = ?`) before RETURNING, binding
the original expected time. The initial service comparison is insufficient:
the SQL predicate must reject intervening writes. Do not round timestamps,
introduce tolerances, or silently retry an exact conflict.

Use the repository's existing workspace select projection and scanner
normalization for the returned row. Extract a small scanner/projection shared
with `GetWorkspace` as needed; do not refactor unrelated list or cascade code.
Preserve the mapping of stored NULL/empty defaults to nil model values without
assigning those omitted columns. The current bool binding and scanning must
remain valid on both dialects, as verified by the existing PostgreSQL idle
policy test. No schema migration or persistence-pool change is needed.

A single statement avoids a stale read/whole-write sequence and returns the
row associated with that mutation. A missing ordinary target maps to the
existing workspace-not-found error; no returned row with an exact predicate
maps to `repoerrors.ErrTaskVersionConflict`, consistent with the whole-row CAS.
Statement or context failures propagate without publishing success. Constraint
aborts must roll back all assigned fields and timestamp. Cancellation before
SQL admission must leave the row alone; this creates no new post-commit
cancellation or busy-wait latency guarantee.

This boundary is smaller and more reliable than service locks, which cannot
coordinate independent handles/processes, or a new read/merge transaction,
which requires extra locking and reads for scalar fields. The established
allowlisted UPDATE RETURNING pattern directly constrains omitted assignments.
Rationale fits this local design; no additional ADR or system owner is needed.

## Responses and events

Return the repository-observed row through `Service.UpdateWorkspace` and pass
that same row to `publishWorkspaceEvent(events.WorkspaceUpdated, ...)` after
successful persistence. The initial object must not supply a stale response or
event. Preserve `dto.FromWorkspace` and the existing event keys, including the
unit and default IDs. Preserve the event formatter's RFC3339 timestamps rather
than inventing a precision or wire-shape change.

The earlier response in an overlap may legitimately precede the other write;
the later response must include changes already committed before its mutation.
A later writer may run before publication or delivery. This design promises
neither global event ordering nor that a response is current when received.
Tests compare each event with its own observed response and assert persisted
union after both writers settle, not equality of every historical response
with the final row. Observe actual event-bus delivery with bounded waits where
needed; Publish completion alone is not subscriber completion.

## Up-front compatibility inventory

All production direct callers at the baseline are accounted for here. Repeat
the inventory on the released implementation head and checkpoint new scope.

| Caller/writer | Contract and disposition |
| --- | --- |
| Registered workspace REST/WS handlers | Required field seam through the service; preserve request and response shapes |
| `backendapp/settings_domain_operations.go` workspace branch | Existing decoded partial request goes through the same service; preserve its catalog/validation wiring |
| `backendapp/plugins_workspace_admin.go` defaults command | Existing ExpectedUpdatedAt precheck, validation, idempotent observation, and exact write predicate remain; use field seam with fence |
| Repository `UpdateWorkspace` / `UpdateWorkspaceIfUnchanged` | Retain deliberate complete settings writes and exact full-row behavior; no production bypass caller found outside the service at baseline |
| `PlaceWorkspace` (`org_unit_placement.go`) | One-shot unit placement writer retained; ordinary omitted-unit updates preserve a placement committed before their write |
| `TransferWorkspaceOwnership`, `ClaimUnownedWorkspaces`, `org_data.go` | Existing ownership/organization assignments retained; no new serialization or authorization guarantee |
| `defaults.go` | Initial workspace insert and `office_workflow_id` update retained; excluded from partial settings allowlist |
| `task.go` number allocator | Independent task_sequence increment retained; not part of the faulty UPDATE |
| `plugins/instances/store.go` row-admission write | Existing owner-based no-op SQL retained; no generic writer coordination |
| `backendapp/e2e_reset.go` | Task/workflow and other-domain cleanup; no workspace settings-row update found; no reset rewrite |

The baseline default-column assignment search found only the complete settings
UPDATE; there is no separate default-reset writer to migrate. Empty default
requests through Settings or exact administration express those clears.

Interface implementations requiring an added method are the real repository,
`handlers/process_handlers_test.go` mock, `orchestrator/executor/executor_mocks_test.go`
mock, and `service_resources_test.go`'s `WorkspaceRepositoryStub` (also embedded
by `errWorkspaceRepo`). Real-repository wrappers embedded in service access,
delete, and unarchive tests inherit it; new read barriers must forward it to
the real repository. Preserve deliberate full-row fixtures in SQLite CRUD,
unit-placement, PostgreSQL-schema, executor-policy, orchestrator-idle, and
repository-policy tests. No second in-memory writer implementation or
conditional production fallback is warranted.

## Verification, mobile, and public documentation

Map criteria .1-.8 to independent service/real-SQLite overlaps, registered REST
and WS integration tests, presence/normalization controls, real constraint
rollback, exact match/intervening conflict, authorization, unit moves, and
deliberate full-write compatibility. PostgreSQL tests exercise actual physical
row contention, returned observation, exact predicates, nullable defaults,
boolean values, and rollback under the existing isolated-schema harness. See
the [single work order](../../../plans/preserve-workspace-settings-updates/task-01-persist-settings-fields.md)
for exact commands, test names, and release gates.

This repair has no rendered layout, touch, navigation, scrolling, store, or API
shape change. Desktop and phone use the same existing save builder and backend
contract; backend registered-flow evidence is the causal parity check. No new
browser test, UI preview, or product build is required. Shared Go/SQL code has
no platform-specific branch: execute narrow new cases on the existing hosted
Windows native suite to establish native database/scanner compatibility. Linux
tests and a native build alone do not substitute for their RUN/PASS evidence.

The public-doc assessment searched `docs/public/**`, README, and the screenshot
catalog. `docs/public/tasks-and-workflows.md` describes workspace defaults;
`docs/public/team-access.md` describes placement. Neither needs a new control,
setting, operation, or API explanation for this repair. Public documentation
changes are unnecessary for the design package and expected bounded fix;
record that assessment in delivery. Executor parking and reach policies are
not redefined. Internal contracts and delivery records are the four artifacts.

## Related contracts and decisions

- [Organization unit design](org-units.md).
- [Idle parking design](../../executors/system-design/idle-runtime-parking.md).
- [Existing task/workflow field-update contract](../../tasks/requirements/task-field-updates.md).
- [Settings save coordinator](../../../decisions/0046-settings-route-save-coordinator.md).
- [SQLite transaction entry](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md).
