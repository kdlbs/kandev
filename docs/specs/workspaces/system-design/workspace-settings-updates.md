---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-SETTINGS-UPDATES-001
  - REQ-WORKSPACES-SETTINGS-UPDATES-002
  - REQ-WORKSPACES-SETTINGS-UPDATES-003
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
| `REQ-WORKSPACES-SETTINGS-UPDATES-002` | Current catalogue publication; client compatibility inventory; independent client verification |
| `REQ-WORKSPACES-SETTINGS-UPDATES-003` | Successful creation publication; creation consumer inventory; creation verification |

## Verified baseline path

`buildWorkspaceUpdates` in
`apps/web/app/settings/workspace/workspace-edit-save.ts` compares the local draft
with its saved baseline and includes only changed name, default executor,
default agent profile, and idle policy fields. `updateWorkspaceAction` in
`apps/web/app/actions/workspaces.ts` forwards optional values by JSON to
`PATCH /api/v1/workspaces/:id`. Payload construction and that action remain
unchanged; the separate acknowledgement publication is extended below.

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

The backend repair for requirement 001 has no rendered layout, touch, navigation,
scrolling, store, or API shape change. The separate client publication extension
for requirement 002 is described below. For requirement 001, desktop and phone
use the same existing save builder and backend contract; backend registered-flow
evidence is the causal parity check. No new
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

## Current catalogue publication

Requirement 002 extends only the client acknowledgement boundary. The existing
current backend design and every requirement-001 contract above remain intact.
The [catalogue preservation work order](../../../plans/workspace-save-catalogue-preservation/task-01-preserve-current-workspaces.md)
records independent causal integration evidence and the two-file implementation.
Hosted review and merge remain separately gated in the delivery plan.

`WorkspaceEditPage` in `app/settings/workspace/[id]/page.tsx` renders
`WorkspaceEditClient`. `useWorkspaceEditForm` constructs
`buildWorkspaceSaveHandler` and registers it through
`useWorkspaceFormSaveContributor`. `SettingsSaveProvider.saveAll` retains the
submitted callback while awaiting its real `useRequest(updateWorkspaceAction)`
PATCH. Registered `registerWorkspacesHandlers` notifications publish independently
through `store.setState`. `AppSidebarWorkspacePicker` reads that same store.

Before the repair, the save handler awaited PATCH and then mapped the `workspaces`
array captured when it was built. A registered `workspace.created` can already have added a
visible switcher choice; that earlier array replaces it on acceptance. The
qualified evidence establishes client catalogue loss and accepted target save,
not backend deletion or any delete-route consequence. The existing backend
partial-update seam does not protect a later client whole-array replacement.

The immediate form caller supplies the existing `useAppStoreApi` to
`WorkspaceSaveHandlerOptions`, following the shipped Office appearance/Configuration
Chat idiom. Its narrow shape is
`getState: () => Pick<AppState, "workspaces" | "setWorkspaces">`. After the
successful await, the handler reads this owning provider's current
`workspaces.items` and current setter immediately before publication. Keep read, map, and setter
synchronous with no await or deferred scheduling between them. Do not add a
singleton, second store, generic updater API, or revision framework.

Map current rows by the acknowledged target ID. Preserve row order and every
nonmatching row. Spread the matching current row, then apply exactly the
existing projection: `name`, nullable `default_executor_id`,
`default_environment_id`, `default_agent_profile_id`,
`acp_idle_suspension_enabled`, and `acp_idle_timeout_minutes`. Keep existing
null coalescing on the three default IDs. Do not spread the entire response over
the catalogue row: that would widen ownership to description, scopes, owner,
unit, configuration default, and timestamps. A map cannot insert an absent
row; this does not define target-deletion lifecycle or navigation behavior.

