---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-013
---

# Managed conversation exclusive deletion admission design

## Ownership and status

Plugins owns the exact installation-scoped lifetime contract; Tasks supplies
the actual lifecycle and native persistence boundary. This focused supplement
preserves the [broad coordination design](managed-coordination.md) and its
draft/history. That file is 32,687 bytes at proofbase, close to the 32 KiB limit.
Its atomic settings section explicitly excludes `DeleteManaged` preflight and
removal. This supplement owns that gap rather than extending the earlier
settings guarantee by implication.

The existing [coordination ownership ADR](../../../decisions/2026-09-25-plugin-coordination-platform.md)
continues to govern. The accepted shared SQLite writer-admission ADR and
Platform persistence design define the DB operational boundary; the single
linked work order owns implementation, acceptance evidence and delivery.

## Requirement mapping

| Criteria for REQ-PLUGINS-MANAGED-COORDINATION-013 | Design boundary |
| --- | --- |
| .1, .2, .3, .7 | Current native admission and participating writers |
| .4, .5 | Exclusive admission, preparation, final deletion and reconciliation |
| .6 | Registered Host and receipt path |
| .8 | Compatibility boundary |

## Verified producer and root cause

At `d803d6f209030789d14751db90c5e9eaf5ba74a6`,
`internal/task/service/managed_conversation_settings.go:242` implements
`AgentConversationService.DeleteManaged`. It uses process-local `ensureLocks`,
discovers a retained task, checks detach and expected revision, then calls
`deleteManagedConversationTask` with only task ID. An independent service can
commit a new revision or detach after this preflight. No current predicate
reaches lifecycle admission or repository deletion.

`internal/backendapp/services.go:649` registers the real `taskSvc` through
`SetTaskDeleter`. `agentConversationTaskDeleter` currently exposes only
`DeleteTask(context.Context, string) error`. `deleteManagedConversationTask`
uses that lifecycle service when registered and otherwise falls back to the
bare task repository. That fallback cannot serve as a new exact managed
deletion guarantee. Legacy `Delete` and `DeleteAllForPlugin` have their own
existing absence/error semantics and must remain precise.

ROOT's immutable proof uses actual `createTestService`, real SQLite task and
session persistence, real `state.Store`, and two production conversation
services. Its construction-time forwarding gate pauses before actual
`Service.DeleteTask`, rather than mocking deletion or mutation predicates.
Both `accepted_update_after_check` and `detach_after_check` delete an
independently accepted retained row and incorrectly return OK. Current-delete
and already-stale controls pass. Accept the supplied joined behavioral RED
receipt without replay; the earlier fixture compile error is not behavioral
evidence. See the manifest for the immutable paths and hash.

## Actual lifecycle and native boundaries

`Service.DeleteTask` calls `deleteTaskWithReasonAndOptions`, which authorizes
`authz.ScopeTaskWrite`, uses the archive deadline, and calls
`deleteTaskWithReasonAndDBDelete`. The latter reads task, sessions, worktrees,
dirty-worktree consent, environment, attachments, and runtime stop targets.
Before final row deletion it performs:

1. `preserveTaskEnvironmentForActiveBorrower`, invoking
   `TransferTaskEnvironmentOwnership` with owner/generation checks. This is an
   independently committed environment mutation, not a read-only inventory.
2. `canvasCleanup.CleanupTaskCanvases`. `internal/canvas/service.go` delegates
   this to `CleanupTask`, which removes task-scoped canvas metadata/instances,
   clears promoted origins, and publishes canvas events. Failure currently
   aborts the task mutation. This prevents orphaning an active canvas release.
3. `persistTaskResourceCleanup`, creating a prepared durable cleanup job with
   resource handles and snapshot. Failure aborts row deletion.

Only then does the lifecycle call repository deletion. After commit it removes
attachment bytes and dependency edges, publishes `TaskDeleted`, pulls on a
vacated workflow step, clears activity, and activates prepared cleanup.
Activation failure is logged while durable recovery remains responsible.
Stop, worktree/remote-directory reclamation, and environment cleanup run through
the existing worker. A claim or prepared job alone is not a deleted task.

