---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-EXTERNAL-ID-001
  - REQ-TASKS-EXTERNAL-ID-SCENARIOS-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-001
  - REQ-TASKS-DETACHED-WORKSPACE-CONTINUITY-002
created: 2026-09-06
updated: 2026-09-06
owners:
  - cfl
---
# Task Creation Protocol

## Authority

This document owns task creation admission, private handles, durable required
steps, Complete/Abort/recovery, and workspace-delete interaction. [Archive
Cascade Execution Contracts](archive-cascade-execution-contracts.md) owns global
lock order. [External Task ID Idempotency](external-id-idempotency.md) owns
external-ID response and release behavior.

## Begin and visibility

Creation is an aggregate protocol, not an insert plus best-effort publication.
`CreateTask`, `CreateTaskIfWorkflowStepHasCapacity`,
`CreateTaskWithWorkflowStepAdmission`, private SQL insert helpers, and all service
callers move to `archivecascade.Store.BeginTaskCreation`. No repository method
inserts `tasks`.

`BeginTaskCreation` returns
`BeginTaskCreationResult{Task, Outcome, Handle *CreationHandle}`. `Handle` is
non-nil only for `Created` and is private outside the service:
`CreationHandle{OperationID, TaskID, WorkspaceID, CompletionToken,
ActorBindingHash, ManifestHash}`. REST/MCP/internal callers retain that same
value through required work and pass it to Complete or Abort; it is never
serialized or reconstructed from task fields.

`task_creation_operations` retains every handle field, manifest hash, state
`preparing|preparing-release|completing|completed|aborted`, recovery action
claim, generation/lease, result/event identity, error, and timestamps. Begin
locks every referenced workspace identity at tier 1, including the complete
read-only candidate scope when the command spans workspaces. It then locks
workflow/admission rows at tier 2, discovers and locks active archive membership
barriers plus `(operation_id, scope_workspace_id)` rows at tier 4 in sorted
operation/scope order, and restarts from tier 1 when the scope digest changes.
It inserts/locks the creation operation at tier 4, then external-ID and
task/parent/blocker keys at tier 5, retained identity at tier 6, and group/member
rows later. It reloads the membership fence before inserting
live task with `creation_state=preparing`, task/revision and task-repository rows,
plan steps. It includes a `workspace_attachment` step exactly when policy selects
a group, membership, or inherited/shared environment binding; no-policy plans
have no attachment step. Begin never inserts those effect rows outside the ledger.
Retained identity conflict returns `task_identity_reused`; the transaction rolls
back.

`preparing-release` is a durable, transient release phase, not a lifecycle
state. A release claim atomically changes `preparing -> preparing-release` only
when no step is `unknown|running` and no compensation is pending; it stores its
release operation/claim generation. Its commit clears the external-ID claim and
changes `preparing-release -> preparing` with `release_applied=true`, without a
task revision or event. Recovery retries that same release operation; workspace
deletion treats either preparing state identically and aborts without an event.
Complete may start only from `preparing`, and therefore emits
`CreatedIdentityLost` when a prior release cleared the ID. CAS and replay never
observe or manufacture a completed holder during this phase.

### Canonical bindings

`CreationPlan` is RFC 8785 JSON with `version:1`, canonical task/workspace/
workflow/parent UUIDs, validated-create-request SHA-256, sorted
`database_rows[{table,key,row_sha256}]`, and sorted
`steps[{kind,step_key,request}]`. UUIDs are lowercase RFC 4122; absent optionals
are JSON null; hashes are 64 lowercase hex; lists sort by their canonical key;
unknown/extra fields are invalid. Each per-kind request below is the complete
`request` object, not an extensible property bag.

`ManifestHash` is lowercase hex
`SHA-256(ASCII("kandev/task-creation-plan/v1") || 0x00 || RFC8785(plan))`.
The `ActorBindingDocument` has exactly `version=1`, sealed actor `variant`,
canonical `principal_id` and `workspace_id`, `auth_mode`, authority `kind`,
authority `id`, unsigned `authority_revision`, and nullable unsigned
`lease_generation`; absent nullable fields are JSON null. Variant/kind pairs use
the sealed actor registry and reject extras. `ActorBindingHash` uses this
document with domain
`kandev/task-creation-actor/v1`. `CompletionToken` is 32 random bytes; only its
SHA-256 is stored. Handle validation recomputes all three bindings after locking
the operation, workspace, and actor authority. No database-native JSON
serialization or caller-supplied hash is accepted.

Only exact external-ID lookup may expose `preparing|completing` as
FoundUnsettled. Lists and lifecycle commands exclude or reject either.