Keep `setCurrentWorkspace`'s existing functional response merge and
`setSavedState`'s existing accepted-response/draft fallbacks in their existing
success path. Keep local drafts, contributor IDs/revisions, no-dirty return,
error toast/rethrow, permissions, validation, and navigation intact. Do not
change `buildWorkspaceUpdates`, API payload, backend routes, schemas, store
slice, WS handlers, picker markup, or response/event types. Same-target settings
revision arbitration remains excluded; accepted projected fields keep their
existing behavior even if another target update arrives during PATCH.

`setWorkspaces` updates items and retains non-null active identity; it does not
reset `activeIdRevision`. Tests must observe the identity and revision current
at publication, including an intervening real `setActiveWorkspace` action. Do
not invent selection behavior for an empty/missing target catalogue.

## Client compatibility inventory

Read-only audit at the qualified source baseline:

| Consumer | Existing behavior and disposition |
| --- | --- |
| `workspace-edit-save.ts` / immediate `workspace-edit-client.tsx` caller | Sole production changes; current-store read on accepted Save |
| `buildWorkspaceUpdates` / `updateWorkspaceAction` | Optional changed fields and real PATCH stay unchanged |
| `SettingsSaveProvider` / navigation guard | Submitted revisions, error state, newer edits, and route protection remain consumer contracts |
| Registered `workspace.created/updated/deleted` | Real current-store writers retained; integration stimuli, no handler edits |
| `AppSidebarWorkspacePicker` / `navigation/app-nav-sheet.tsx` | Desktop/shared phone consumers of current catalogue; unchanged choices/selection composition |
| `useWorkspaceDeleteDraft` in the immediate caller | Captured filter remains read-only; no causal qualification or delete-route claim in this package |
| `workspaces-page-client.tsx` creation | Captured prepend observed read-only; separate route not admitted by this proof |
| `WorkspacePlacementCard` / `placeWorkspace` | Immediate named move with its own API, local draft/rollback; no settings coordinator migration |
| Office `buildSaveAppearanceHandler` and `ConfigChatAgentSection` | Already read owning current store after await; do not remigrate corrected callers |
| Configuration Chat `saveDefaultConfigProfile` / `useUpdateWorkspaceInStore` | Existing best-effort partial default save with current-store updater; read-only, no change |
| Backend partial settings, exact administration, full writes, placement writers | Existing requirement-001 inventory and contracts remain unchanged |

The bounded inventory was repeated after implementation release with no scope
expansion. A similar captured array alone supplies no authority to expand
production ownership.
Checkpoint an unexpected caller or required boundary change with ROOT.

## Independent client verification, mobile, and documentation

Use one independently authored permanent
`app/settings/workspace/workspace-edit-save.integration.test.tsx`, optionally
with a colocated `.test-helpers.tsx` for fixture size. Mount the real Page,
StateProvider/createAppStore, real routing, ToastProvider, TooltipProvider,
SettingsSaveProvider, and AppSidebarWorkspacePicker. Drive the actual Name and
settings controls and coordinator, real API action/useRequest, and registered
workspace notification handlers. Only external fetch/WS delivery is simulated;
no product component, store, coordinator, router, action, or UI primitive mocks.
A small observer may expose the real owning store/coordinator without replacing
production behavior. Strict transport routing must reject unexpected requests.

The work order names the criterion-to-case matrix and exact commands. Addition,
update, and removal overlaps require independent causal failing evidence before
production edits. Observe current-store values and actual rendered switcher rows
both before and after resolving the held PATCH; assert the accepted target too.
Use independent metadata/selection, unchanged success, pristine, rejection/newer
edit, accepted/newer edit, and payload-presence controls. No protected ROOT proof
source is read, copied, imported, or replayed. Additional claims require their
own causal coverage rather than a broadened assertion on an unrelated route.

Mobile parity uses the skill's pure state/data exception: unchanged JSX,
composition, copy, touch targets, scrolling, navigation, and breakpoints share
this publication path. The shipped phone `AppNavSheet` uses the real picker.
Targeted real component/provider evidence satisfies this bounded repair; no new
Playwright case, UI preview, screenshot, or product build is planned. Any rendered
or navigation change invalidates this assessment and requires a ROOT checkpoint.

