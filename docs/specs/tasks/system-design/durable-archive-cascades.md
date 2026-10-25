---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-EXTERNAL-ID-001
  - REQ-TASKS-EXTERNAL-ID-SCENARIOS-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002
created: 2026-09-05
updated: 2026-09-05
owners:
  - cfl
---
# Durable Archive Cascades

## Purpose and boundaries

This design defines restart-safe archive/unarchive, membership, authorization,
parent admission, and recovery. Runtime/worktree teardown is owned by
[Task Runtime Cleanup](runtime-cleanup.md).

ADR: [Persist task cascade mutation progress](../../../decisions/2026-09-05-durable-task-cascade-mutations.md).

Refs: [Archive Cascade Execution Contracts](archive-cascade-execution-contracts.md) and [Archive Cascade Boundary Contracts](archive-cascade-boundary-contracts.md).

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| `AC-TASKS-RUNTIME-CLEANUP-001.6` | Operation ledger, archive, unarchive, cleanup identity |
| `AC-TASKS-RUNTIME-CLEANUP-001.10` | Archive caller context and recovery |
| `AC-TASKS-RUNTIME-CLEANUP-001.11` | Membership admission, security, bounded traversal, read failures |
| `AC-TASKS-RUNTIME-CLEANUP-001.12` | Partial archive and unarchive recovery |
| `AC-TASKS-RUNTIME-CLEANUP-001.13` | Workspace boundary |
| `AC-TASKS-RUNTIME-CLEANUP-001.14` | Task and workspace-group restoration |
| `AC-TASKS-RUNTIME-CLEANUP-001.15` | Scheduled auto-archive |
| `AC-TASKS-RUNTIME-CLEANUP-001.16` | Legacy conversion |
| `AC-TASKS-RUNTIME-CLEANUP-001.17` | Transactional event outbox |
| `AC-TASKS-RUNTIME-CLEANUP-001.18` | Aggregate transaction boundary |
| `AC-TASKS-RUNTIME-CLEANUP-001.19` | Cleanup worker leases and fences |
| `AC-TASKS-RUNTIME-CLEANUP-001.20` | Group workspace authorization |
| `AC-TASKS-RUNTIME-CLEANUP-001.21` | Archive/unarchive phase CAS |
| `AC-TASKS-RUNTIME-CLEANUP-001.22` | Blocked-candidate barriers |
| `AC-TASKS-RUNTIME-CLEANUP-001.23` | Session/runtime/environment admission |
| `AC-TASKS-RUNTIME-CLEANUP-001.24` | Durable event revision allocation |
| `AC-TASKS-RUNTIME-CLEANUP-001.25` | Retained evidence and FK matrix |
| `AC-TASKS-RUNTIME-CLEANUP-001.26` | Unknown cleanup reconciliation |
| `AC-TASKS-RUNTIME-CLEANUP-001.27` | Canonical resource keys and markers |
| `AC-TASKS-RUNTIME-CLEANUP-001.28` | Legacy operator wire/CLI contract |
| `AC-TASKS-RUNTIME-CLEANUP-001.29` | Typed actor derivation and authorization |
| `AC-TASKS-RUNTIME-CLEANUP-001.30` | Sole outbox/event-bus authority |
| `AC-TASKS-RUNTIME-CLEANUP-001.31` | Public CLI contract validation |
| `AC-TASKS-RUNTIME-CLEANUP-001.32` | Cascade cleanup retry policy |
| `AC-TASKS-RUNTIME-CLEANUP-001.33` | Pre-authentication security audit |
| `AC-TASKS-RUNTIME-CLEANUP-001.34` | Legacy conversion proof matrix |
| `AC-TASKS-RUNTIME-CLEANUP-001.35` | Complete database lock order |
| `AC-TASKS-RUNTIME-CLEANUP-001.36` | Revision migration and workspace tombstones |
| `AC-TASKS-RUNTIME-CLEANUP-001.37` | Repeated archive and missing-group outcomes |
| `AC-TASKS-RUNTIME-CLEANUP-001.38` | Aggregate-owned task creation and identity |
| `AC-TASKS-RUNTIME-CLEANUP-001.39` | Historical revision gaps and watermarks |
| `AC-TASKS-RUNTIME-CLEANUP-001.40` | Complete durable workspace deletion |
| `AC-TASKS-RUNTIME-CLEANUP-001.41` | Workspace deletion supersession |
| `AC-TASKS-RUNTIME-CLEANUP-001.42` | Absolute request deadline |
| `AC-TASKS-RUNTIME-CLEANUP-001.43` | Complete authenticated group snapshot |
| `AC-TASKS-RUNTIME-CLEANUP-001.44` | Authenticated legacy cleanup binding |
| `AC-TASKS-RUNTIME-CLEANUP-001.45` | Trusted filesystem and Git proof |
| `AC-TASKS-RUNTIME-CLEANUP-001.46` | Public-wrapper and publisher cutover |
| `AC-TASKS-RUNTIME-CLEANUP-001.47` | Concrete sealed actor bindings |
| `AC-TASKS-RUNTIME-CLEANUP-001.48` | Mandatory recovery runtime lifecycle |
| `AC-TASKS-RUNTIME-CLEANUP-001.49` | Locked workspace confirmation |
| `AC-TASKS-RUNTIME-CLEANUP-001.50` | Fenced workspace config identity |
| `AC-TASKS-RUNTIME-CLEANUP-001.51` | Blocker/comment deletion ownership |
| `AC-TASKS-RUNTIME-CLEANUP-001.52` | Production Runtime startup and quiescence |
| `AC-TASKS-RUNTIME-CLEANUP-001.53` | Typed deletion reason cutover |
| `AC-TASKS-RUNTIME-CLEANUP-001.54` | Opaque creation handle and recovery |
| `AC-TASKS-RUNTIME-CLEANUP-001.55` | Exact workspace table registry |
| `AC-TASKS-RUNTIME-CLEANUP-001.56` | Office confirmation propagation |
| `AC-TASKS-RUNTIME-CLEANUP-001.57` | Workspace deletion versus unsettled creation |
| `AC-TASKS-EXTERNAL-ID-001.1` | External-ID create/lookup aggregate |
| `AC-TASKS-EXTERNAL-ID-001.2` | Found-unsettled representation |
| `AC-TASKS-EXTERNAL-ID-001.3` | Found no-side-effect path |
| `AC-TASKS-EXTERNAL-ID-001.4` | Lookup-first REST/MCP/Office/agentctl preflight |
| `AC-TASKS-EXTERNAL-ID-001.5` | Opaque creation handle |
| `AC-TASKS-EXTERNAL-ID-001.6` | Completed release revision/outbox |
| `AC-TASKS-EXTERNAL-ID-001.7` | Preparing release event ordering |
| `AC-TASKS-EXTERNAL-ID-001.8` | Internal creation recovery |