Begin receives a complete `CreationPlan` and inserts one retained
`task_creation_steps` row per canonical `(operation_id, kind, step_key)`. Closed
kinds are `workspace_attachment`, `attachment_claim`, `fresh_branch_prepare`,
`remote_contribution_associate`, `session_prepare`, and `launch_intent`.
`workspace_attachment` is required whenever policy resolution selects a group,
membership, or inherited/shared environment binding. REST, WebSocket, MCP,
plugin, workflow, and internal child routes all construct that same step.
Every route plans `workspace_attachment` when policy selects it, then its
route-specific steps; MCP plans remote/session/launch as applicable. MCP and
other creators use the same resolver for policy/default selection.

The manifest cardinality is closed. `CreationPlan.start_policy` is `none`,
`immediate`, or `deferred`, resolved from `start_agent` and exactly one
canonical dependency admission: the create request's `blocked_by` list is
empty. A non-empty `blocked_by` always selects `deferred`, regardless of
whether listed predecessors are already resolved; creation never evaluates
predecessor state retroactively. false `start_agent` selects `none`; true
`start_agent` selects `immediate` iff dependency admission passes, otherwise
`deferred`. `immediate` has exactly one `session_prepare` and one
`launch_intent`; `none` and `deferred` have neither. `deferred` records exactly
one start-when-unblocked intent with the resolved launch metadata and promotes
only from the later `dependencies_resolved` transition of the last unresolved
predecessor (deletion never fires it), atomically materializing exactly one
session/launch pair under the same task/admission fence. WIP occupancy never
changes the created policy: it is a launch-time admission gate at the
auto-start chokepoint that decides whether an unblocked immediate launch runs
now or waits for queue promotion. The other counts are zero or one
`workspace_attachment` by policy, one `attachment_claim` per nonempty
attachment set, one `fresh_branch_prepare` per selected repository, and one
`remote_contribution_associate` per requested provider contribution.
`inherit_parent` selects the attachment when the parent binding is resolved;
`shared_group` selects it for a group/member or explicit shared environment;
`new_workspace` selects it only for an explicit group/environment binding; an
unbound policy selects none. `blocker_ids` are part of the attachment request,
so sequential blocker attachment has durable step evidence and compensation.

Provider PR/MR linking in `github_task_prs`/`gitlab_task_mrs` is a separate optional
post-Complete projection named `provider_pr_association`; it is not a required
creation step, does not affect `creation_complete`, and is retried by the provider
integration after `task.created`. Feeder pull and last-used recording are likewise
best effort after Complete.

Each step stores request hash, deterministic idempotency key, state, claim
owner/generation/lease, attempt, canonical evidence bytes/hash, provider handle/
ownership marker, compensation state/evidence, error, and timestamps. The key is
SHA-256 over operation, kind, canonical step key, and request hash. Provider
calls receive it when supported; local effects persist an operation/step
ownership marker before mutation.

### Closed step schemas and proof

`task_creation_steps` is the sole step-state/claim owner for every kind. It
stores canonical `request_json`, `request_sha256`, `evidence_json`,
`evidence_sha256`, `provider_handle`, `ownership_marker`, nullable
`cleanup_job_id`, and closed `disposition`; database/provider tables named below
are effect authorities, not competing state machines. `request_sha256` uses
domain `kandev/task-creation-step-request/v1`. Evidence is the closed object
`{"version":1,"kind","step_key","request_sha256","observation","observed_at","effect"}`
where effect is `present_exact|absent_proven|conflict|transferred`; observation
has the per-kind schema below. All bytes use RFC 8785 and all referenced rows are
reloaded under their global lock tier.

