---
status: current
system: tasks
requirements:
  - REQ-TASKS-FIELD-UPDATES-001
  - REQ-TASKS-FIELD-UPDATES-002
---

# Task and workflow field updates system design

The explicit metadata-merge boundary below covers criteria .8 through
.12. The preceding ordinary-field implementation and criteria .1 through .7 remain current.

## Ownership and dependencies

Tasks owns request intent, the task-row mutation, and its publication. This extends ordinary
field updates without converting internal full snapshots into patches. It reuses
[canonical parent admission](subtask-reparenting-drag-drop.md),
[association replacement](attach-workspace-source-replacement.md),
[human assignment](human-assignee.md), and [completion gating](task-completion.md).

Existing boundaries are `service/service_requests.go` (pointers), `service/service_tasks.go`
(UpdateTask/reference/priority/preparation/reload/publication), `service_task_metadata.go` and
models (protection/carriers), `service_members.go` (assignee reach/orphan fallback),
`repository/interface.go` (required/optional methods), `repository/hierarchy/hierarchy.go`
(admission/validator/reader and repository aliases), `sqlite/task_hierarchy_admission.go`
(locks/normalization), `sqlite/task.go` (updateTaskTx/guards/ledger/entries/runner/dispatch),
shared `internal/db/taskhierarchy.go` (dialect locking), and registered task HTTP/WS handlers
(mapping/DTO/events). Repository preparation/replacement/finalize remains separately atomic.
WorkflowStepID is service-only; no new transport field. Paths are task-relative except shared DB.

## Request-presence inventory

| Request field | Locked-row application and existing coupling |
| --- | --- |
| `Title *string` | Set only when present. Preserve current title otherwise. Explicit title removes `agent_title_pending` and `agent_title_owner_session_id` after metadata processing. Existing length check and REST/WS trimming remain. |
| `Description *string` | Present replaces, including empty. No title/owner mutation when omitted or description-only. |
| `Priority *string` | Present replaces after enum validation. Genuine priority-only requests retain scalar path. Add `AssigneeUserID == nil` to the fast-path guard: priority plus assignee currently drops the assignee. |
| `State *TaskState` | Present applies to current state; absent retains it. Completion guard evaluates actual current-to-requested transition. Return prior locked state for event bookkeeping. |
| `WorkflowStepID *string` | Service-level present applies to current workflow context. Absent retains current step, queue/WIP fields, workflow identity/overrides and runner projection. Existing transition/entry rules remain; no new WIP admission policy. |
| `Position *int` | Present retains literal position, including zero; absent retains current position and existing arrival behavior. Keep current explicit-position flags in `updateTaskTx`. |
| `ParentID *string` | Present goes through existing locked final parent validator, including explicit empty; absent uses current parent. Use existing materialized workspace mode/group normalization and admitted-parent marker. |
| `AssigneeUserID *string` | Preserve presence separately from the service-resolved/trimmed user ID. Absent retains current human assignee; empty clears it. No agent/session/runner reassignment. Existing reach validation stays before entity preparation; no new transaction-wide membership policy. |
| `Metadata map` | Nil preserves current. Non-nil goes through the existing protection operation against current metadata, then existing SQL replacement/pending-title behavior. Empty map is intentional ordinary replacement; do not merge arbitrary ordinary keys. |
| `Repositories slice` | Nil/no wire field/null omits; non-nil (including empty) prepares and later atomically replaces the set. Never carry it into the task-row patch or its transaction. |

## Typed repository boundary

Introduce `models.TaskFieldUpdate` and `models.TaskFieldUpdateResult` in a small model file.
The patch contains eight scalar pointers (title, description, priority, state, workflow step,
position, assignee, parent) and the nullable metadata map, using the existing types.
It carries the resolved assignee pointer, not an early
complete `Task`. It contains no repository inputs, policy callback, revision, or opaque field
mask. The result carries the committed candidate task, prior state, prior workflow step, and
effective parent-change flag, captured under the mutation lock. These identifiers are implemented; existing dependencies retain their own contracts.

Add the required method to `TaskRepository`:

