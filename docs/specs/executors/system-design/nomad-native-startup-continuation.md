---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-NOMAD-002
  - REQ-EXECUTORS-NOMAD-003
  - REQ-EXECUTORS-NOMAD-004
---

# Native Nomad startup continuation

## Boundary and mapping

This supports the [native executor design](nomad-native-executor.md) and its
[job identity contract](nomad-native-job-identity.md). It continues unfinished
creation of an owned environment; initialized ACP conversation recovery remains
on the existing authenticated recovery path.

| Requirement | Contract |
| --- | --- |
| REQ-EXECUTORS-NOMAD-002 | Retain original intent and continue under its execution, generation and operation fences |
| REQ-EXECUTORS-NOMAD-003 | Unknown allocation ownership survives reconciliation and blocks replacement; repaired fixtures are cleanup-only |
| REQ-EXECUTORS-NOMAD-004 | Reveal original saved credentials and attach the original runtime without bootstrap replay |

## Retention before reconciliation

An executor row with runtime Nomad can own a registered job while its task
session is still Created and its SQL status is starting. Missing auth token,
resume token or ACP initialization does not prove absence. In particular,
`orchestrator.Service.reconcileOneSessionOnStartup` must not send this row
through the Created-session prune branch in `service.go`, nor through
`pruneOrRepairExecutorRow`'s conversation-resumability decision in
`reconcile_liveness.go`. Preserve the complete row and its secret references,
task/session state and initial-delivery ownership. Do not relabel the unresolved
runtime dead/stopped to make a conversation repair rule apply.

Retain malformed/missing typed Nomad evidence as unknown, too. It cannot
authorize automatic continuation, deletion or fresh creation. Only the reviewed
positive cleanup/absence path can relinquish allocation ownership. Runtime
retention is separate from `models.RowMustBePreserved`'s generic conversational
resume policy; other runtimes keep their existing reconciliation behavior.

`PrepareStartupRecovery` already snapshots Nomad inventory and queues exact
pending owners behind `RecoveryGuard`. Establish that ownership before startup
reconciliation or request handlers can replace an execution. A failed inventory
read remains an admission failure, not permission to allocate a new UUID.

## Post-readiness pending-operation branch

The current `NomadExecutor.RecoverInstances` requires a ready row with saved
auth token. Generic `processRecoveredRemoteInstance` then proves an initialized
agent session and marks the reconstructed execution initialized. Neither is the
path for a Created session whose ACP has not initialized.

After startup readiness, classify exact pending Nomad owners separately before
generic remote attach. Reuse existing pending-attempt serialization
(`beginRemoteRecovery`, `attemptDone`, stop-request ownership) and the
session-keyed execution singleflight; do not add a second worker inventory or
unbounded startup loop. The operation must:

1. Re-read the canonical task/session/environment and exact current inventory
   execution, environment generation, operation, revision and cleanup claim.
   Require the original Created session, unchanged admitted inputs and no
   archive, deletion, terminal state, active cleanup or stop claim. Failed,
   Completed or Cancelled sessions cannot enter this initial-creation branch.
2. Resolve canonical authorized `WorkspaceInfo`, retaining the original
   `AgentExecutionID`. Reuse `prepareExecutionCreateRequest` and the immutable
   workspace/input/job-policy checks before calling the retained-operation
   runtime path. Reveal only the original saved nonce/token references through
   the manager secret store. Never discover private bootstrap files during
   automatic recovery or generate replacement credentials.
3. Continue through the existing workspace-only creation/registration pipeline
   (`createExecutionWithMode(..., false)`), with narrowly bound pending-owner
   admission where necessary. Reconcile the original job and full allocation,
   worker start fence and authenticated ServerIdentity. Do not register, restage,
   launch another controller or fall back to a fresh UUID when evidence fails.
   GET the exact original agentctl instance; create that instance only on
   authenticated definite not-found and reconcile ambiguous create responses.
   An attach failure must not delete a preexisting instance.
4. Revalidate ownership and stop/cleanup admission before and after runtime
   contact and before registration/publication. Publish one workspace-only
   execution under the original identity, with ACP initialization false. Keep
   the task/session Created state; do not set Running, synthetic ACP/resume
   identity or the generic initialized-session recovery-success marker.
5. Complete only this matching pending entry after successful publication and
   final admission. Until then retain its guard; an unknown/error outcome
   closes only newly opened host transports and retains durable ownership.
   Never globally release the guard first to call ordinary Launch. Concurrent
   stop, terminalization or successor generation wins by the existing exact
   operation/claim checks and cannot be resurrected by a late continuation.

Phase intent/registered/allocated is the initial case. Also handle a second
crash after runtime phase ready but before first ACP initialization: ready
describes agentctl/instance readiness, not an initialized conversation. For
this case require authenticated exact existing-instance/no-ACP evidence and
attach only; ready metadata alone cannot authorize another instance or first
prompt. If an ACP session already exists, use initialized recovery and its
delivery-evidence rules; never initialize it again under this branch.