## Components

`HandoffService` authorizes roots; `archivecascade.Store` owns the
handle/CreationPlan/step ledger, cascade/revision/outbox writes, retained
release replay, and canonical-provenance deletion on one dialect-aware DB.
Task, Office, integration, workspaceconfig, and workspace services delegate.
Production startup gates all post-initial-agent/agentctl starters on Runtime;
leased generation-fenced executors and Runtime own recovery, never optional
maintenance or best-effort events.

## Scope discovery and reservation

Read-only discovery returns sorted task IDs, workspace IDs, and a digest. Any
foreign workspace returns `cross_workspace_descendant` before reserve or effects.
An accepted scope is `{root_workspace}`. Reserve locks that singleton identity
at tier 1, re-enumerates under the lock, and rejects newly foreign workspaces
before writing operation, scope, membership, or lifecycle rows.
Discovery traverses with a bounded depth counter and collects at most
`max_cascade_members+1` IDs using a fixed-capacity buffer; it rejects
`archive_size_exceeded` on the sentinel before sorting or digesting. A streaming
hash covers the canonical traversal order, and only the validated bounded set is
sorted/materialized for reservation. No database count or caller value controls
allocation.


## Operation ledger
- IDs/root/workspace/mode; phase
  `preparing|preparing-release|archiving|archived|unarchiving|restore_pending|
  foreign_scope_blocked|completed|foreign_scope_resolved|workspace_deleted|
  retry_wait|aborted`; membership counts/timestamps, retry kind/schedule/error,
  sorted scope, generation, lease owner/expiry/heartbeat, and timestamps.