```go
UpdateTaskFieldsWithParentAdmission(context.Context, string, models.TaskFieldUpdate,
    TaskParentValidator) (*models.TaskFieldUpdateResult, error)
```

Use the existing repository alias for the validator type. Implement the method in
`repository/sqlite/task_field_updates.go`. Required interface membership makes wrappers that
embed `TaskRepository` forward the concrete implementation. Update standalone interface fakes
only as necessary to compile and preserve their test purpose. There is no silent snapshot
fallback for an ordinary update. Keep the old `UpdateTaskWithParentAdmission` API and its
snapshot semantics for intentional callers; it still implements prior hierarchy tests.

Share the existing metadata protection calculation from a neutral task-model helper,
`models.ProtectedTaskMetadataUpdate`, retaining a service wrapper for existing callers/tests.
Move the calculation without changing its protected namespaces or shallow-copy behavior.
Do not import service into repository, duplicate its rules, or change other metadata endpoints.

## Mutation sequence

1. Keep authorization, supplied title/priority checks, early task lookup, normalized assignee
   reference check, and locked parent preflight before repository-entity preparation.
2. Genuine priority-only updates keep `UpdateTaskPriority`; all other ordinary updates supply
   the typed patch. Early snapshots remain preparation/authorization observations, never the
   source of omitted task fields at commit.
3. Begin the existing hierarchy transaction. Reserve SQLite's writer before reads. PostgreSQL
   uses READ COMMITTED, immutable workspace location and sorted workspace locks, then sorted
   current source/destination step locks, then the task row. Preserve the existing lock order
   and any arrival mutex required by the existing pipeline before opening the transaction.
4. Locate current source step under hierarchy reservation, using the explicit destination step
   only if supplied. Acquire existing step locks. Read the complete current task through the
   writer transaction, with `FOR UPDATE` on PostgreSQL **after step locks**. This last read is
   necessary: scalar priority/title/metadata writers can bypass workspace reservation and
   commit while a candidate is between its graph read and task-row lock. A workspace-only
   reread does not cover them. No read-pool escape is allowed.
5. Validate explicit parent against this current subject and transaction-bound reader with
   the existing validator. Build the candidate from current data, apply only patch presence,
   protect supplied metadata against current metadata, resolve explicit human title ownership,
   and reuse parent/workspace normalization. Preserve graph policy, cancellation and typed
   errors, including errors from related-row reads and encoding.
6. Call existing `updateTaskTx` with the same explicit-position-dependent position/deferred
   protection flags and attribution. It remains responsible for completion guards, source
   checks, hierarchy/provenance protection, encoding, ledger, entries, runner projection, and
   timestamp. No new workflow/runner/launch policy is introduced. Locked-current data naturally
   prevents an omitted state, workflow or other field from reverting a scalar prior commit.
   The typed writer also attaches private `taskFieldMetadataOmittedKey` presence to this
   transaction only; postcommit dispatch receives the original caller context so nested
   legacy writes cannot inherit patch presence. `updateTaskTx` passes it as
   `preserveLockedMetadata` to
   `buildTaskUpdateQuery`. Omitted metadata writes the locked current document, including
   sanctioned hierarchy/provenance/title-owner processing, rather than submitting it as a
   pending-title merge patch that deletes current JSON-null values on SQLite. Protected
   deferred-launch stripping is unnecessary for this locked document. Supplied maps retain
   the existing deferred protection, replacement and pending-title merge expressions; legacy
   full-snapshot callers supply no marker and retain their existing behavior.
7. Commit before returning the result and before dispatching existing step-entry effects.
   On failure return the original wrapped error and no successful result/dispatch. The service
   uses locked prior state/step for its existing state/manual-transition bookkeeping; state
   change requires an explicit state and an effective difference at this boundary.
8. Keep the ordinary response reread, separate association replacement/finalize, relation list,
   and events. A reread can observe a later commit; do not label it an exact receipt. Preserve
   the fallback candidate on reread failure and existing clear-parent marker. Association
   failure retains its own rollback and success-event suppression, without undoing the task row.

## Writer inventory and guarantee limits

Full-snapshot participation is directional: patches preserve preceding commits; later stale
snapshots retain their own overwrite rules. Audit included these writer families and recovery/initialization touches. Audit new SQL;
no assumed global coverage or silent migration.