`sqlite.Repository.DeleteTaskWithVacatedStep` begins `hierarchyTxOptions`
(READ COMMITTED on PG), locks hierarchy and task step, checks current task and
children, checks recovery authority, captures and purges prompt/queue policies,
deletes the task, commits, and then notifies queue purge. Its lock, hierarchy,
session capture, queue/prompt purge, and notification work must be reused.
Replacing this path with a standalone guarded SQL `DELETE` loses those rules.

`taskCleanupBarrierLocked` rejects session/environment creation during existing
prepared/pending/running/retry-wait/waiting-for-clean jobs. SQLite uses native
writer serialization; PG locks the task row. `CreateTaskResourceCleanupJob`
currently has recovery-claim exclusion but no managed identity/revision/detach
predicate. `recoveryclaim.EnsureTaskAvailableTx` checks environment recovery
authority, not exact managed deletion authority. Neither is an existing
managed delete claim by implication.

## Selected boundary and compatibility decision

A committed exclusive authorization/exclusion boundary precedes
environment or canvas effects. Physical task/session removal remains a separate
commit. A PREPARED barrier is reversible, non-runnable, and never represents a
deleted task. Before admission commits, rejection is clean. After admission,
baseline environment/canvas preparation may partially succeed even when final
deletion fails or is cancelled. Task/session/transcript rows survive an own
failed final deletion; canvas authority/events and ownership transfer are not
promised rollback beyond actual baseline compensation.

Keep the current environment-before-canvas-before-final-delete ordering and
canvas-failure abort behavior. Reopening retained ownership after an admitted
failure does not undo prior effects. A final-commit-first path, postcommit
canvas cleanup engine, schema/protocol change, and general lifecycle rewrite
are explicitly excluded. Authorization admission is distinct from irreversible
physical deletion; preparation or a cleanup job does not establish removal.

## Required typed contracts and durable representation

The typed internal contracts are:

- `managed.DeleteRequest`: `Identity`, expected revision, task creation-time
  incarnation, exact operation ID and payload digest. `DeleteManaged` retains
  its existing outward signature and passes this request to mandatory
  `Service.DeleteManagedConversationTask`. `SetTaskDeleter(taskSvc)` registers
  this typed capability alongside legacy deletion. A legacy-only/nil deleter
  fails Unavailable for exact deletion; legacy helpers keep their existing
  behavior. No bare repository fallback for this operation.
- `managed.DeleteClaim`: cleanup job ID, operation ID, invocation owner token,
  retained identity/incarnation/revision, and persisted phase. The owner token
  is a fresh unguessable per-invocation identifier, not the Host idempotency key.
  An independent retry knowing the operation ID is not the live owner.
- Required native methods `AdmitManagedDeletion`, `InspectManagedDeletion`,
  `FinalizeManagedDeletion`, and `ReleaseManagedDeletion` on the managed
  repository contract. Admission/finalization return current typed rows/outcome
  and claim evidence; release is owner/state CAS. No supplied business callback.
- A typed `managed_delete` envelope in the existing cleanup job JSON snapshot:
  version, Host operation/digest, owner token, identity, creation-time incarnation,
  admitted revision, phase (`reserved`, `prepared`, `deleted`), and existing
  resource inventory. It is internal snapshot data, not a database migration
  or new public wire contract. A deterministic cleanup operation ID derived
  from the exact operation distinguishes replay from another cleanup attempt.
  Never reuse a bare task-ID cleanup ID for this authority.

Extend `taskResourceCleanupSnapshot` with the optional typed envelope and preserve
it through every decode/re-encode, claimed inventory update, retry and resource
snapshot repair. Existing generic snapshot writers must not strip the owner or
deleted marker. The managed-only predicate uses native dialect JSON handling
and locked cleanup rows; no parallel metadata authority or side table.