Public procedures remain accurate: workspace defaults in
`docs/public/tasks-and-workflows.md`, placement in `docs/public/team-access.md`,
and the README/screenshot catalogue need no new operation, label, API, or image.
Internal contracts and the four-file package describe the repair. The implemented diff retains
this assessment; no public documentation edit or new ADR is needed for the local
current-store idiom.

## Successful creation publication

Requirement 003 extends the same current-catalogue boundary to the explicit
Settings Add Workspace action. The backend field-presence contract, merged
Save implementation, and their existing delivery records remain unchanged.
The [creation work order](../../../plans/workspace-create-catalogue-preservation/task-01-preserve-create-catalogue.md)
records the completed local implementation and validation. Hosted review and
verified merge remain separate delivery gates.

`WorkspacesPage` in `app/settings/workspace/page.tsx` is mounted at
`/settings/workspaces` by `src/settings-routes.tsx`. It renders
`WorkspacesPageClient`, whose `handleAddWorkspace` trims the name and awaits
real `useRequest(createWorkspaceAction)`. That action uses its existing
`fetchJson` boundary to POST `/api/v1/workspaces` and propagate backend errors.
At the diagnostic baseline, the callback called `setWorkspaces` with
`mapWorkspaceItem(created)` followed by the `items` captured before the await.
Registered workspace notifications could already have changed the owning store.
Replacing it with the captured array lost those current choices; it did not
delete a backend row.

The implemented handler uses the existing `useAppStoreApi` in this component
to read the initiating provider's current items and setter only after successful
acknowledgement. It keeps that read, mapping, identity filtering, and publication
synchronous, without an intervening await or scheduled callback. It reuses the
existing `mapWorkspaceItem` unchanged for the accepted response and prepends
that mapped item to current rows whose ID differs from the accepted ID. This is
a local same-identity upsert: it accepts one canonical response descriptor, removes any
already-notified occurrence of that identity, and keeps every nonmatching row
and its relative order. Do not reconstruct existing descriptors, spread the
response across unrelated rows, or read another provider/global store.

The accepted response continues to own its mapped descriptor, including caller
scopes, role, unit, member count, configuration default, Office identity, idle
defaults and timestamps. This does not add same-ID timestamp arbitration or a
guarantee that an earlier creation response overrides a later independent edit
in chronological order. Subsequent registered events retain their current
semantics. Independent workspace metadata is retained exactly as it exists in
the current catalogue at publication.

Keep `setWorkspaces`' existing non-null active identity retention and unchanged
`activeIdRevision`. Its no-active-ID fallback still selects the first row
without adding a new selection revision. A registered creation on an empty
catalogue has its own existing active-ID/revision behavior; retain that result
if it precedes acknowledgement. An intervening real selection or deletion may
already have changed active identity/revision; preserve those current values.
Do not capture them at request start or invoke a selection action on success.

Keep request payload, blank-name return, request status, accepted form clearing
and closing, and rejected-form/toast behavior intact. The sole expected
production edit is `workspaces-page-client.tsx`, using an existing context API;
no store slice, mapper, WS handler, transport, backend, or layout edit is needed.
Checkpoint any causally necessary extra immediate glue before expanding scope.

### Creation consumer inventory

Read-only audit at baseline `714ee9c2c6e52b90272e56613029826f40646719`:

| Boundary / consumer | Contract and disposition |
| --- | --- |
| `WorkspacesPage` / `WorkspacesPageClient.handleAddWorkspace` | Owning creation publication seam; read live provider state after POST acknowledgement |
| `StateProvider` / `createAppStore` / `useAppStoreApi` | Real per-provider ownership; nested providers share their parent; isolation tests use independent roots |
| `useRequest` / `createWorkspaceAction` / its `fetchJson` | Existing POST, trimmed payload, status and real backend error; read-only |
| `mapWorkspaceItem` | Authoritative accepted descriptor and absence/default mapping; read-only |
| Registered `registerWorkspacesHandlers` | Created upsert, updated row projection, deleted membership/active fallback; actual test stimuli, no edits |
| Workspace slice `setWorkspaces` / `setActiveWorkspace` | Existing identity/revision/fallback and explicit-selection behavior; read-only |
| `orderWorkspacesForDisplay` / `WorkspaceListItem` | Management list promotes active row without reordering other rows; store order differs legitimately from visible order |
| `AppSidebarWorkspacePicker` / shared `WorkspacePickerContent` | Real catalogue choices and names, existing selection/navigation; read-only |
| `AppNavSheet` | Uses the same picker outside its phone menu scroller; unchanged state semantics and composition |
| `useWorkspaceSectionCounts` / section API clients | Independent read effects on real management cards; transports return legitimate empty count data in tests |
| Merged `buildWorkspaceSaveHandler` / immediate form caller | Existing current-store accepted Save projection; requirement 002 retained, no replay or remigration |
| Delete draft, placement, Office onboarding, backend creation/bootstrap | Independent boundaries, read-only; no creation lifecycle or deletion claim |

The previous Save inventory's creation row describes the scope of that earlier
proof. Requirement 003 now qualifies this separate creation callback; it does
not expand the earlier work order or its tests. Adjacent UI settings and Kanban
bootstrap requirements remain independent. No new system owner or ADR is
needed for this existing provider-store idiom.

### Creation verification, mobile, and public documentation

Independently author
`app/settings/workspace/workspaces-create.integration.test.tsx`, with an optional
colocated `workspaces-create.test-helpers.tsx`. Mount the real `WorkspacesPage`,
`StateProvider` (using actual `createAppStore`), Toast/Tooltip providers, real
routing and sidebar picker. Submit actual Add Workspace controls through real
`useRequest`, action, and client. Deliver external messages through the actual
registered workspace handlers bound to that provider. Mock only external
fetch/WS delivery, including legitimate section-count responses; reject any
unexpected transport. An observer may expose the owning store without replacing
product hooks, actions, handlers, primitives, components, or mapper.

The linked plan maps criteria .1-.5 to a bounded causal matrix: held success
with independent addition/update/deletion; notifications after settlement;
same-ID notification before and after acknowledgement; descriptor/default,
ordinary/empty/blank/rejected creation; current selection and independent-store
controls. Reach actual store, picker and management choices before and after
held acknowledgement and assert the accepted workspace as well. Expected
regression assertions, rather than fixture exceptions, establish RED. Settlement
controls may pass before the fix. Own and join deferred transport, asynchronous
count reads, and cleanup; leave no timers or provider observations behind.
No protected ROOT source proof is read, copied, imported, or replayed.

Mobile parity uses the skill's pure state/data exception. This correction
changes no JSX composition, layout, copy, controls, touch, scrolling, navigation,
or breakpoints. The phone `AppNavSheet` consumes the same picker/catalogue.
Real component/provider tests prove the shared publication outcome; no new
Playwright test, browser session, screenshot, UI preview, build, or database
run is planned. A surface change invalidates the exception and checkpoints ROOT.

Public-doc audit: `docs/public/tasks-and-workflows.md`'s Create a workspace
procedure still directs Add Workspace then naming and adding; Kanban bootstrap
and workspace defaults remain unchanged. `docs/public/team-access.md`'s placement
instructions, root README, and screenshot catalogue remain accurate. No public
docs change is needed: this restores choices in the existing procedure without
adding an operation, setting, terminology, wire contract, or image. Internal
requirement/design and delivery records carry the change.

## Related contracts and decisions

- [Organization unit design](org-units.md).
- [Idle parking design](../../executors/system-design/idle-runtime-parking.md).
- [Existing task/workflow field-update contract](../../tasks/requirements/task-field-updates.md).
- [Settings save coordinator](../../../decisions/0046-settings-route-save-coordinator.md).
- [SQLite transaction entry](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md).