- Ordinary Service.UpdateTask delegates use typed patches except genuine scalar priority.
  TaskPriorityRepository.UpdateTaskPriority, Office priority/project and task_reorder.go retain
  native row serialization, arrival/position locks and Office policy/publication.
- ClaimTaskTitleSession/SetTaskTitleIfPending retain title/owner CAS and human precedence.
  SetTaskMetadataKey*, RemoveTaskMetadataKey*, TakeTaskMetadataKeyIfDestinationStep,
  ClearManualMoveLifecycleMarkersIfCompleted, deferred CAS/prompt, handoff/carry/causation,
  orphan/recovery/launch-error and management/completion metadata keep predicates/receipts.
  Omission sees current records; explicit replacement retains protection and ordinary-key
  removal, without arbitrary key preservation.
- UpdateTaskState*, Office conditioned state/step/tree_holds keep owned bundles. Parent admission,
  DetachTask, bulk reparent, conditional restore and Office scalar parent retain serialization/
  normalization; no stricter Office policy or new state priority.
- UpdateTask, UpdateTaskPreservingDeferredLaunch, UpdateTaskIfWorkflowMatches,
  UpdateTaskWithExplicitPosition, exact/admission wrappers, MarkDeferredMoveAppliedForSession,
  full-row capacity/promotion and runtime/workflow snapshots remain full writes. Keep parent/
  workspace/position/title/provenance, receipt/WIP/queue/runner and legitimate handoff guards.
- Service.UpdateTaskMetadata uses explicit merge below. Office assignment/checkout/generation,
  participants and step-deletion retain independent bundles/projections. Creation/import/
  migration/reset/archive/unarchive/delete/cascades/sequence/default repair retain lifecycle
  authority/barriers, without guarantees across rebuild/delete. Association/folder/set/document/
  plan/comment/attachment/canvas/issue-watcher/queue writers retain separate transactions or
  narrow touches, without global cross-table atomicity.

## Failure, validation, and observations

Actual current-row validation/encoding/storage/cancellation failures roll back the task row,
ledger, entry allocations and runner effects in the existing transaction. Preserve `errors.Is`
for parent, missing task, cancellation and completion errors. Postcommit reads/events keep
their existing observation contract, including later commits and relation-read fallbacks.
There is no new revision, retry, mutex, event order, schema, flag, metric, or exact update mode.

## Verification and scope audit

Real-service SQLite tests must hold two independent services' delegated initial reads after
both actual snapshots exist, then explicitly release and join APIs in both write orders.
This reproduces the defect before the new API exists; a compile error is not RED. Add mixed
requests, omission/null/empty/same-field controls, metadata/title-owner races, real current-row
validation/rollback/cancellation failures, and state/workflow ledger/event checks.

Registered PATCH and WS dispatch use real service/storage and verify stored row, response and
actual events. Transport shapes stay unchanged; if a mapper must change, add its registered
case before publication. PostgreSQL tests run the touched new method through independent
connections in one private schema and observe actual backend lock waits, including a task-row
blocker that owns no workspace lock; elapsed time or a mocked validator is not proof.

Mobile-parity audit: backend persisted-data correction only; no UI, copy, layout, navigation,
touch, breakpoint or frontend state changes. Existing desktop/mobile subscriptions consume
unchanged DTO/events; browser/build/E2E work is not causal. Public docs are assessed through
`tasks-and-workflows.md` and `websocket-api.md`: design intent stays here; at implementation
add only a short ordinary-field omission/concurrency clarification if the current reference
needs it, explicitly excluding intentional metadata replacement/internal full snapshots.

## Decision scope

The existing workspace/step/task transaction is reused. A local typed patch boundary and this
explicit guarantee preserve enough rationale; `/record` does not warrant a separate ADR.
An in-process mutex cannot cover independent services/connections. Another early reread still
loses intent. Global revisions, generic transaction callbacks and arbitrary per-key merging
would alter contracts beyond the demonstrated defect.

## Requirement mapping