| Kind | Complete request object | Effect authority and success evidence | Unknown reconciliation | Compensation or transfer |
| --- | --- | --- | --- | --- |
| `workspace_attachment` | `{"task_id","workspace_id","parent_id":null|string,"mode":"inherit_parent|shared_group|new_workspace","group_id":null|string,"member_ids":[],"blocker_ids":[],"environment_id":null|string,"repository_ids":[]}` with canonical sorted IDs | locked group/member, `task_blockers`, environment, and environment-repository rows; evidence repeats every row hash, policy, owner, generation, membership role, and blocker edge | exact rows and policy -> success; all rows absent with unchanged plan -> retry; mixed, foreign, or changed rows -> unknown | delete operation-owned untouched rows; otherwise transfer the complete attachment proof to durable cleanup |
| `attachment_claim` | `{"task_id","workspace_id","attachments":[{"attachment_id","staging_owner_id","content_sha256","size"}]}` sorted by attachment ID | locked `task_message_attachments`; evidence repeats every ID, prior/current owner, content hash/size, and row hash and proves all are claimed by this operation/task | exact claimed rows -> success; all rows still exact staged ownership -> retry; missing/mixed/foreign/content-changed -> conflict/unknown | one transaction restores exact staged owners when unchanged, otherwise creates `task_resource_cleanup_jobs` with the full attachment proof |
| `fresh_branch_prepare` | `{"task_id","workspace_id","repository_id","task_environment_id","task_environment_repo_id","resource_key","canonical_path_sha256","branch","base_commit"}` | operation 4; task 5; retained identity 6; step 7; `task_environments`/`task_environment_repos` 11; locked Git registration and marker 14; evidence is the complete local/Git proof document | exact path/registration/branch/HEAD/base/marker -> success; authenticated marker plus Git registration proving prior removal -> absent/retry; generic not-found or mismatch -> unknown | fenced Git removal/quarantine with exact marker proof, or atomic transfer to `task_resource_cleanup_jobs` |
| `remote_contribution_associate` | `{"provider":"github|gitlab","workspace_id","repository_id","contribution_kind":"pull_request|merge_request","contribution_id","connection_id","task_id"}` | required effect authority is `task_remote_contribution_associations(provider,connection_id,contribution_id,task_id,workspace_id,repository_id,operation_id,idempotency_key,ownership_marker,claimed_at)` locked at tier 15; evidence binds every column, provider response/delivery handle, and row hash | authenticated provider read plus exact locked association -> success; authoritative absence with unchanged request -> retry; credential loss, handle mismatch, or conflicting association -> unknown | delete only the operation-owned association by idempotency key, or transfer its exact cleanup proof before Abort; `github_task_prs`/`gitlab_task_mrs` are never required |
| `session_prepare` | `{"task_id","workspace_id","session_id","task_environment_id","executor_id","executor_profile_id","agent_profile_id","launch_config_sha256","resource_keys":[]}` with sorted keys | `task_sessions`, `task_environments`, `task_environment_repos`, and absence of an unaccounted `executors_running`; evidence hashes complete prepared rows and each resource marker | byte-exact prepared rows/no unexpected runtime -> success; all rows/resources proven absent -> retry; partial rows, runtime, or marker mismatch -> unknown | transactionally delete untouched prepared rows; every possibly started/runtime/resource effect transfers to task/resource cleanup jobs |
| `launch_intent` | `{"task_id","workspace_id","session_id","agent_profile_id","executor_id","executor_profile_id","launch_config_sha256"}` | retained step row is the sole intent; evidence proves exact intent and no dispatch | exact intent -> state `armed` with evidence effect `present_exact`; absent result -> retry; pre-Complete runtime -> unknown and cleanup transfer | workspace-delete completion/Abort cancels an armed intent with `workspace_deleted` and `absent_proven`; observed runtime transfers to cleanup |

`step_key` is kind-specific and canonical: workspace-policy hash for
`workspace_attachment`, attachment-set hash, environment-repo UUID,
`provider:connection:contribution-kind:contribution-id`, session UUID, or
session UUID for launch intent. The workspace attachment claim locks workspace,
operation/barrier, task/revision, then environment/repository and group/member
effect rows in global order; Complete owns the `task.created` outbox.
Duplicate kind/key or two steps claiming the same effect authority is invalid at Begin.
Providers that lack native idempotency are called only after an operation-owned
association/marker row is committed and reconcile by that immutable handle.

`state` is exactly
`planned|armed|running|retry_wait|unknown|failed|succeeded|cancelled`; `armed`
is legal only for `launch_intent` and means the pre-Complete intent row is
complete. `cancelled` is legal only for `launch_intent` during workspace-delete
completion or workspace-delete Abort. Its closed `disposition` is
`none|workspace_deleted`; cancellation requires `armed`, that disposition,
`absent_proven` evidence, and the same operation/lifecycle generation. Only a
matching lease/generation may leave running states. `failed` requires
`absent_proven`; conflict remains unknown until proof or transfer. A transfer
commits the target cleanup job, its full canonical proof, and `cleanup_job_id`
in the same transaction.