Existing `ensureWorkspaceExecutionLocked` resolves canonical workspace inputs
and publishes workspace-only infrastructure. Existing `Launch` refuses a held
recovery guard, so the internal continuation must claim the exact pending owner
and preserve guard coverage rather than use a public caller-supplied bypass.
The ownership transfer is local to this operation and cannot admit unrelated
workspace commands or agent launches while recovery remains unresolved.

## Original workspace input versus resolved runtime path

`nomadLaunchDigest` in `executor_nomad.go` hashes both the original
`ExecutorCreateRequest.WorkspacePath` as `workspace_input` and the independently
derived environment path as `workspace_path`. They have different meanings. A
fresh new-workspace session may have an empty original input; the immutable
runtime path is nevertheless nonempty. `projectCreatingNomadWorkspacePath` in
`task/service/service_turns.go` correctly exposes the retained runtime path to
workspace admission and matching. That projection must not substitute the
resolved path for the original input when recomputing the launch digest.

Record two comparable scalar fields in the existing typed Nomad state:
`WorkspaceInputPath` (string) and `WorkspaceInputCaptured` (bool). A captured
empty string is valid and distinct from an old record with no capture. Persist
the exact original request value in the first intent checkpoint before staging
or registration. No database schema or pointer field is required; state remains
value-comparable. Once captured, ordinary metadata checkpoints cannot change
either field. `WorkspaceInputCaptured == false` with a nonempty path is invalid;
false with empty is a legacy missing-evidence record, never a captured empty.
Fresh dispatch requires true. Reject conflicting evidence.

Use one shared resolution rule in manager and executor validation, and pass the
resolved original input explicitly to digest construction. Runtime requests,
created-instance matching, workspace admission and job policy continue using
the immutable environment `WorkspacePath`. Do not temporarily clear or rewrite
the live request's runtime path to make a digest pass, and do not change the
saved InputDigest, job metadata, job ID or expected job-spec SHA. The digest's
existing wire shape remains unchanged; only its original input source becomes
explicit. All other canonical digest fields still match the original operation.

`MetadataKeyOriginalWorkspacePath` is not a substitute: its documented meaning
is the first agent-visible CWD, `executor_backend.go` falls back to the runtime
path, and `persistence.go` omits it when empty. It cannot distinguish an empty
original digest input from missing historical evidence. Session worktrees and
`RepositorySnapshot["path"]` are existing durable source inputs used by
`GetWorkspaceInfoForSession` before environment/runtime projection; the current
projected path is not historical input evidence.

For a pre-release intent without the new fields, permit explicit operator
repair only when existing durable original session/task/environment
snapshots identify a unique original request value for this exact operation.
In particular, the observed fixture can resolve empty only with its original
blank session worktree/repository-path and task/environment workspace snapshots,
the explicit new-workspace binding, unchanged owner/generation/profile and no
conflicting inherited/shared/prepared path or override. A currently empty field
or new-workspace mode alone is insufficient. Reconstruct that single evidenced
value and require the complete canonical digest to equal the retained
InputDigest exactly, alongside the existing full job/allocation/fence checks.
Do not search candidate paths or try empty merely because the projected path
failed hashing. A private offline probe is diagnostic evidence, not production
recovery authority by itself.

After successful reconstruction, a dedicated operator checkpoint may enrich
the original operation with the captured input and redacted repair provenance
before diagnostic continuation. The only permitted change to input evidence is
legacy false/empty to true/the uniquely evidenced value, conditional on exact
current row identity, owner, execution, revision, generation, operation and no
stop/cleanup/successor claim. Preserve the digest/job/artifact identities. Do not
loosen normal repository CAS, whole-state equality or
`sameNomadPreACPOperation` to ignore the new fields or permit arbitrary input
rotation. The operator path is separately identified and narrowly checked; a
second repair or change of a captured input is rejected.

Perform this fixture repair only with the backend and automatic dispatch
stopped, retaining worker quarantine/fence. Then use a fresh ordinary startup
to load the enriched row and canonical WorkspaceInfo and establish its pending
record/guard normally. Live pending-entry replacement or snapshot re-pinning is
forbidden: do not add an operator-specific recovery API or ignore a comparison
failure to make an old in-memory capture=false entry match a changed row.
Normal strict equality and revision checks remain unchanged. Revalidate
owner/admission/digest/policy at startup and continuation; a changed stop,
cleanup or successor claim prevents continuation. A process that still holds
the old revision cannot perform an authorized checkpoint.

Never label an ambiguous record as captured. Missing provenance, conflicting
sources or digest mismatch leaves the operation unknown and cleanup-only; no
new operation or in-place job/digest replacement is allowed. An input-repaired
fixture is diagnostic/operator evidence and cannot satisfy automatic WO05
recovery proof, even if later continuation succeeds. Run a fresh operation with
input captured before dispatch for that proof. Subsequent operations must
always capture the input rather than rely on historical resolution.