| Criteria | Design sections |
| --- | --- |
| `.1` to `.3` | Request-presence inventory; Typed repository boundary; Mutation sequence |
| `.4` and `.5` | Request-presence inventory; Writer inventory and guarantee limits |
| `.6` and `.7` | Failure, validation, and observations; Verification and scope audit |

All criteria refer to `AC-TASKS-FIELD-UPDATES-001`. Delivery is recorded in
[the implementation plan](../../../plans/concurrent-task-field-edits/plan.md).


## Explicit metadata-merge boundary

### Entry points, interfaces and dependencies

Inventory is grounded in base `93c80f75454f11a60c3e8458d70b3148194d7dac`.
The direct production caller of `Service.UpdateTaskMetadata` is
`TaskHandlers.httpUpdateTaskPortForwarding`, registered by `registerHTTP` as
`PATCH /api/v1/tasks/:id/port-forwarding`. It passes only `port_forwarding_enabled`, retains
strict boolean/body validation, calls the real service, and returns `dto.FromTask`.

Two GitHub issue operations call the *similarly named* `TaskIssueStore.UpdateTaskMetadata`.
`backendapp.githubTaskIssueStoreAdapter` implements that method using ordinary
`Service.UpdateTask` with supplied `Metadata`, not the merge service. Link copies a snapshot;
unlink deletes keys before replacement. Leave both calls, the adapter and its interface alone.
Migrating them to merge would break unlink and claim snapshot safety outside this contract.
There is no current WS metadata-merge action or new metadata wire field.

| Boundary | Reuse or bounded change |
| --- | --- |
| `service/service_workflow.go` | Keep `UpdateTaskMetadata` authorization, early existence observation, error propagation and postcommit reread/publication; send supplied intent instead of an early full task. |
| `repository/interface.go` | Add required `MergeTaskMetadata(context.Context, string, map[string]interface{}) error` to `TaskRepository`. No optional assertion or snapshot fallback. |
| `repository/sqlite/task_metadata_merge.go` | New domain-specific canonical implementation shared by SQLite and PG through existing driver detection and binding. Writes metadata and timestamp only. |
| `repository/sqlite/task.go` | Reuse `pendingTaskMetadataMergeExpression`, metadata object normalizers and existing timestamp source. Leave full-row methods and title CAS predicates unchanged. |
| `repository/sqlite/task_hierarchy_admission.go` | Reuse `hierarchyTxOptions`, SQLite writer reservation through `lockTaskHierarchy(ctx, tx, nil, nil)`, and materialized workspace identity rule. No parent validator or graph mutation. |
| `models/task_metadata_update.go`, `models/models.go` | Reuse protected key names and carrier stripping; preserve existing ordinary replacement helper. Any small raw-document merge protection helper stays domain-specific, not a general callback/merge API. |
| Storage primitives | `SetTaskMetadataKey*` and tx-capable `setTaskMetadataKeyWithExecutor` are one-key operations. They cannot provide one all-keys commit plus pending-title compatibility and ownership rules by independent calls. `lockMetadataRow` has PG row locking but textual missing-row errors and no independent-handle SQLite reservation; reuse its pattern without weakening typed task errors. |

`*sqlite.Repository` is the sole concrete production task repository and implements both
supported SQL dialects. Required membership forwards through embedded `TaskRepository` and
`*Repository` wrappers. Standalone existing fakes with the typed method occur in
`service/service_test.go`, `handlers/process_handlers_test.go`,
`handlers/task_http_handlers_external_id_test.go`, `handlers/task_update_repositories_test.go`,
and `orchestrator/executor/executor_mocks_test.go`; adapt only actual required implementations.
The construction-time gate in `service/service_task_field_updates_test.go` embeds the real
repository. `handlers/task_port_forwarding_test.go` currently intercepts the old snapshot
writer; update its data behavior and retain body-validation cases, with real registered-route
storage evidence in a separate test. Re-inventory method sets during compilation; do not hide
required capability behind permissive stubs used by the merge regressions.

### Canonical operation and lock order

1. The service checks task-write scope and performs its existing `GetTask` existence read.
   That task is never a write payload. It may be stale by admission to storage. Keep the
   observation boundary so existing-API RED can pause real early reads before real commits.