Cancelled job reuse requires fresh native current identity/revision/hierarchy
validation and a new owner token in the same transaction as cancelled-to-prepared
CAS. The existing attempts counter remains the cleanup-worker attempt counter;
do not repurpose it as deletion-owner generation. Exact owner token plus row ID,
operation/digest, state and prior snapshot are the fence. Direct calls with no
operation pair retain one-shot compatibility by generating an internal unique
pair; do not add a new caller validation solely because the old service ignored
those parameters. A supplied exact pair enables replay reconciliation.

## Phase 1: Native exclusive admission and rollback

`DeleteManaged` discovery is advisory. Under required native admission, acquire
the same workspace/hierarchy -> task-step -> task order used by native deletion,
then lock/check current children, recovery authority and active cleanup rows.
PG uses `hierarchyTxOptions` READ COMMITTED and current task reads after waiting;
SQLite factory transactions acquire their writer at BEGIN before predicate
reads, as selected below; existing hierarchy checks remain inside the Tx. Current `managed.Matches`, task incarnation, detached state,
`managed.Revision`, hierarchy deletion eligibility and cleanup/recovery authority
are all validated before inserting or restoring the exact PREPARED owner row.
Commit this reservation before any environment/canvas/stop/byte/worktree effect.
It is a current native predicate plus committed exclusion, not a preflight
reread. Missing/foreign/detached/replaced returns `managed.ErrNotFound`, stale
returns `managed.ErrRevision`, and contention/hierarchy failures use distinct
pre-admission conflict classification. Storage/cancellation is unavailable or
the existing context error; no effects and no active barrier on proven rollback.

Admission commit errors are reconciled against the exact owned row under a
bounded cancellation-independent context. If committed ownership is proven,
classify as admitted. If absence/rollback is proven, classify pre-admission.
An unreadable/uncertain outcome is unavailable and retains any possible barrier;
never retry insertion blindly or start effects from an uncertain claim.

Parent/child validation shares actual native deletion logic. Child creation and
reparenting into a task with this active admission must respect the owner barrier
under the same hierarchy/task ordering. Finalization rechecks hierarchy too.
Do not invent a new stronger hierarchy policy or cascade descendants.

## Phase 2: Real lifecycle preparation outside SQL locks

The typed Service path authorizes scope and uses the existing archive deadline,
then calls factored real `deleteTaskWithReasonAndDBDelete` preparation and
postcommit logic with the admitted claim. Inventory is captured after the
barrier, using real sessions, runtime, worktree/SSH, environment and attachments;
dirty-worktree consent and inventory failures still abort. No SQL transaction
spans inventory, external stop, canvas removal or worktree I/O.

Carry the explicit internal owner claim through narrowly typed native helpers
for `TransferTaskEnvironmentOwnership` and cleanup snapshot persistence. The
owner may bypass only its own managed admission barrier, never foreign cleanup
or recovery authority. `recoveryclaim.WithTaskCleanupJob` alone is insufficient:
its existing marker does not validate this prepared owner token. The environment
transfer keeps its current owner/generation/borrower checks and independent
commit. Canvas cleanup remains the real `CleanupTaskCanvases`; no new canvas
postcommit adapter or engine is added. A canvas error aborts final deletion.

Reuse the admitted job instead of creating a second prepared cleanup job late
in the helper. Persist its complete existing resource snapshot and `prepared`
phase with owner/state CAS, preserving the managed envelope. A reserved/empty
inventory can never be activated. Any error after admission is an admitted
failure, even if no physical effect happened in that particular run. Return
Unavailable, reconcile release, and never emit task.deleted or run destruction
for an uncommitted final delete. Partial environment/canvas effects remain
honestly possible. Do not map a later hierarchy or owner mismatch to a clean
pre-admission conflict.

## Phase 3: Guarded final native deletion and commit evidence