`task_archive_operation_scopes` is keyed by `(operation_id, scope_workspace_id)`,
uses `ON DELETE RESTRICT`, tier 4, and stores generation/phase/lease. Reserve
writes generation-zero scopes; membership commit increments generations; writers
lock/reload and restart on change. Rows persist terminal results, including
`workspace_deleted`; `archived` stays open until unarchive or deletion.

Terminal results use `ArchiveOperationResultV1` on the operation row:
`result_version`, operation/root/workspace IDs, phase/direction, sorted member
and skipped IDs, per-member dispositions, lifecycle event IDs, and
`(task_id,epoch,revision,queue_sequence)` references. `result_sha256` is
`SHA-256(ASCII("kandev/task-archive-result/v1") || 0x00 || RFC8785(payload))`.
Terminal payload/digest are immutable; same operation/request hash replays exact
bytes and IDs without reading a later holder, while hash mismatch or missing
result blocks replay.
At most one open operation exists per root. Open phases are
`preparing|preparing-release|archiving|unarchiving|restore_pending|foreign_scope_blocked|retry_wait|archived`
with qualified retry kinds; the partial unique constraint covers exactly them.
`completed|aborted|foreign_scope_resolved|workspace_deleted` are replayable
terminals; deletion rejects restore and stores delete-owned evidence. Only
zero-commit may abort; execution errors and ambiguous legacy evidence retry or
remain diagnostic.

`task_archive_operation_members` stores immutable membership:

- operation, task, workspace, depth, and deterministic order;
- archive and unarchive progress;
- the matching resource-cleanup operation ID;
- confirmed-missing disposition where deletion won before membership mutation.