2. The repository shallow-copies the supplied map and filters server-owned input. Encode the
   complete admitted overlay before any mutation, without mutating nested input maps. Caller
   mutation concurrent with encoding is outside map ownership support. Ignore deferred-launch,
   step-handoff carry, handoff source/handoffs, Office carrier keys and generated-title control
   markers in this ordinary merge intent. These owner records remain in the current document;
   their sanctioned writers retain their own predicates. No owner record is restored from
   the early service read. No new namespace is made writable by a metadata endpoint.
3. Begin a writer transaction with `hierarchyTxOptions`. Before reads, SQLite reserves the
   writer via the existing empty-ID `lockTaskHierarchy` call. PG acquires transaction-scoped
   `pg_advisory_xact_lock(hashtextextended('task-metadata-merge:' + taskID, 0))`, as required
   by backend read-modify-write guidance, then reads this physical task row `FOR UPDATE`.
   The advisory namespace is task-specific and only coordinates this operation; the physical
   row lock is what interoperates with native scalar/CAS/ordinary writers. Never depend on
   process mutexes or one particular writer pool. Do not lock workspace/step rows after the
   task: this operation has no hierarchy or workflow mutation and requires no later locks.
4. Read only canonical metadata from the writer transaction, not a read-pool task projection.
   Translate missing rows to wrapped `ErrTaskNotFound`. Normalize SQL NULL, empty text or JSON
   `null` to an object; reject malformed JSON and non-object persisted values without repair.
   Use `map[string]json.RawMessage` for current document values to avoid round-tripping
   unrelated large numbers through `float64`. Raw bytes need not retain whitespace/order.
5. Protect current materialized `workspace.mode=shared_group` and `group_id` if the overlay
   supplies `workspace`, using the existing `preserveHierarchyWorkspace` identity rule with
   an isolated envelope or equivalent raw subdocument splice. Do not change ordinary sibling
   workspace keys or overwrite the caller map. Parent and all row identity fields stay outside
   the statement. Preserve raw unrelated fields and protected records, including literal null.
6. If current title is pending, apply the existing dialect-specific pending merge expression
   to the *supplied admitted overlay only*. Otherwise apply top-level replacement of supplied
   keys to the current raw object and encode once. Write one `UPDATE tasks SET metadata=?,
   updated_at=? WHERE id=?` (using the pending expression when applicable). Timestamp comes
   from `r.nowUTC()` after the physical lock and canonical read, never the early service read.
   Check affected rows and every database error. Commit all keys and timestamp together.
7. Return only after commit. Do not call `updateTaskTx`, `UpdateTaskFieldsWithParentAdmission`,
   parent/workflow validators, runner synchronization, ledger/entry allocation or postcommit
   step dispatch. Rollback on every failure; retain wrapped encoding/store/context causes.
   There is no new retry, schema, revision, metric or exact-command receipt.
8. Service rereads through its existing repository and publishes one `TaskUpdated` describing
   that returned observation. Do not publish an earlier candidate or synthesize state events.
   A later write can be observed. Keep existing behavior on postcommit read failure: return
   error and suppress publication, with the successful mutation still durable. Publish errors
   retain the existing event-bus contract. No cross-request event ordering is promised.

### Supported value compatibility

| Input/current context | Canonical merge result |
| --- | --- |
| Nil/empty overlay | Preserve current keys, including current explicit null; normalize absent metadata to an object; successful call still advances the modification timestamp and publishes after reread. |
| Nonpending current title | Replace each supplied ordinary top-level value, including JSON null, empty object/array, false and zero. A nested object replaces that key's old object; omission preserves other top-level keys. |
| Pending title on SQLite | Existing `json_patch` semantics for supplied values: explicit null deletes that supplied key and nested object patches recursively. An omitted current null is no longer accidentally resubmitted as deletion. Arrays replace. |
| Pending title on PG | Existing JSONB object concatenation: supplied top-level values replace, explicit null is stored, and nested objects replace. No new uniform recursive merge is introduced. |
| Protected ownership input | Ignore forged/replayed owner values. Retain canonical present/absent owner records and title/owner CAS outcomes. Ordinary metadata replacement still keeps its own existing title-control behavior. |
| Shared materialized workspace | Preserve current mode/group identity even for attempted null/object replacement; ordinary workspace subkeys follow the applicable supported merge rule. |
| Literal key punctuation | Keys are object members, including dots/quotes; do not route map keys through the existing unescaped `jsonPath` utility or interpret them as nested SQL paths. |