`FinalizeManagedDeletion` uses the real native `DeleteTaskWithVacatedStep`
transaction logic, factored into a controlled native helper. Under hierarchy ->
step -> task -> owned cleanup -> captured session/resource locks, validate the
same current retained incarnation/revision/detach and PREPARED claim/owner token,
complete prepared inventory, hierarchy and recovery authority. Reuse native
prompt-sequence/queue-policy purge, task/session removal, vacated-step result,
and queue notification after commit. There is no bare guarded SQL alternative.

In the SAME transaction as removal, owner-CAS the existing snapshot phase to
`deleted`. This marker supplies actual commit evidence even when acknowledgement
fails or a new incarnation subsequently uses the same task ID. It cannot commit
if deletion/purge rolls back. Reject a missing/replaced current task during an
uncommitted finalization as an admitted failure/uncertainty; absence does not
authorize removing a replacement or activating an empty old snapshot.

After confirmed removal, run existing attachment/dependency cleanup, task.deleted
publication, vacated-step handling, activity clearing and prepared cleanup
activation once through their current lifecycle path. Marker recognition on
retry does not re-run task deletion or task.deleted publication. Existing
non-transactional event delivery is not upgraded to exactly-once delivery by
this design. Activation uses the owned complete snapshot and existing
PREPARED-to-PENDING CAS; worker claim/attempt handling remains unchanged.

## Phase 4: Owner release, cancellation, replay and restart

Actual `CancelTaskResourceCleanupJobIfPending` permits four states by job ID;
`CompleteTaskResourceCleanupJob` is unfenced; restoration checks cancelled state
and attempts. These methods alone do not prove this exclusive owner. Add narrow
managed-specific owner/state/operation CAS inside the native cleanup repository,
using `detachedCleanupTransitionContext`'s existing bounded WithoutCancel policy.
Never release/restore/overwrite a foreign, pending, running or newer owner row.

| Proven state | Required reconciliation |
| --- | --- |
| Same admitted task and exact owner, final deletion rolled back/not attempted | Owner CAS PREPARED to CANCELLED; retain task/primary/transcript. Prior preparation effects are not undone by cancellation. Fresh update/detach may win after release. |
| Same operation CANCELLED and current original incarnation/revision | A new invocation may acquire fresh exclusive admission with a new owner token and full predicate validation. It may not blindly restore the earlier snapshot. |
| Same operation PREPARED, foreign invocation owner | Return admitted/ownership Unavailable. Operation ID equality does not authorize stealing or duplicating preparation. Original owner may continue its own invocation. |
| Persisted `deleted` with complete owned snapshot | Deletion is proven for the old incarnation; activate/finalize through existing CAS/worker durability. Same-operation replay reports proven already committed and never deletes a replacement. |
| Task missing/replaced but no `deleted` marker, unreadable owner/commit, or inconsistent snapshot | Retain safe non-runnable barrier and return Unavailable/uncertain. Do not infer successful removal from absence; do not touch replacement. |

Existing `preparedTaskCleanupMutationCommitted` infers deletion from task absence;
`reconcilePreparedTaskResourceCleanupJobs` cancels aged pre-startup jobs or
mutation-unknown jobs when a task remains. These inferences remain baseline for
ordinary jobs. For snapshots marked managed deletion, dispatch to the required
native inspector/reconciler above. Startup cutoff is NOT proof that a foreign
owner is dead. An independent restarting Service must not cancel its live
prepared claim. A retained PREPARED task with unproven owner liveness remains
Unavailable rather than automatically reclaimed by age. This is an explicit
fail-closed restart limitation, not a new lease/heartbeat promise. Normal owner
failure/shutdown releases after its attempt settles; a proven deleted marker
allows recovery without owner takeover. Any later request to force-release an
unproven crashed owner requires its own reviewed recovery authority; it is not
invented here. No schema or generic engine is required for this conservative
policy. If implementation cannot fence these selected transitions in existing
snapshot/native structures, checkpoint the exact missing seam before expansion.

Commit-then-error uses `InspectManagedDeletion` rather than generic absence-based
`resolveTaskResourceCleanupAfterMutationError` for managed snapshots. A proven
rollback releases only the owner; a proven deletion retains/activates the exact
job; unknown keeps the barrier. Never mark admitted failure successful merely
because a compensation action or cleanup row transition succeeded.