`task_archive_operation_groups` stores versioned immutable full group/member
snapshots, canonical hash, identities, ownership/cleanup generations, and restore
progress; reconstruction is defined by the [boundary contract](archive-cascade-boundary-contracts.md#complete-group-reconstruction-snapshot).
`task_group_cleanup_jobs` and `task_cleanup_jobs` store operation/resource
identity, immutable snapshot/key hash, direction/state, retry, generations,
lease, error, and unknown evidence; cascade jobs never terminalize.
`task_archive_resource_markers` is per-resource CAS authority.
`task_lifecycle_revisions` and `task_lifecycle_event_outbox` retain task
identity/order and immutable pending/publishing/retry/delivered events.
Exact names, retention, FKs, migration, and delete-before-cleanup rules are
authoritative in the [workspace deletion table registry](workspace-deletion-table-registry.md)
for both dialects. No retained evidence cascades to mutable resource rows.

## Aggregate transaction boundary

Production package `internal/task/archivecascade` owns `Store`, including
`Store.DetachTask`, the sole public detachment transaction owner. Its constructor
requires the shared `*sqlx.DB` and dialect; no constructor accepts task/Office
repository interfaces. Commands `BeginTaskCreation`, handle-bound
`CompleteTaskCreation`/`AbortTaskCreation`, `ReleaseTaskExternalID`, `Reserve`,
`CommitMembership`, `CommitArchiveMember`, `CommitUnarchiveMember`,
`CreateOrAttachGroup`, `SetTaskParent`, `DetachTask`, typed-reason `DeleteTask`,
`DeleteExpiredTask`, `DeleteEphemeralTasksByAgentProfile`, and `DeleteWorkspace`
own transactions and
mutate required task/Office/integration rows in execution-contract lock order.
Existing repository mutators become private dialect transaction helpers callable
only by this Store; service and Office callers cannot bypass admission.
`SetTaskParent` is the narrow non-empty-parent command. Its complete command,
result, expected-generation, sorted-lock, no-op, replay-ledger, and
`task.updated` envelope contract is authoritative in
[Detached Workspace Continuity](detached-workspace-continuity.md#atomic-detachment).
Empty parent uses `DetachTask`; callers cannot compose either command with
stewardship or environment mutations.

`DeleteWorkspace` replaces task and Office deletion loops. It rechecks confirmation
and actor after locking, deletes the complete registry, supersedes cascades,
fences config claims, and persists cleanup steps plus ordered outboxes. Provider
and filesystem callbacks run outside SQL. Exact inventory and aftermath are in
the [boundary contract](archive-cascade-boundary-contracts.md#complete-workspace-deletion-transaction).

## Lock and admission order

All commands, admitted writers, workers, dispatchers, and operator routes use
the complete tiered order in
[Archive Cascade Execution Contracts](archive-cascade-execution-contracts.md#complete-database-lock-order).
Physical guards are acquired outside SQL transactions.

The checked-in writer inventory is the implementation cutover checklist:

| Existing production writer | Tables relevant to admission | Aggregate command |
| --- | --- | --- |
| all task inserts/helpers/finalizers/settlers/rollback/surfaces | creation operation, typed steps/idempotency/evidence/compensation, handle/plan, revisions/outbox | `BeginTaskCreation`, step Store APIs, coordinator, `CompleteTaskCreation`, `AbortTaskCreation` |
| repository/service external-ID settle/release/direct publisher | release operation/key/result, ID clearing, revision/outbox | Complete or aggregate `ReleaseTaskExternalID` |
| task `UpdateTask`, `UpdateTaskIfWorkflowMatches`, all `UpdateTaskWithWorkflowStepAdmission*`, `UpdateTaskIfWorkflowStepHasCapacity` | `tasks.parent_id` | `SetTaskParent` or non-parent narrow update |
| Office `CreateWorkspaceGroup`, `AddWorkspaceGroupMember` | group and member rows | `CreateOrAttachGroup` |
| task `CreateTaskSessionWithSharedGroupWorkspaceBinding` | group owner, environment/session | `BindSharedWorkspace` |
| Office `MarkWorkspaceMaterialized`; task `FinalizeTaskEnvironmentMaterialization` | group owner/generation, environment | `FinalizeWorkspaceMaterialization` |
| task `TransferTaskEnvironmentOwnership` | environment ownership/generation | `DetachTask` aggregate |
| Office `ReleaseWorkspaceGroupMember`, `RestoreWorkspaceGroupMemberByCascade` | group members | archive/unarchive member commit |
| task `ArchiveTask`, `ArchiveTaskIfActiveWithVacatedStep`, `UnarchiveTaskByCascade`, `UnarchiveTask` | task archive stamp/state | archive/unarchive member commit; direct methods removed |
| task cleanup `CreateTaskResourceCleanupJob`, both snapshot updates, `MarkTaskResourceCleanupJobRunning`, `StartPreparedTaskResourceCleanupJob`, both completion methods, cancel/reset methods | `task_resource_cleanup_jobs` | cleanup claim commands |
| Office `UpdateWorkspaceGroupCleanupStatus`, `ClaimWorkspaceGroupCleanup`, `CompleteWorkspaceGroupCleanup`, `UpdateWorkspaceGroupRestoreStatus` | group cleanup/restore state | cleanup claim commands |
| task environment create/update/materialization/transition/delete methods | environment ownership/lifecycle | `MutateTaskEnvironment` |
| task `DetachTask`; Office `UpdateTaskParentID` | parent, stewardship, environment/repository ownership | `DetachTask` aggregate |
| task `ReparentDirectChildren` | parent | `SetTaskParent` (narrow) |
| task environment-repo `reconcileTaskEnvironmentReposTx`, `updateTaskEnvironmentRepoTransitionTx`, `tombstoneOmittedTaskEnvironmentReposTx`, `CreateTaskEnvironmentRepo`, `insertTaskEnvironmentRepoTx`, `UpdateTaskEnvironmentRepo`, `DeleteTaskEnvironmentRepo`, `DeleteTaskEnvironmentReposByEnv` | `task_environment_repos` identity/path/lifecycle | `MutateTaskEnvironment` |
| worktree store `CreateWorktree`, `UpdateWorktree`, `DeleteWorktree`; session `UpdateTaskSessionWorktreeBranch`, `UpdateTaskSessionWorktreeBranchByRepository`, `UpdateTaskSessionWorktreeBranchByWorktree` | `task_environment_repos` physical handle | `MutateTaskEnvironment` |
| task session `CreateTaskSession`, `CreateTaskSessionWithInitialRuntimeSeed`, `CreateTaskSessionWithWorkspaceBinding`, `CreateTaskSessionWithSharedGroupWorkspaceBinding`, `CreateOfficeTaskSession`, `ClaimPromptableTaskSessionIfActive`, `UpdateTaskSession`, `UpdateTaskSessionIfCurrentState`, `UpdateTaskSessionIfCurrentStateRemovingMetadataKeys`, `UpdateTaskSessionWithMetadata`, `DeleteTaskSession` | `task_sessions` creation, binding, full-row mutation, deletion, or transition to runnable | `MutateTaskSession` |
| task runtime `UpsertExecutorRunning`, `DeleteExecutorRunningBySessionID`, `DeleteExecutorRunningIfCurrent` | `executors_running` resource identity creation/replacement/removal | `MutateTaskRuntime` |
| Task HTTP/MCP, maintenance, exact provider/reset, automation, plugin, e2e deletes | task/revision/outbox plus canonical trusted provenance | typed commands/constructors; no fallback |
| all creation compensation currently calling delete | creation step/effect state | ledger-eligible handle-bound Abort |
| quick-chat expiry and profile bulk cleanup | selector plus canonical maintenance provenance | `DeleteExpiredTask`/`DeleteEphemeralTasksByAgentProfile` |
| Task/Office workspace handlers, blockers/comments, exact table registry | locked confirm/actor, cascade/creation supersession, outboxes | `DeleteWorkspace` |
| all loader/writer/Git/sync/default/settings/memory/path-event paths | resolver, unconstructable fence, state/access reload, VerifiedConfigRoot | exact workspaceconfig replacement API |

Every other production writer is a column-scoped exemption in the checked-in
registry: session display/profile/route/metadata/usage/base/read/primary/review
fields, terminal session state, runtime status/heartbeat/error/branch, message
accounting, and migration-declared columns only. None may create, bind, resume,
or remove a runtime, environment, physical handle, lifecycle identity, or
publisher. The registry records package/function/table/columns, classification,
invariant, aggregate command, and every public caller; AST/SQL inventory rejects
missing methods/tables, extra columns, wildcard updates, unclassified deletes,
or direct lifecycle publication. The boundary contract defines the only normal
update and publisher exceptions.

Reservation-first returns typed conflict; writer-first state enters the snapshot. Writers discover active operations for each referenced task/workspace, lock scope workspace identities at tier 1 then operation/scope rows at tier 4 in canonical order, and reload generation/scope. Candidate discovery is read-only; locked revalidation restarts from tier 1 on change, otherwise the write is rejected as archive-admission conflict.
Every command accepts a sealed actor constructed from trusted composition, never
request identity fields. User, task-session, auto-archive, operation-recovery,
creation-step-worker, cleanup-worker, outbox-worker, task-maintenance,
legacy-operator, and installation-recovery variants carry exact selector,
timestamp, auth-mode, workspace/task/operation/job/event, generation/owner/lease
fields and transaction checks defined in
[Archive Cascade Boundary Contracts](archive-cascade-boundary-contracts.md#sealed-actor-structures-and-checks).

Every command reloads the binding and authorization under the ordered locks.
Global admin grants no foreign workspace access; synthetic authority expires
when auth mode changes. Mismatch returns typed not-found/conflict with zero side
effects.

The aggregate transaction commits task/group rows, membership marker, and
`archiving` together. Cleanup starts only after commit. Orphaned unmarked
`preparing` has no cleanup jobs.

## Archive flow
1. Authorize/load the root and perform read-only candidate discovery. Reject any
   descendant outside the root workspace with `cross_workspace_descendant`.
   Reserve then commits the singleton root `admission_scope` barrier.
2. CAS-claim an attempt generation and start its renewable lease heartbeat.
3. Snapshot bounded task and group membership and commit the membership marker.
4. Prepare deterministic cleanup jobs for every member. Recovery action
   `resume_pending_cleanup` atomically activates all matching `prepared` rows to
   `pending` before any marker claim; the activation transaction has a failpoint
   and is replay-safe.
5. Detach caller cancellation while retaining `TaskArchiveTimeout`.
6. Stop runtimes, then archive deepest first. Each transaction compare-and-sets
   the task, records member progress, and inserts its event outbox row.
7. Start matching task/group cleanup and release snapshotted memberships with
   operation and ownership-generation compare-and-set.
8. Mark `archived` only after all member/group phases commit, every required
   nonmissing cleanup job is `succeeded`, and no cleanup job remains `prepared`.

Task mutation failure moves the same generation to `retry_wait`; no replacement
cascade is allowed after any member commit. Retry derives truth from member
progress and task stamps, repairing response-loss ambiguity. A lost/expired
lease cancels the local attempt context; later claims increment generation, and
all stale progress/finalization writes fail their fence.

Scheduled auto-archive uses the injected root-only coordinator and the same
deadline, ledger, cleanup, group-release, event, and unarchive rules.

## Archive caller context

The cancellation cutoff, absolute deadline, and compensation budgets in
[Task Runtime Cleanup](runtime-cleanup.md#archive-cascade-coordination) apply
unchanged. Preparation remains caller-cancellable; later work detaches
cancellation without extending that deadline.

## Cleanup identity, leases, and fencing

Task and group jobs carry target/work direction, lifecycle and claim generation,
owner/lease/heartbeat, and expected
operation/cascade/root/task/workspace/group/ownership identity. Unarchive
advances the lifecycle fence before joining a running cleaner; unknown outcome
is reconciled under the physical guard rather than called cancelled. Every
transition is state/generation CAS; false CAS reloads the winner.

`CleanupFence` carries that bound worker claim, target/work direction, lifecycle
generation, snapshot hash, and sorted canonical resource keys. Exact encoding,
namespaces, provider proof, marker states/CAS, absent or prior-operation marker
behavior, cancellation, and multi-key acquisition are owned by
[Archive Cascade Execution Contracts](archive-cascade-execution-contracts.md#resource-keys-markers-and-provider-proof).

Claim commits before guard acquisition. Short marker and completion transactions
bracket synchronous I/O while the physical guard is held; no SQL transaction
waits for it. Restore reuses identical keys under the advanced lifecycle fence.

## Unarchive flow

1. Resolve the durable operation by root/cascade or active unarchive phase.
2. Set target restore and advance each task/group lifecycle fence, then join only
   those exact cleanup claim IDs and reconcile any possibly executed I/O.
3. Restore every snapshotted materialized group and membership with expected
   ownership generation and row-count compare-and-set. Delayed group cleaners
   revalidate the same operation/generation before physical deletion.
4. Restore task metadata with atomic member progress and outbox insertion. This
   is metadata-safe only; it does not perform physical worktree I/O.
5. On the first resumed session, `MaterializeTaskForResume` claims the shared
   `task_resume_materialization_claims` row keyed by task, lifecycle generation,
   environment generation, environment, repository, and worktree. Requesting
   sessions share a succeeded claim but each CAS its own session admission after
   the claim commits; physical I/O follows the claim and only that succeeded
   claim admits a runnable session.
6. If every member, group, ownership, outbox, and required physical
   materialization is ready, complete and return success. Otherwise enter
   `restore_pending`; keep the operation open, mark affected tasks not-ready,
   and return a pending result. Only successful `MaterializeTaskForResume`
   claims plus finalization may transition `restore_pending` to `completed`.
   Pending restoration is never exposed as fully restored or full success.

Retries resume from member progress. A non-not-found task read fails the attempt
and preserves retry state. A confirmed missing task is recorded in the member
row and returned in `SkippedTaskIDs`; it is never silently omitted from a
full-success result.

Legacy manual unarchive cancels only cleanup jobs with the legacy `archive` trigger. Cascade unarchive never performs task-wide archive-cleanup cancellation.

## Operation phase transitions

Every mutation compare-and-sets phase, explicit `retry_kind`, and generation:

| From | Request | To | Barrier |
| --- | --- | --- | --- |
| absent | archive reserve | `preparing`, generation 0 | acquire |
| `preparing` | archive claim | `preparing`, generation 1 | retain |
| `preparing` without marker | expired recovery | `aborted` | release |
| `preparing` with committed membership | archive | `archiving` | retain |
| `preparing|archiving` | repeated archive | same operation/phase, stored `in_progress` | retain |
| retry kind `archive` | repeated archive | same operation/retry result | retain |
| `archiving` | failure | `retry_wait`, kind `archive` | retain |
| `archiving` or retry kind `archive` | unarchive | `unarchiving`, generation + 1 | retain |
| `archiving` | archive complete; groups/tasks and every required nonmissing cleanup job succeeded, confirmed-missing tasks have durable delete-owned transfer | `archived` | retain |
| `archived` | repeated archive | same operation, stored member/result payload | retain |
| `archived` | unarchive | `unarchiving`, generation + 1 | retain |
| `unarchiving` | failure | `retry_wait`, kind `unarchive` | retain |
| `unarchiving` | metadata ready but required materialization absent | `restore_pending` | retain |
| `restore_pending` | all required materialization ready | `completed` | release |
| retry kind `archive|unarchive` | due recovery | direction phase, generation + 1 | retain |
| `unarchiving` or `restore_pending` or retry kind `unarchive` | archive | typed `unarchive_in_progress` conflict | retain |
| `preparing` with committed membership | workspace delete | `workspace_deleted`, retained membership/cleanup proof, no task event, reject restore | release |
| any phase with exact scope `{workspace}` | workspace delete | `workspace_deleted`, delete-owned evidence/dispositions, reject restore | release |
| any active phase with foreign scope | workspace delete | `foreign_scope_blocked`, kind `foreign_scope`; retain foreign members/no external effect | retain |
| `foreign_scope_blocked` | authenticated absent-root resolution | `foreign_scope_resolved`, disposition `foreign_scope_resolved`; foreign members untouched | release |
| `completed` | repeated unarchive | `completed` stored result | released |
| `blocked` diagnostic | adopted proof | proven archive/unarchive phase | retain candidates as operation barrier |
| `completed|aborted|foreign_scope_resolved|workspace_deleted` | new archive | new operation/cascade after terminal barrier release | new barrier |

Lease claim CAS requires expected phase/retry kind/generation and unowned or
expired lease; success increments generation and writes owner/expiry. Renewal
requires owner plus generation and only extends expiry. Retry transition clears
owner/expiry and records kind/schedule. An unexpired claim is reported/reused,
never stolen. Completion/finalization requires owner, phase, and generation.

Member delete also supersedes that member's resource job to fenced
`delete_owned` cleanup; `confirmed_missing` may complete archive only after this
ownership is durable, while unarchive remains blocked for an affected group.
Cleanup continues independently under the exact
[deletion contract](archive-cascade-execution-contracts.md#revision-allocation-deletion-and-event-delivery).
Unarchive from partial archive cancels uncommitted prepared cleanup and reverses
only ledger-committed effects. Delayed archive worker/finalizer fails CAS and
cannot archive, release groups, complete, or alter the barrier.

## Workspace-group consistency

Group restoration is required, not best effort. Every group/task/role is
snapshotted. Unarchive joins matching cleanup, restores with exact ownership
generation/row-count CAS, and blocks dependent tasks on retry/unknown.

A missing group is recreated only from its complete immutable snapshot; ID or
workspace conflict and incomplete proof create `missing_group_snapshot` and
remain blocked. Exact reconstruction and conflict dispositions are in
[Archive Cascade Boundary Contracts](archive-cascade-boundary-contracts.md#complete-group-reconstruction-snapshot).

## Failure and recovery

- Expired unmarked `preparing` CAS-aborts and releases its barrier; marked
  preparation resumes.
- Expired attempts and task/group cleanup claims increment generation on claim.
- Any committed-member execution error remains retryable with its barrier.
- Blocked legacy diagnostics retain candidate barriers and require audited
  resolution; they are never selected as executable operations.
- Cross-workspace, traversal, or storage ambiguity fails before side effects.
- No success is returned while a required nonmissing member/group phase is retryable; a `confirmed_missing` member follows retained delete cleanup independently.

Startup conversion canonicalizes every ID and applies the
[legacy proof matrix](archive-cascade-execution-contracts.md#legacy-conversion-proof-matrix).
Live task rows, committed retained member markers, and versioned cleanup
snapshots bound to those markers may prove a deleted member. Unbound cleanup
rows and group rows only corroborate. Any malformed, conflicting, ambiguous, or
cross-workspace evidence blocks the whole candidate set.

`task_archive_migration_diagnostics` stores ID, raw-evidence hash, reason code,
revision, status, and timestamps. `task_archive_legacy_candidates` stores
diagnostic, canonical candidate task/workspace, evidence hash, and active flag
with unique active barriers. `task_archive_resolution_audit` stores ID, unique
`(actor_id, request_id)`, canonical request hash, diagnostic ID/revision, action,
old/new evidence hashes, outcome, HTTP status, resulting operation ID, and time.

The versioned dedicated-loopback API and native
`archive-cascade-reconcile list|resolve` command use the exact scopes, DTOs,
authentication, raw-peer policy, canonical request hash, replay, status,
redaction, JSON stream, and exit contracts in
[Archive Cascade Execution Contracts](archive-cascade-execution-contracts.md#local-operator-and-security-audit).
Tokens bind actor and filters; every page and replay reauthorizes. Real users
cannot vacuously authorize zero-candidate diagnostics.

`task_archive_resolution_audit` records authorized resolution attempts in the
mutation transaction. Separate `task_archive_security_audit` accepts nullable
pre-auth/body identities and uses the durable emergency fallback contract.
Audit failure never permits a resolution mutation.

## Events and composition

Task/workspace outboxes are sole lifecycle-event authority. Aggregate owns
completed `task.created`, all lifecycle `task.updated` including completed-task
external-ID release, every typed `task.deleted`, and `workspace.deleted`.
Handoff/direct/optional publishers, reasonless/provider fallbacks, repository
identity/deletion writes, and destructive subscribers are removed.

Creation Begin returns an opaque handle and retains revision zero/no event.
Complete durably enters `completing`, then writes revision one/created; Abort is
allowed only before completing and emits nothing. Stable release replay never
reads a later holder; preparing release folds into Created/Abort and completed
release advances revision. Workspace delete rechecks original confirmation/
actor, aborts preparing creation, completes-then-deletes completing creation,
uses exact table/config registries, and orders task before workspace events.
Migration uses explicit absent gaps/watermarks.

Production constructs without starts, binds bootstrap, runs initial-agent/
agentctl prerequisites, then Runtime before every other starter/route/readiness;
transient retry keeps liveness up and failures reverse-clean. Runtime owns
recovery and stop-once quiescence. Exact creation, evidence, config, boundary,
table, startup, and publication contracts are in the linked system-design
documents and generated table inventory.

HTTP, WebSocket, MCP, scheduled, startup, worker, Office, and operator paths
share Store/Runtime. Tests cover wrappers, failpoints, lock,
actor/state, self-cancellation, gaps, deletion, reconstruction, authenticated
evidence, Git proof, aftermath, event ordering, and PostgreSQL deadlock freedom.