Preserving omitted nulls is part of omission intent, not an extension of explicit null support.
This correction removes incidental deletion caused by copying an early full document. Supplied
ordinary replacement and legacy full-snapshot pending-title null behavior remain unchanged.
The merge does not confer authority to set or clear generated-title controls; human title and
agent naming still use their existing typed/CAS paths. Tests must pin owner outcomes in both
race directions, including a human title committed after the merge and an owner record removed
before the merge. Protected-only input is an empty admitted overlay, not an ownership update.

### Writer relationships and exclusions

The earlier inventory remains authoritative for ordinary updates. For this explicit merge:

- Two merges have a symmetric guarantee for disjoint admitted top-level intents. Two nested
  changes under one top-level key do not qualify as disjoint, regardless of dialect behavior.
- Ordinary typed patches with omitted metadata (including human title, priority, assignment,
  parent, state, step and position) retain both edits in either order. An ordinary patch with
  explicit metadata retains replacement/pending-title rules: replacement committed after a
  merge may remove it; a merge committed after replacement overlays current metadata only.
- Native priority/project/position/state writers and title claim/set CAS serialize on the row.
  Merge changes no scalar column, and the metadata read follows their already committed writes.
  Metadata key/CAS, deferred-launch, handoff and provenance, carrier, orphan/recovery/launch
  error writers keep predicates and receipts. Omitted ordinary keys and protected owner values
  survive a preceding commit; later owner writes are authoritative under their own rules.
- Intentional full-row variants, exact/versioned commands, workflow admission, runner capacity
  and promotion, Office bundles, creation/import/reset, hierarchy writes and lifecycle deletion
  keep existing contracts. A later stale snapshot can overwrite ordinary merged keys or fields
  its own contract permits. No arbitrary later full-snapshot safety is asserted.
- Associations remain their separate atomic operation and F19 contract. Plans, messages, queues,
  canvas, integration and other task-touching tables retain ownership; no cross-table transaction
  is introduced. A merge must leave those tables, row fields and workflow effects untouched.

### Evidence and acceptance mapping for the amendment

Delivery uses [one sequential work order](../../../plans/concurrent-task-metadata-merges/task-01-canonical-metadata-merge.md).
Its test matrix maps `.8` to both real service merge orders/same-key/sequential controls, `.9`
to typed/scalar interactions and replacement exclusions, `.10` to value/owner/input controls,
`.11` to real store/encoding/cancellation rollback and row/effect isolation, and `.12` to actual
registered REST plus postcommit DB/DTO/event/error behavior. PostgreSQL evidence must exercise
this method through independent physical connections, observe a row-owner lock wait without a
workspace/advisory blocker, commit changed metadata/title before release, and verify the merged
row and timestamp. SKIP, mocked predicates, calls counted, or elapsed time are not lock proof.

No new browser/build/E2E work is causal: backend persisted state only, unchanged desktop/mobile
DTOs, layout, touch, navigation, copy and frontend behavior. The reference clarification in
`docs/public/websocket-api.md` next to partial updates describes the existing REST preference endpoint,
explicitly retaining ordinary metadata replacement and full-snapshot exclusions. No new public
page or UI wire field. The operation uses an existing persistence boundary; the named contract,
compatibility matrix and work order preserve rationale sufficiently, so `/record` adds no ADR.

## Workflow field boundary

Prior task/metadata sections remain current.
Delivery: [one sequential work order](../../../plans/disjoint-workflow-edits/task-01-preserve-workflow-patches.md).

### Audited writers and consumers

At the diagnostic baseline, UpdateWorkflow in `service/service_resources.go` overlays four pointers
onto an early workflow and calls full-row UpdateWorkflow. SetWorkflowHidden/SetWorkflowSource share
that footprint. All three now use field patches, retaining no-ops and the source/path bundle. Shared SQLite/PG
`repository/sqlite/workflow.go` rewrites mutable columns; UpdateWorkflowIfUnchanged atomically
checks timestamp. Direct snapshots, creation/deletion/reorder/steps retain separate contracts.