## Participating writers and adapter ownership

Own the following native boundaries in this work order. Guards recognize the
persisted exclusive owner envelope; incoming task metadata never supplies or
grants a bypass token. No universal guarantee is added for legacy SQL writers
or arbitrary full snapshots outside an active exclusive admission.

- `EnsureManagedConversation`, including creation, replay, unchanged config,
  changed config, and missing-primary repair. Barrier checks today occur in
  primary repair/update, not uniformly before replay or task metadata writes.
  Add active-owner exclusion after the canonical task lock and before all
  replay/no-change/state/repair/config writes. Typed current conflict is returned
  without overwriting or repairing the disposing incarnation.
- `ChangeManagedConversationState`: `PauseExact`, `PauseInstallation`,
  `Invalidate`, `Detach`. It calls `beginManagedAdmission` but does not currently
  reject an active task-cleanup barrier before changing managed state. Add
  the same managed exclusive-owner check for all four intents, including detach.
  A lifecycle losing this admission cannot proceed to its execution stopper.
- `managed_conversation_admission.go`: `patchManagedTaskTx` overlays current
  metadata; `repairManagedPrimaryTx` and `updateManagedPrimaryTx` honor cleanup.
- `UpdateTask`, `UpdateTaskPreservingDeferredLaunch`, and
  `UpdateTaskExactOperation`/shared task-update primitives can replace metadata.
  At `updateTaskTx`/necessary metadata replacement primitive, a current retained
  row under live managed deletion rejects full-row authority replacement,
  including forged or omitted incoming managed keys. Outside the active barrier,
  preserve documented full-replace/incoming metadata semantics; do not claim
  to make arbitrary legacy snapshots revision-aware. Narrow unrelated column
  updates need no new global version framework.
- Native task deletion, ordinary cleanup-job creation/restoration/cancellation,
  session creation/promotion, `UpsertExecutorRunning`, environment creation and
  ownership transfer, attachment lifecycle, and task/workspace cascades must
  not steal a live owned reservation or activate its uncommitted cleanup.
  `CreateTaskSession`, `SetSessionPrimary` and promotion share the task barrier.
  `UpdateTaskSession` and its current-state guarded launch variant, state writers
  into STARTING/RUNNING, execution/environment-binding admission, and
  `UpsertExecutorRunning` acquire task before session and reject a foreign
  prepared managed owner before accepting a new runtime identity. Terminal
  observations of an already captured execution need no blanket suppression;
  cleanup stop authority continues through existing owned job validation.
- `TransferTaskEnvironmentOwnership` gains an explicit typed owner-aware internal
  variant for admitted lifecycle preparation. It validates the exact prepared
  row/token as well as existing owner/generation/recovery predicates; its public
  unowned form continues to reject active cleanup. No bypass from arbitrary
  context strings or task metadata. Attachment inventory uses the existing
  lifecycle lock and snapshot; existing claimed worker/resource adapters remain
  the stop/worktree/Docker/SSH/Kubernetes effect owners.
- Ordinary lifecycle deletion of a retained managed row must also reserve
  exclusive cleanup authority before its first effect, using the same native
  owner fence with an explicit ordinary-delete kind and normal task-write
  authorization, rather than plugin expected-revision authorization. Scope this
  early reservation/owner check to retained managed tasks; other ordinary tasks
  and legacy uninstalls retain baseline. This prevents an ordinary lifecycle
  call from passing an early read, racing managed admission, then producing
  unowned canvas/environment effects. Shared native `DeleteTaskWithVacatedStep`
  rejects a foreign active owner and accepts only the validated owner path.
  Workspace cleanup reserves every initially listed task before canvas
  preparation; an existing foreign owner rejects there, and a generic prepared
  reservation excludes later managed admission. Native final cascade separately
  checks every current task, including late-created tasks. Native reparent/child
  admission respects the same fence; no new cascade policy is introduced.