`PreLaunchArmPredicate` requires every non-launch step `succeeded`, no
compensation pending, and an exact un-dispatched launch intent; it does not
require Complete or an armed state. `ArmLaunchIntent` changes the matching
`running` intent to `armed`. The separate `CompletePredicate` requires every
non-launch step `succeeded` plus an included intent in `armed` state; with launch
omitted (`start_agent=false`) no launch row may exist.
Workspace-delete Complete permits `cancelled/workspace_deleted` only for an
`armed` launch intent, with exact `absent_proven` evidence. Workspace-delete
Abort may make the same transition for an armed intent, then requires all other
step effects `absent_proven`, `compensated`, or `transferred`. Normal Abort
never cancels an armed intent. All Complete paths require exact evidence/request
hashes and byte-equivalent effect rows.
`RecoveryAction` is a closed enum:
`reconcile_unknown_step|retry_compensation|resume_preparing|complete_preparing|
retry_preparing_release|complete_completing|abort_preparing|reconcile_launch_dispatch|
retry_launch_dispatch|transfer_launch_dispatch|replay_terminal`. Exactly one
action is legal per locked condition; all else return typed conflict with zero
writes:

| Locked condition | Legal action |
| --- | --- |
| operation `preparing`, any step `unknown` | `reconcile_unknown_step` |
| operation `preparing`, no unknown/running, compensation pending/retry_wait | `retry_compensation` |
| operation `preparing`, no unknown/running, no pending compensation, zero permanent failed steps, resumable planned/retryable step exists, Complete predicate false | `resume_preparing` |
| operation `preparing`, all non-launch steps succeeded, no pending compensation, and launch is omitted or armed | `complete_preparing` |
| operation `preparing-release`, release unfinished, no unknown/running | `retry_preparing_release` |
| operation `completing`, phase-B result not committed | `complete_completing` |
| operation `preparing`, launch intent not armed, no unknown/running/pending compensation, at least one permanent failed step, and every effect absent/compensated/transferred | `abort_preparing` |
| workspace deletion owns an unknown/running dispatch claim | `transfer_launch_dispatch` |
| operation `completed`, dispatch claim unknown, no deletion fence, with matching provider/runtime proof | `reconcile_launch_dispatch` |
| operation `completed`, dispatch claim unknown, no deletion fence, with authenticated no-dispatch proof | `retry_launch_dispatch` |
| operation `completed|aborted`, no earlier recovery condition | `replay_terminal` (read-only) |
| otherwise | no action permitted |

The `abort_preparing` predicate explicitly excludes an armed launch intent.
The coordinator also rejects any permanent-failure transition after arming under
the operation lock; only workspace-delete Abort may disarm an armed intent with
exact `absent_proven` evidence. An invariant violation returns a typed
`creation_state_conflict` with zero writes rather than silently cancelling or
launching.

`complete_preparing` claims the operation, commits phase A to `completing`, then
reacquires the same lock set and runs phase B. `complete_completing` claims an
unfinished phase-B operation and reruns that finalizer idempotently; an already
committed result is replayed. Recovery claims persist the action, operation
generation, actor binding hash, owner, and lease. Workspace-deletion ownership
is evaluated before dispatch proof, and proof predicates exclude a deletion
fence; thus a transferred claim cannot become succeeded or retryable. Predicates
are evaluated under one operation lock, so the rows are mutually exclusive;
stale generation or lease returns a typed conflict with zero writes.
`replay_terminal` cannot mutate.


Handlers perform no untracked effect. They call
`CreationCoordinator.Execute(handle)`, which uses only:

| Store API | Contract |
| --- | --- |
| `ClaimTaskCreationStep(handle, stepKey, actor)` | `planned|retry_wait -> running`, increment generation, lease one step |
| `ArmLaunchIntent(claim, evidence)` | matching `running` launch step to `armed` only when `PreLaunchArmPredicate` holds; evidence proves exact intent and no dispatch |
| `CompleteTaskCreationStep(claim, evidence)` | matching claim to `succeeded`; evidence proves exact requested effect |
| `FailTaskCreationStep(claim, failure)` | no-effect permanent failure to `failed`; transient to `retry_wait`; ambiguity to `unknown` |
| `ResolveTaskCreationStep(actor, stepKey, proof)` | `unknown` to `succeeded|retry_wait|failed` only from provider/marker proof |
| `Claim/CompleteTaskCreationCompensation(...)` | fence `required|retry_wait -> running`, then record `compensated|transferred|unknown|retry_wait` with closed proof |
| `CompleteTaskCreation(handle)` | only when every plan row satisfies the exact Complete predicate above |
| `RecoverTaskCreation(operationID, actor, action)` | recovery-only operation/lease path; revalidates retained hashes and generation, then resumes steps, compensation, Complete, or eligible Abort without a caller handle |
| `AbortTaskCreation(handle, cause)` | only before completing and when the exact Abort predicate above holds |