Test both empty and nonempty captured inputs, absent versus captured-empty,
resolved-path provider projection, conflicting or changed snapshot/input,
legacy unique reconstruction with exact hash, and ambiguous legacy rejection.
Verify runtime/matcher/job paths remain resolved and unchanged in every case.

## Initial delivery remains independently owned

Infrastructure continuation sends no prompt and does not prove that a prior
prompt was delivered. `schedulingOrphanRecoverableSession` deliberately leaves
Created sessions alone; `NotifyQueuedUserPrompt` starts a Created session when
a new durable queue notification arrives, but is not a startup rescan.
Therefore retention alone, or putting intent into generic recovery, cannot
complete this startup path.

After runtime publication, notify/resume only the original durable initial
delivery owner through its existing admission and delivery claim. For an
already persisted queue entry, use the existing Created-session start/queue
drain semantics, without recording another user message. A workflow entry,
deferred launch or delivery submission must retain its own exact accepted
entry/payload binding and guards; do not replace it with a new automatic start.
First ACP initialization then occurs through the normal created-session start
path. The dispatch claimant rechecks terminal/cleanup/current-generation state
at dispatch time. The initialized conversation path settles uncertain prior
deliveries through existing evidence rather than replaying them.

If no durable initial-dispatch owner/payload exists, leave the retained Created
session available for explicit authorized start and report recovery pending or
action required. Do not infer a prompt from task description, latest message,
an open turn or process existence. Automatic restart of the exact original
prompt requires its existing durable delivery identity; an in-memory accepted
request alone is insufficient. The real restart proof must identify which
durable owner is resumed and demonstrate one initialization and one delivery.

## Cleanup-only disposition of a pruned test fixture

A test run whose original row was already deleted by the faulty startup path
is not clean automatic recovery evidence. An explicit operator can use the
captured pre-crash row plus the surviving original secret-store reference to
recover ownership for cleanup only. This is a fixture procedure, not production
automatic inventory/credential discovery:

1. Stop/guard automatic startup and dispatch for the fixture, retain its worker
   fence, and establish the dedicated-worker quarantine under exact ownership.
   Verify captured row identity and original nonce reference against the same
   database/secret store, unchanged canonical owner/environment generation and
   admitted profile. Re-read the actual full job policy, creation/definition
   indexes, exact allocation/NodeID and worker fence. Do not trust a same-name
   job, captured snapshot or SQL absence alone.
2. Restore inventory only with a transaction/conditional write that still sees
   no current row and unchanged canonical owner/generation. Never overwrite a
   concurrent row or successor. Before it becomes visible to recovery, bind an
   exact cleanup claim and cleanup-pending/quarantined disposition; retain the
   original intent snapshot and redacted operator-repair provenance. Do not
   expose a restored auto-startable intent, invent a ready/initialized state,
   or modify the original job, bootstrap paths or credential value.
3. Use the original saved nonce only for the authenticated handshake to the
   exact original leader; immediately persist any returned original auth token
   reference. Do not create an instance or start ACP. Obtain the real generic
   seal-and-drain receipt, including the valid zero-instance case, and durably
   bind it to the exact ServerIdentity/resource/generation/cleanup claim.
4. Perform the reviewed owned whole-job stop, observe exact live-worker
   allocation/process absence within the supported managed-process scope,
   and release the exact worker fence/quarantine only after that evidence.
   Retain the repaired record/tombstone and identify this as operator cleanup
   in evidence. The unfinished WO07 destructive-stop path must not be claimed
   implemented merely because this operator procedure is available.

If any ownership, secret, handshake, policy, drain or live-worker absence check
fails, preserve the surviving allocation/fence and quarantine. Use externally
verified owned cleanup or controlled worker reboot before releasing capacity;
terminal/missing Nomad history alone still does not prove process absence.
After fixing retention and continuation, use a fresh operation for the clean
crash/restart proof. Never count the reconstructed fixture toward that proof.

## Required verification

Cover Created intent retention without nonce consumption, no auth/resume
token, malformed evidence retention, unchanged metadata/CAS revision and
unaffected non-Nomad reconciliation. Restart at post-register/pre-response,
pre-instance, ambiguous instance-create, and ready/pre-ACP barriers. Prove the
same execution/job/allocation/fence/workspace, no extra registration or staging,
no new nonce, exactly one instance and initially false ACP initialization.
Race continuation with cancellation/cleanup, terminal session change and
successor generation; none can create or initialize after losing authority.
Verify queue/delivery binding and no prompt duplication separately from runtime
attachment. Missing durable delivery evidence must leave Created rather than
manufacturing dispatch. The real fixture must kill the actual backend process,
restart the same database/secret store and retain all original evidence without
operator repair; report repaired cleanup runs separately.