- Managed-specific snapshot update/start/release/restore/inspection in
  `sqlite/resource_cleanup.go`, service outcome resolution and prepared-job
  restart dispatch in `resource_cleanup_jobs.go`, typed Service wiring in
  `backendapp/services.go`, and real Host error recognition in
  `plugins/host_managed_conversations.go` are owned here. Existing generic
  complete/cancel/restore methods must dispatch or reject marked managed rows
  so their unfenced SQL cannot steal the owner through a legacy call.

Parent/hierarchy conflict is checked before admission effects and again at
finalization; descendants are never implicitly cascade-deleted. For unsupported
native adapters return Unavailable before admission, never optional fallback.

## Workspace cascade scope and known limitation

Workspace cleanup reserves each initially listed task before canvas preparation.
An existing managed owner rejects there, and a generic reservation first
excludes managed admission. Final native cascade checks current tasks again,
preserving any foreign owner's workspace/task/primary/transcript and PREPARED
claim without a task-deleted event.

A late-created task can win managed admission after the initial inventory but
before `CleanupWorkspaceCanvases`. Its final cascade rejection preserves rows
and owner authority after actual canvas removal. This is a known partial-effects
limitation. Baseline workspace cleanup already prepared canvases before final
cascade/name/secret validation and completed late-task inventory after commit.
Requirement .8 preserves that workspace boundary; .4's effect-free pre-admission
rejection governs exact/ordinary retained-task deletion, not every workspace
operation. Workspace-wide protection would need admission before inventory and
canvas that also excludes task/managed creation until release; existing per-task
claims cannot cover unknown future tasks. That separate improvement is deferred.
Keep existing canvas-failure abort, avoid SQL locks over external I/O, and do not
infer that final cascade rejection rolls back earlier canvas effects.

## Registered Host and receipt path

`host_exact.go` registers `DeleteManagedAgentConversationExact` under
`host.v2.write:managed_agent_conversations`. `pluginHostManagedConversationManager`
in `host_managed_conversations.go` derives installation identity, locks current
approval authority, checks manifest and workspace grant, and calls real
`state.CommandStore.Admit`. Production `internal/plugins/provider.go`
`initializeRequiredStores` creates `state.NewCommandStore(dbPool)` and wires it
through `Service.SetExactCommandStore`; `exactCommandStoreDep`/`hostForPlugin`
provide the registered ledger dependency.
Tests must use this producer and a real SQL CommandStore, not merely SDK
round-trip fakes or a fake business service.

SDK `grpcManagedAgentConversationManager.Delete` and
`grpcHostServer.DeleteManagedAgentConversationExact` in `pkg/pluginsdk/host.go`
transport expected revision and identities through existing protocol fields.
No generation/protocol change is required to reproduce the gap. Typed Aborted
or FailedPrecondition maps to `CommandConflict`; NotFound to `CommandNotFound`;
other storage/transport failures to `CommandUnavailable`. The Host completes
durable receipts for non-unavailable outcomes; a completed replay bypasses
business deletion. Current incomplete replay blanket-converts NotFound to
`CommandAlreadyApplied`; that inference needs a narrow causal correction here.

Add a concrete service admitted-deletion error exposing a narrow typed error
classification to the registered Host, while implementing gRPC Unavailable and
wrapping the actual cause for direct callers/logs. Classifications are
`admitted_failed` and `outcome_uncertain`; Host maps them BEFORE generic code
mapping to `CommandUnavailable`, reasons
`managed_conversation_delete_admitted_failed` and
`managed_conversation_delete_outcome_uncertain`. Keep the receipt incomplete
for retry/reconciliation, just as current unavailable errors do; never complete
it as Applied/Conflict/NotFound based on a post-admission failure. Existing
protobuf CommandResult reason and gRPC error text suffice; no wire fields.