`running` with expired lease becomes `unknown`, never assumed failed. Step or
compensation `unknown` blocks Complete and Abort. Runtime uses the kind-specific
reconciliation matrix, retries idempotent steps, and completes compensation.
A request may synchronously drive the coordinator, but its error or cancellation
never deletes or settles directly. Existing rollback deletes become handle-bound
Abort; ineligible Abort returns pending/recovery and leaves Runtime owner.

## Complete, Abort, and recovery

`CompleteTaskCreation(handle)` has two durable transactions. Phase A acquires
workspace 1, creation operation 4, task 5, retained identity 6, and all
step/effect rows 7-15; it verifies the handle/actor/manifest and evidence, then
commits `preparing -> completing`. Phase B reacquires the same lock set plus
outbox 16, requires `completing`, revalidates all hashes/proof, and calls
`completeTaskCreationLocked(tx, verifiedCreation, workspaceDeleteMode)`. The
locked finalizer never starts a transaction or reacquires rows; it marks the
task/operation complete, allocates revision one, and inserts `task.created`.
Normal Complete commits phase B. Workspace deletion uses phase B plus revision
two `task.deleted`, its dependency, and live-row deletion in one transaction.
An armed launch intent is cancelled there with `workspace_deleted`; an armed
intent in the workspace-delete Abort path is cancelled with the same exact
evidence. Same-handle replay returns the stored outcome/event; Runtime recovers
an unfinished `completing` phase.

`AbortTaskCreation(handle,cause)` accepts only ledger-eligible `preparing`. One
transaction creates/fences every cleanup transfer, deletes partial live rows,
retains operation/steps/identity as aborted, and emits no lifecycle event.

Runtime leases expired operations/steps. `OperationRecoveryActor` does not
reconstruct a caller `CreationHandle`: it invokes privileged
`RecoverTaskCreation(operationID, actor, action)`, whose trusted actor is derived
from the retained operation claim and recovery lease. It reloads the stored
manifest/actor hashes and completion-token hash, resumes/reconciles/compensates
preparing work, and retries Complete only for `completing`; callers receive no
handle. Complete after manual ID release emits creation without ID and returns
`CreatedIdentityLost`. Dispatch starts only after the locked
`task_creation_operations` row is `completed`, revision one and its
`task.created` outbox evidence are committed, the task lifecycle is
non-deleted, and the armed launch intent matches that lifecycle generation.
`SettleExternalID`, `finalizeCreatedTask` publication, and lifecycle-delete
rollback are removed.
`LaunchTask` uses retained `task_launch_dispatch_claims`, unique by
`(creation_operation_id, task_id, session_id)`. The row stores
`planned|running|unknown|retry_wait|succeeded|cancelled|transferred`, claim
generation/owner/lease, lifecycle generation, idempotency key
`task-launch/<creation-operation-id>/<session-id>`, request hash, runtime
handle, and evidence. `planned|retry_wait -> running`; matching provider/runtime
proof makes `unknown -> succeeded|cancelled`; authenticated no-dispatch proof
makes it `retry_wait`; workspace deletion makes unstarted `cancelled` or
ambiguous/in-flight `transferred` with delete ownership. Only matching
non-deleted `succeeded` completes launch. Runtime owns these recovery actions;
stale claims cannot replay or perform I/O.

Workspace deletion is the sole lifecycle exception. Its single Store transaction
locks workspace 1, deletion/creation/release operations 4, sorted external/task
keys 5, retained revisions 6, all creation step/effect rows 7-15,
launch-dispatch claims 8, and outbox rows 16. It invokes
`completeTaskCreationLocked` without nested locking or public re-entry, cancels
armed launch intents and unstarted dispatch claims or transfers ambiguous claims,
terminalizes owned `preparing-release` rows as `workspace_deleted`, inserts
created then deleted revisions and the workspace-event dependency, deletes live
rows, and commits all state atomically. Preparing or `preparing-release`
transfers effects and aborts with no event. Crash replay uses stored operation IDs
and exactly the same lock set/failpoints.

## Verification ownership

Work order 01 implements schema, Store APIs, coordinator, and caller cutover.
Work order 03 implements Runtime reconciliation/compensation and workspace-delete
interaction. Work order 04 proves production composition and both dialects.
Same-database failpoints cover each kind independently: exact request/effect
schema, present/absent/conflict reconciliation, every lease/state transition,
idempotency-key reuse, compensation versus cleanup transfer, and recovery after
each external/DB boundary. Tests mutate every CreationPlan, actor document,
manifest, request, evidence, provider handle, ownership marker, and cleanup-job
field; mismatches cannot Complete or Abort. They also cover completing/commit
ambiguity, takeover, workspace deletion in both states, exact events, and every
old insert/rollback bypass.