Registered `handlers/workflow_handlers.go` PATCH `/api/v1/workflows/:id` and WS `workflow.update`,
MCP config_workflow_handlers and backendapp settings domain operations retain partial presence.
Exact plugins supply ExpectedUpdatedAt. backendapp.workflowProviderAdapter and
web `components/settings/workflow-card-actions.ts` intentionally supply all four fields for
sync/import/profile/editor updates. Retain all supplied replacements; no stale full-draft merge.

### Local patch seam and failure behavior

models.WorkflowFieldUpdate carries pointers Name/Description/Prompt/AgentProfileID/Hidden/Source/
SourcePath. Required WorkflowRepository.UpdateWorkflowFields(context.Context, string,
models.WorkflowFieldUpdate) (*models.Workflow, error) returns the row; embedded interfaces forward it and only
necessary standalone fakes adapt. Ordinary/domain writers have no full-row fallback.

In `repository/sqlite/workflow_field_updates.go`, build one fixed-allowlist UPDATE assigning
only present columns plus updated_at, bind via Rebind, RETURNING workflowSelectColumns through
scanWorkflowRow. Empty/false are presence. Keep profile trimming, source normalization and
Boolean binding; omit identity/template/style/created_at. Empty ordinary requests still touch
timestamp. One atomic statement serializes through the native row, without storage RMW,
advisory/hierarchy locks, mutex or generic transaction. No PG RMW advisory rule applies. Missing rows use wrapped ErrWorkflowNotFound; scan/context/storage failures retain causes
and existing mappings. Prove rollback by pre-write cancellation or statement rejection;
never reverse a committed mutation or infer commit outcome from transport failure.

Keep authorization/initial existence observations; payloads derive only from request presence.
Return/publish the persisted row. Hidden/source no-ops remain unchanged, otherwise patch only
the owned field/bundle. A stale equal-value no-op promises no new same-field enforcement.
ExpectedUpdatedAt keeps initial equality check, fail-closed exactWorkflowVersionUpdater assertion
and UpdateWorkflowIfUnchanged SQL CAS. Never refresh/retry/downgrade a conflict. Ordinary patches
advance timestamp, invalidating stale exact commands. Full-row snapshot APIs remain intact.

Responses/events use the persisted observation; later commits/publication may interleave.
No global latest snapshot/order/receipt. Keep DTO/log/event/publication-failure behavior;
no schema/API/revision/metric/flag/UI.

### Evidence and scope mapping

002.1: independent bare real-store services, actual SQL-read barrier, both name/prompt and
hidden/source directions with physical connection proof. 002.2/002.5: omission/null/empty/false/
mixed/profile/same-field/no-field/full-draft controls and unrelated columns. 002.3: real aborting
UPDATE trigger/pre-write cancel/missing/auth/read-only failures, row/timestamp rollback, no success event,
stale/successful CAS and unavailable fencing. 002.4: registered router/WS dispatcher through
actual service/store/response/event recorder. Independently author tests; never import ROOT proof.

Shared dialect-sensitive RETURNING/binding requires scoped PG evidence: distinct backend PIDs,
native workflow-row blocker, observed lock wait, committed disjoint change before release,
joined patch and final row; also presence/normalization/hidden/source/CAS/statement rollback.
Use a bounded proven-owned private fixture only after ROOT heavy grant. SKIP/schema replay/
mocked predicates/elapsed time are not evidence. Every API/process is released and joined.

Mobile-parity: no rendered/touch/layout/navigation/copy/client-state change; existing desktop/
phone interfaces carry corrected values. Registered request-to-DB/response/event evidence is
causal; browser/E2E/build replay is unnecessary. Docs-maintainer updates the existing
public WebSocket API partial-workflow reference. Under /record this local seam needs no separate
ADR: the pair/work order retain rationale, early reads/mutexes cannot cover independent
connections, and exact fencing stays fail closed. No global writer/hierarchy project.