`InspectManagedDeletion` precedes fresh task discovery for a supplied exact
operation. A prior admitted operation retains admitted classification even
when a later read is missing/detached/stale: a new error from its retry must not
erase partial-effects history. A persisted `deleted` phase produces a typed
proven-committed replay result (with existing NotFound direct-API classification
where the conversation is gone); only this marker allows the Host to complete
an incomplete receipt as AlreadyApplied. Plain missing/detached identity on
an incomplete receipt is NotFound when no admitted record exists, or Unavailable
when an admitted outcome is unresolved. It never proves this operation deleted
anything. An ordinary fresh deletion still returns Applied after actual commit.
Payload mismatch remains CommandConflict before business effects. Receipt
acknowledgement failure after proven commit keeps `command_receipt_unavailable`;
retry uses the durable deletion marker, bypasses physical deletion/preparation,
and can complete AlreadyApplied even if a replacement task now exists.

## Compatibility and verification boundaries

Ordinary `Service.DeleteTask`, dirty-worktree consent, parent/child conflicts,
borrowed environments, cleanup job startup/restart, queue/prompt notifications,
and legacy `Delete`/`DeleteAllForPlugin` behavior remain regression gates.
Provider cleanup continues through the existing lifecycle/service abstractions
for local/worktree, Docker, SSH, and Kubernetes resource handles; no provider
cleanup implementation is replaced. Native backend support is SQLite and PG16;
unsupported/missing typed admission is unavailable before effects.

The [single work order](../../../plans/managed-deletion-admission/task-01-guard-managed-deletion.md)
defines exact compatibility tests, independent-pool
winner-ordering proof, rejected side-effect observations, actual Host receipts,
and physical PG/SQLite waiting evidence. No local browser/E2E/build is planned.
Pure-data mobile audit: no frontend or rendered interaction changes. The public
managed-conversation reference documents revision/detach rejection before
admission, admitted partial preparation effects and Unavailable, operation-proven
replay without replacement deletion, and the workspace canvas limitation.
Uncertain owner/commit remains fail-closed; a different command key is not a
recovery shortcut. Root README, screenshot catalog and localization need no
changes. Backend-development guidance explains the writer/reader distinction
and bounded native cancellation latency.

## SQLite writer factory boundary

The shared `db.OpenSQLite` writer factory uses supported `_txlock=immediate`
as the accepted writer-entry boundary. See the [accepted DB ADR](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md)
for caller/transaction audit, alternatives and cancellation ownership. The
existing Platform persistence design links the same invariant; no duplicate
incident requirement or second order.

Genuine database/sql/sqlx transactions acquire the writer at BEGIN; current
identity/revision/cleanup predicates, owner CAS and deleted marker stay in that
Tx. Keep `OpenSQLiteReader` deferred/read-only and PG READ COMMITTED/lock order.
Preserve the hierarchy helper; no row-touch substitute supplies waiting.
Arbitrary injected deferred pools can fail safely BUSY/Unavailable before
effects, without fallback. Writer read-only transactions also reserve at BEGIN;
real snapshots continue on the dedicated reader. No new external IO under SQL.

Entry failure has no Tx to roll back and no owner/effects. Context cancellation
while a holder remains locked may settle at busy-timeout return, not instantly.
A returned genuine Tx near cancellation must check context before predicates,
settle rollback and join before reuse. Tests must prove no accepted admission,
no leaked connection, failed-entry/rollback reuse and real reader progress.
Commit uncertainty follows the same owner/marker reconciliation, never absence.

Acceptance requires permanent factory RED and actual-factory physical BEGIN
waiting/current-after-wait, bounded cancellation while the holder remains locked,
pool reuse and separate reader progress. Diagnostic collector success alone is
not acceptance. Native guards, owner conformance, lifecycle/Host outcomes and
affected compatibility retain their distinct verification boundaries; exact
receipts and delivery results belong in the linked order and external task plan.

Public-doc audit identified `docs/public/backend-development.md`'s persistence
explanation: concise writer-BEGIN/deferred-reader and bounded cancellation
guidance accompanies the focused plugin reference. No page/navigation/operator
setting, README, screenshot, mobile or locale change.
