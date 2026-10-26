---
id: "05-recovery"
title: "Ambiguous-operation and restart recovery"
status: in_progress
wave: 5
depends_on:
  - "04-real-session"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NOMAD-002
  - REQ-EXECUTORS-NOMAD-004
acceptance_criteria:
  - AC-EXECUTORS-NOMAD-002.1
  - AC-EXECUTORS-NOMAD-002.2
  - AC-EXECUTORS-NOMAD-004.2
system_design:
  - ../../specs/executors/system-design/nomad-native-executor.md
---

# Task 05: Ambiguous-operation and restart recovery

## Summary

Prove original-operation recovery across lost register responses and backend restart. Retain one exact allocation, workspace and authenticated runtime.

## In scope

- Inject real post-commit response loss before host job/descriptor checkpoint; exercise found, unknown, conflict and positively never-submitted absence.
- Restart backend at allocating/staging/bootstrap/ready barriers with existing database/secret references; reattach exact allocation/NodeID/ServerIdentity without prompt duplication.
- Cover changed profile/input digest, stale generation/claim, foreign same-name job, unexpected allocation, history GC and retained CAS operation fences.

## Pre-instance continuation clarification

- Complete the original registered/allocated operation through a manager-owned
  pre-instance continuation before UUID allocation, using canonical authorized
  WorkspaceInfo and prepareExecutionCreateRequest with the original execution
  ID. Validate exact row/environment generation, operation/job/allocation/fence,
  admitted settings and current admission. GET/create-only-if-missing the
  original instance, then normal registration and first ACP initialization.
  Do not send/replay prompts or claim initialized-session recovery.
- Persist nonce value/reference before worker staging/register and token
  value/reference immediately after handshake, through existing manager vault
  and typed metadata CAS. Whitelist credential-reference updates; preserve
  unrelated metadata. Return AuthToken/BootstrapNonce on ExecutorInstance.
  Missing consumed credentials stay unknown; do not generate replacements.
- Test registered-without-instance continuation, ambiguous instance-create
  response, changed owner/config rejection and nonce/token checkpoint failure.
  A manual fixture-only original-credential repair is allowed only under the
  paired design's exact private-file/ownership/CAS procedure. Record it as
  operator repair, never automatic recovery proof. If credentials are lost,
  preserve/quarantine the operation and positively clear it before fresh work.

## Full job identity clarification

Follow the [startup continuation contract](../../specs/executors/system-design/nomad-native-startup-continuation.md)
for Created-session intent retention, post-readiness guarded workspace-only
continuation, ready-before-ACP classification, separately owned first dispatch
and cleanup-only operator repair of already-pruned test inventory. Retention
alone is insufficient: generic initialized recovery cannot consume pending
creation. Add the specified startup/terminal-race cases and a clean real backend
kill/restart without repair; repaired fixture cleanup is separate evidence.
Its workspace-input section also separates the exact original digest input
(including captured empty) from the resolved runtime path. Capture it in typed
intent metadata before dispatch and use it consistently in manager/executor
digest checks. Legacy continuation needs unique durable original provenance
and an exact saved-digest match; never rewrite a digest/job to fit projection.

Follow the [job identity contract](../../specs/executors/system-design/nomad-native-job-identity.md).
Persist original nonsecret staged paths, policy version and expected canonical
job-spec SHA before registration. Validate full raw job/allocation policy with
closed path-specific normalization before projecting to a small struct; owner
metadata alone is insufficient. Preserve true CreateIndex and JobModifyIndex
separately from volatile ModifyIndex. Pre-release missing evidence stays unknown.
Include default-round-trip and foreign behavioral-field tests named there.

## Fenced environment readiness publication

Follows startup-continuation design § "Post-readiness pending-operation branch"
step 4 (publication) for the environment row, and executor design § "Pre-instance
continuation and credential checkpoints". Guarded Created-session recovery reaches
runtime phase Ready but never writes `task_environments`: the orchestrator
`persistTaskEnvironment` → `FinalizeTaskEnvironmentMaterialization` path died with
the crashed launch. The environment stays `creating` with an empty path, so every
ordinary workspace read rejects once the recovery guard is released.

**Deliberate deviation (named).** The startup-continuation design does not contain
a `task_environments` `creating`→`ready` write; this is the only non-orchestrator
writer of that transition, and only for `nomad` environments. Specs are frozen on
this card, so the deviation ships recorded here and in the plan, and the owed
follow-up is an amendment of the startup-continuation design (step 4) to add the
environment publication, the re-election live-evidence rule below and the
error mapping below. The PR body must name it. It is not silently ahead of its
authority: the amendment is a blocking item for marking this work order complete.

### Repository method

Add `PublishRecoveredNomadEnvironmentReady(ctx, params)` in a new
`sqlite/task_environment_nomad_recovery.go`, called from lifecycle through a narrow
interface (name it in the lifecycle package, wired in `backendapp`). Params: session
ID, environment ID, execution ID, ownership generation, `expectedRevision`, and repo
rows. There is no operation-ID or expected-path param: the operation identity is the
execution ID (`createdNomadRecoveryStateEligible` already requires them equal), and
the path compared is `state.WorkspacePath` from the locked inventory row against the
locked environment row's `workspace_path` (empty or identical). `expectedRevision` is the inventory revision from the lifecycle's immediately
preceding re-read of the inventory (never the pending entry's pinned revision, which
is stale once the first attempt advanced to `ready`). Repo rows are projected from
the canonical `WorkspaceInfo.WorkspaceRepositories` of the prepared continuation
request, using the same `RepositoryID`/`BranchIdentitySlug`/`WorkspaceRelativePath`/
`Position` projection as `environmentReposForLaunch`'s clone-based branch. The branch
slug copies `launchRepoBranchIdentitySlug` exactly (its fallback to the branch slug
and its sanitising, `executor_environment_reuse.go`), re-implemented in lifecycle
because lifecycle cannot import the orchestrator; a test in
`internal/orchestrator/executor` asserts the two functions agree on the same inputs
so the first-prompt `PersistTaskEnvironmentTransition` row match (RepositoryID plus
sanitised BranchSlug) never produces duplicates. Invent no worktree path or branch. "Repo-backed" is decided by the SQL `task_repositories`
table inside the transaction (`taskHasRepositoriesTx`, as Finalize does), not by
lifecycle `WorkspaceInfo`: a repo-backed task with an empty projection returns
`ErrWorkspaceReuseUnsafe`, keeps the guard, and is tested.

`ErrNomadRecoveryNotReady` is declared in `internal/task/models` next to
`ErrExecutionRotated`, so lifecycle can `errors.Is` it without importing the sqlite
package. Params carry no task ID: after lock 1 and before lock 2 the method selects
the environment row's `task_id` explicitly (as `FinalizeTaskEnvironmentMaterialization`
does) and uses it for the barrier and recovery-claim helpers. A missing environment
row therefore surfaces (`ErrExecutionRotated`) before step C; "C is evaluated first"
holds once the environment row exists.

Lock order, one transaction. Lock 1 is `lockTaskEnvironmentMaterializationTx` on the
environment ID, which resolves the environment's owning task and takes that task's
row first; `CheckpointNomadExecutorInventory` takes the session's task row first. For
a Nomad environment the session's task and the environment's task are the same
unless the session is an `inherit_parent`/`shared_group` guest; a guest session never
runs Nomad recovery publication (the preconditions below require the environment's
`task_id` to equal the session's `task_id`, else `ErrExecutionRotated`), so both
writers take the same task row and cannot deadlock. After lock 1:
2. `taskCleanupBarrierLocked`
3. `recoveryclaim.EnsureAvailableTx`
4. `lockTaskSessionRow`
5. `task_environments` row FOR UPDATE
6. `executors_running` row FOR UPDATE (PostgreSQL)
7. `lockEnvironmentPhysicalClaims`
Locks 5 and 6 are taken in the same order as `CheckpointNomadExecutorInventory`
(session row, then environment row, then `executors_running` row); a test asserts
the two cannot deadlock under concurrent calls.

Preconditions, evaluated under those locks and BEFORE any already-`ready`
short-circuit. The order is C, A, B, D: C runs first because the barrier and
recovery-claim helpers return their error while taking lock steps 2 and 3, before the
session, environment and inventory rows are locked. So if a cleanup barrier and a
stale revision are both present the result is the barrier error (retry). Steps A to C
apply to both the `creating` and the `ready` branch; only step D branches. A row that
does not exist (session, environment, or `executors_running`) is not read as a
default: after lock 1 the method selects each row explicitly and maps `sql.ErrNoRows`
as in the table. Definitions:
A. Session: a Created session whose `task_environment_id` is this environment, with
   the same `task_id` as the environment.
B. Inventory: an `executors_running` row for the same execution whose decoded state
   has operation equal to the execution ID, the same environment ID and ownership
   generation, and non-empty `WorkspacePath`, allocation and server identity. Checks
   inside B run in this order: identity (operation, environment, generation,
   execution), then phase, then (phase `ready` only) the non-empty checks, then
   revision. Identity includes the inventory's environment ID: any mismatch is
   `ErrExecutionRotated`. Phase `ready` proceeds; phase `intent`, `registered` or
   `allocated` returns `ErrNomadRecoveryNotReady` before any non-empty check runs
   (allocation and server identity are legitimately empty then); phase
   `cleanup_pending`, `quarantined` or `absent` returns `ErrExecutionRotated`
   (ready can only advance to those, so it is terminal). In phase `ready`, an
   empty `WorkspacePath`, allocation or server identity is `ErrExecutionRotated`
   (terminal: a ready inventory without them is not the retained operation). Last,
   the decoded revision must equal `expectedRevision`, else `ErrExecutionRotated`.
C. Claims: no cleanup barrier or recovery claim (steps 2 and 3). Evaluated first.
D. Environment, a `nomad` environment at the same ownership generation:
   - `creating`, materialized by that session, with an empty or identical path:
     write (below).
   - `ready` with the same ID, ownership generation, `executor_type='nomad'`,
     `workspace_path = state.WorkspacePath` and empty materializer: return
     `(false, nil)` without writing (idempotent replay, and the race-loser outcome).
   - an environment ownership generation different from the inventory's is
     `ErrExecutionRotated` (checked before the status branch); any other state
     (status not `creating`/`ready`, foreign materializer, different path, not
     `nomad`): `ErrWorkspaceReuseUnsafe`.
`updated_at` is not a caller input; the CAS token is read from the locked row.

Error mapping. The retry column names what the lifecycle ticker does next (see
Lifecycle publication): terminal means the entry is marked operator-action and the
ticker stops attempting it until restart; retry means the next tick attempts again.

| Failure | Sentinel | Retry |
|---|---|---|
| stale revision, wrong operation or execution, inventory environment ID mismatch, ready inventory with empty `WorkspacePath`/allocation/server identity, environment or inventory generation mismatch (either side, one sentinel), session/env task mismatch, session bound to a different environment, missing session, environment or `executors_running` row (`sql.ErrNoRows`), inventory phase `cleanup_pending`/`quarantined`/`absent`, zero-row CAS | `ErrExecutionRotated` | terminal |
| non-Created session, foreign materializer, different path, empty repo projection for a repo-backed task, environment not `creating`/`ready` | `ErrWorkspaceReuseUnsafe` | terminal |
| cleanup barrier or recovery claim | the existing error from the barrier helpers, passed through | retry |
| inventory phase `intent`, `registered` or `allocated` | new `ErrNomadRecoveryNotReady` | retry |
| canceled context | the context error, rolled back | retry |
| database error | wrapped, rolled back | retry |

`ErrNomadRecoveryNotReady` is never used for a post-ready phase. A PostgreSQL
deadlock abort or serialization failure from a concurrent guest-session writer is a
database error row (retry), not a sentinel.

Write: store the repo inventory through a helper extracted from Finalize, which
REPLACES the environment's `task_environment_repos` rows for that environment
(delete then insert, as Finalize does, inside the same locks). Rows left by a
partial earlier attempt are therefore overwritten, never merged and never treated
as a conflict; `lockEnvironmentPhysicalClaims` still refuses a physical claim held
by another environment. Then CAS `status='ready', materialization_session_id='',
workspace_path=state.WorkspacePath` guarded by the same fields and the locked
row's `updated_at`.

Columns decision. The publish writes only `status`, `materialization_session_id`,
`workspace_path`, `updated_at` and the repo rows. It deliberately leaves
`workspace_layout`, `task_dir_name`, container and secret IDs and the base-branch
self-heal alone, because the Nomad launch path does not use them and no Nomad
workspace reader (files, terminal, Git) reads them while the row is `ready`;
workspace reads resolve through `workspace_path` and the repo rows. The later
first-prompt `persistTaskEnvironment` already tolerates a `ready` Nomad row: with
`isInitialMaterializer` false (status is not `creating`) it takes the non-initial
branch, refreshes `workspace_path`, `task_dir_name` and `workspace_layout` from
the request, keeps `status` ready, and rewrites repos through
`PersistTaskEnvironmentTransition` without a reuse-unsafe rejection when the
inventory is present. The builder adds a regression test over that path with a
published row (no status regression, no `ErrWorkspaceReuseUnsafe`, layout and task
dir name filled) and, if it fails, fixes `persistTaskEnvironment`, not the publish; the fix is limited to the branch for an existing `ready` `nomad` row that has an inventory and touches no other executor path.

### Lifecycle publication

Shadow paths, decided: an environment already `failed` before this change, or failed by
`FailTaskEnvironmentMaterialization` (the launch-gave-up path, which gets no companion
guard here), makes publish return `ErrWorkspaceReuseUnsafe` (terminal, `operator_action`);
its operator action is the existing cleanup-only repair and WO05 does not change reset
behavior or touch the retained Nomad job. A guest-session `CheckpointNomadExecutorInventory`
on PostgreSQL may lock the environment row before the owner task row, the reverse of
publish; one transaction is then aborted by the database and surfaces as the retryable
database error row.

Implement in `attemptCreatedNomadRecovery` (`remote_recovery.go`), in this order:
reconcile repositories → re-read inventory and require phase `ready` (its revision
becomes `expectedRevision`; phase `intent`/`registered`/`allocated` ends the attempt
as `starting` without calling publish, phase `cleanup_pending`/`quarantined`/`absent`
ends it as `operator_action`) → publish under the in-flight guarded owner predicate →
re-read and require environment `ready` → `verifyCreatedNomadRecoveryFinal` →
`completeCreatedNomadRecovery`.

Repository reconciliation runs the existing `materializeWorkspaceRepositories`
through the attached instance's agentctl client (the execution returned by
`createdNomadRecoveryExecution`, so reconcile runs after that call and before
`verifyCreatedNomadRecoveryFinal`). Reconcile input: the list is rebuilt from the
same `RepoLaunchSpec` projection the original launch used
(`remoteWorkspaceProjectionFromLaunch` with `includePrimary` true for Nomad), not
from `WorkspaceRepositorySpec` alone, because that type carries no clone URL.
Recovery obtains the clone URL from the same resolver the resume path uses
(`executor_resume.go`, the `cloneURL` derivation in the launch-request builder) and
carries it into `WorkspaceInfo` as a new `RepositoryURL` field on
`WorkspaceRepositorySpec`, populated by the workspace info provider; the destination
comes from `remoteWorkspaceRepositoryDestination` over the repository name and
checkout/base branch. agentctl reuses a checkout only when `remote.origin.url` equals
the locator byte for byte, so a repository whose locator cannot be derived (empty
URL) fails closed as `operator_action` and nothing is published. Test: reconcile over
the original launch's checkout returns `Reused` for every repository and makes no
write; a locator that differs by one byte is a collision classed `operator_action`. Partial-clone handling: fail closed. A directory counts as done only when it is a
valid checkout of the expected repository (git reports a `HEAD` commit and the
origin matches). An existing directory that is not (interrupted or half-cloned,
wrong origin, no `HEAD`) is never deleted, re-cloned or reset: reconciliation
returns an error classed `operator_action`, the guard stays, and nothing is
published. A valid existing checkout is never reset. agentctl already
returns a collision for an existing non-checkout or mismatched destination and
leaves no destination after a cancelled clone (see
`TestMaterializeRepository_CancelledCloneLeavesNoDestination` and
`RejectsExistingCheckoutAtDifferentCommit`); WO05 changes no agentctl code. The
builder confirms the lifecycle classification on the real function with a test using
a directory holding an incomplete clone; if the checks prove insufficient the builder
records a follow-up rather than changing agentctl here. The operator procedure for a
`operator_action` directory: inspect and remove or repair the directory on the worker,
then restart the backend (which clears the in-memory class). Publication happens only after
reconciliation succeeds, so a bad HEAD is never published `ready`.

The same-process retry predicate extracts only the owner/entry-active/
attempt-allowed/identity/snapshot-eligibility checks from
`ensureWorkspaceSessionAdmittedForCreatedNomadRecovery`, and compares the entry to
the freshly read inventory with `sameNomadPreACPOperation` plus monotonic
revision/phase, not full struct equality, so a first attempt that advanced to
`ready` and failed to publish is retried to success. A failure keeps the guard and
pending entry; a stop racing publication leaves the environment `ready`,
completion refuses, and the stop path owns the guard; after that stop completes,
ordinary workspace admission is governed by the session's terminal state like any
other stopped session and publication is not retried.

Retry trigger and meaning of retryable. The only trigger is the existing
`remoteStatusLoop` ticker: `pollRemoteStatuses` → `retryPendingRemoteRecoveries` →
`attachRemoteRecovery` → `continueCreatedNomadRecovery` (one attempt per tick per
pending entry, serialised by the existing pending-attempt guard). A workspace read
(`GetOrEnsureExecution`) never starts an attempt: it is rejected by
`rejectWorkspaceDuringCreatedNomadRecovery` before reaching recovery, and this work
order adds no read-triggered path. Each attempt records its outcome class on the
pending entry (in memory, `remoteRecoveryRetry`): `starting` (retry rows, and an
inventory re-read in phase `intent`/`registered`/`allocated`) or `operator_action`
(terminal rows, a re-read in `cleanup_pending`/`quarantined`/`absent`, a partial or
mismatching checkout, and any attempt-fence loss). An entry classed `operator_action`
is skipped by the ticker for publish and reconciliation (no further Nomad contact or
git work) until the process restarts, which clears the in-memory class and
re-evaluates from persisted state; the guard and pending entry stay.

Default class and attempt failures. Any error from `attemptCreatedNomadRecovery` not
named elsewhere classes `starting`. Fence and identity losses class `operator_action`:
owner changed, snapshot ineligible, workspace-mismatch fields, `currentNomadPreACPRecoveryRecord`
"operation or phase changed", final snapshot ineligible or owner changed, and
completion refused. Read and transport failures class `starting`: snapshot reads,
Nomad API errors and `ErrSessionWorkspaceNotReady` from `createExecutionForCreatedNomadRecovery`.
Reconcile errors split by the agentctl HTTP status carried by
`agentctl.WorkspaceMaterializationError.StatusCode` (already exposed by the client): 409
(collision) and 422 (directory error) class `operator_action`; every other reconcile
failure (network, timeout, 5xx, transient clone failure such as DNS or an expired token)
classes `starting`.

Stop retries. The `operator_action` skip is applied in `retryRemoteRecovery` after
`retryPendingRemoteStop`, so a requested stop on such an entry still makes progress; only
reconciliation, publish and Nomad contact for recovery are skipped.

Surfacing. `rejectWorkspaceDuringCreatedNomadRecovery` returns a typed error that
satisfies `errors.Is(err, ErrSessionRecoveryGuarded)` and carries a stable code read
from the entry's class: `nomad_recovery_starting` or `nomad_recovery_operator_action`;
an entry with no recorded attempt yet reports `nomad_recovery_starting`. Callers
receive the code, not free text; the backend adds no user-facing copy in this work
order (any UI wording is a later localized change using these codes). Barrier, claim,
canceled-context and database outcomes all class as `starting`. Terminal sentinels
class as `operator_action`.

Wire shape (SA-023). The typed error's `Error()` is `workspace recovery guarded: <code>`
and it exposes `Code`. `wrapSessionRecoveryGuardError` (`backendapp/adapters.go`)
carries the code into `orchestrator.SessionRecoveryGuardError` through a new `Code`
field and keeps `Retryable` true for both `nomad_recovery_starting` and
`nomad_recovery_operator_action` (decision SA-030; `Code` is the only discriminator and
`Error()`/`Details()` do not vary by it); other `ErrSessionRecoveryGuarded` causes keep
today's `Retryable: true`. A test in `backendapp` covers both codes. Binding a new session to
the same task keeps its existing `WorkspacePreparingError` behavior in this work order
(a later localized UI change may distinguish the codes); this is recorded, not fixed here.

Time budget. Repository reconciliation runs under a 2 minute budget
(`nomadRecoveryReconcileTimeout`), because the attempt runs inside the singleflight's
`WithoutCancel` context and the 15 second `remoteRecoveryAttemptTimeout` does not
bound it. The budget bounds the whole reconcile call, and the git operations must be
context-bound (the builder confirms they use the context; if not, they are made so).
A budget expiry is classed `starting` and releases the singleflight key; the next tick
retries, and a half-written directory is then judged by the fail-closed rule below.

Attempt context. The 15 second `remoteRecoveryAttemptTimeout` would otherwise end the
ticker's wait while the singleflight body keeps running, after which
`finishRemoteRecoveryAttempt` clears `inProgress` and `createdNomadRecoveryEntryActive`
(which requires `inProgress` and a live `attemptDone`) would fail the publish
predicate. Decision: for an entry whose record is a Created-session Nomad recovery,
`retryRemoteRecovery` derives its attempt context with a budget of
`nomadRecoveryReconcileTimeout + 2*remoteRecoveryAttemptTimeout` (2m30s), so
`inProgress` stays true for the whole body; the publish step itself keeps its own
`remoteAttemptContext(WithoutCancel(...))` bound. Other recovery kinds keep 15
seconds. The outcome class is recorded inside `attemptCreatedNomadRecovery` from the
body's own error, never by the awaiting caller (which only sees a context error and
records nothing). `errRemoteRecoveryFenced` classes `operator_action` only when the
entry was removed or replaced, a stop was requested or the owner changed; a loss
caused only by the attempt window ending classes `starting`. Test
`TestCreatedNomadRecoveryPublish_ReconcileLongerThanAttemptTimeoutPublishes` injects a
short `remoteRecoveryAttemptTimeout` through a manager field and a reconcile slower
than it, and asserts publish and completion succeed.

Restart classification. After a crash following a committed publish but before
`completeCreatedNomadRecovery`, startup finds environment `ready`, session
`CREATED`, inventory `ready`, no ACP session. That state is classified exactly like
the pre-publish `creating` case: the pending-owner branch is selected from the
Created session plus retained inventory and ignores environment status, so the
replay reaches `PublishRecoveredNomadEnvironmentReady`, which returns `(false, nil)`,
then verifies and completes. Ordinary admission and the generic initialized path
are never selected for it. A lifecycle test starts from exactly that persisted
state and asserts the guard is released once, no publish write occurs, and no
prompt is sent.

User-visible outcome: on success the environment is `ready`, path equals
`state.WorkspacePath`, workspace files/terminal/Git work, the session stays
`CREATED` and no prompt is sent. On failure workspace reads keep returning
`ErrSessionRecoveryGuarded` with the sanitized outcome above until a successful
retry or restart.

### Companion re-election guard

Treat any `executors_running` row with `runtime='nomad'` whose session is bound to
the environment (`task_sessions.task_environment_id = task_environments.id`) as
live evidence: in `workspaceMaterializationAbandonedTx` (so the claim is not
abandoned and the environment is not marked failed by
`settleWorkspaceMaterializationClaim`) and in
`reelectTaskEnvironmentMaterializationSQL` via `NOT EXISTS`. Position: the check is the first statement of `workspaceMaterializationAbandonedTx`,
before every existing early return (empty owner, missing owner row, terminal owner,
within the abandon window, not Starting/Running), so a retained Nomad row blocks
abandonment whatever the owner's state, including a CANCELLED owner. The environment
then stays `creating` behind the recovery guard until the stop or cleanup path deletes
the row. A regression covers a missing and a terminal owner. Do not parse
metadata; malformed metadata counts as live. This changes claim semantics only for
environments that have such a row, so it is limited to the Nomad runtime and every
other executor's claim behavior is byte-unchanged (a regression asserts that).
The row is not time-limited by design: its owner is the Nomad cleanup and stop
paths, which delete the `executors_running` row when the operation is positively
cleared, and a row that outlives a dead job is the retained-evidence case the ADR
requires to stay unknown. The user-visible state of such an environment is
`creating` with workspace reads returning `ErrSessionRecoveryGuarded`, and the
operator action is the documented cleanup-only repair that clears the operation;
nothing re-elects or fails it automatically. Regressions: creating take-over, the
fail-abandoned branch, and all three failed-reelect callers (`session.go` twice and
`ReelectFailedTaskEnvironmentMaterialization`). `workspaceMaterializationAbandonedTx`
today takes no environment ID; the builder adds that parameter for the join. This guard is the only part
that could be split into its own work order; it stays here because publication
without it can be undone by a take-over, and the split is a Review-step option, not
a Build decision.

### Tests first

Test function names (prefixes are what the Verification `-run` patterns match;
Verification also requires `go test -list` to show at least one match per prefix).

SQLite (`TestPublishRecoveredNomadEnvironmentReady_*`): `_Success` with repo
inventory, `_StaleGeneration`, `_WrongOperation`, `_StaleRevision`,
`_PreReadyPhase` (`ErrNomadRecoveryNotReady`), `_PostReadyPhase`
(`ErrExecutionRotated`), `_ForeignMaterializer`, `_CleanupOrRecoveryClaimFirst`
(barrier plus stale revision returns the barrier error), `_NonCreatedSession`,
`_TaskMismatch`, `_MissingRows`, `_IdempotentReplay` (ready branch still checks A to
C), `_DifferentPath`, `_RepoBackedEmptyProjection`, `_PartialRepoRowsReplaced`,
`_ConcurrentOneWinner` (loser `(false, nil)`), and
`_PostgresRacesCheckpointNomadExecutorInventory`, gated on
`KANDEV_TEST_POSTGRES_DSN` and run under `-race`: it runs publish and
`CheckpointNomadExecutorInventory` concurrently on one environment/session and asserts
no deadlock and a consistent final row. (A SQLite lock-order test proves nothing,
since there is one writer, and is not counted.) Re-election guard
(`TestReelectNomad_*`): creating take-over, fail-abandoned, three failed-reelect
callers, other executors unchanged.

Lifecycle (`TestCreatedNomadRecoveryPublish_*`): publish before guard release and
ordinary admission succeeds afterwards, no publish for a non-recovery or stopped
caller, failure retains the guard, first attempt advances to ready then publication
fails and the next ticker attempt publishes using the re-read revision, replay after
committed publish does not double-publish, restart from environment `ready` plus
Created session completes without a write, pre-ready phase does not publish and
classes `starting`, post-ready phase classes `operator_action` and the ticker skips
it, the typed guard error carries the stable code and still satisfies
`errors.Is(ErrSessionRecoveryGuarded)`, a workspace read never starts an attempt,
a partial clone or a 409/422 reconcile collision keeps the guard with `operator_action`,
a transient reconcile failure (non-409/422) and a reconciliation budget expiry class
`starting`, a stop requested on an `operator_action` entry still proceeds, and the
`Retryable` flag follows the code.

Added by the round 4 fixes: `TestCreatedNomadRecoveryPublish_ReconcileLongerThanAttemptTimeoutPublishes`,
`_ReconcileReusesOriginalCheckout`, `_ReconcileLocatorMismatchIsOperatorAction`,
`_TransientReconcileFailureClassesStarting`, `_StopProceedsOnOperatorActionEntry`;
sqlite `TestPublishRecoveredNomadEnvironmentReady_ReadyInventoryEmptyFields` and
`_MissingEnvironmentRow`; `TestReelectNomad_MissingOrTerminalOwnerWithRetainedRow`;
and a `backendapp` test of `wrapSessionRecoveryGuardError` for both codes.

Orchestrator (`internal/orchestrator/executor`,
`TestPersistTaskEnvironment_PublishedNomadReady_*`): first-prompt
`persistTaskEnvironment` over a published row: no status regression, no
`ErrWorkspaceReuseUnsafe`, layout and task dir name filled, no duplicate repo rows,
and slug parity with the lifecycle projection.

The real proof remains a fresh captured-input pre-ready crash whose runner asserts
environment `ready`, the selected repository HEAD, and a normal workspace read before
any prompt, with the same execution, job, allocation and fence.

### Known failing tests (decided from the design)

- `TestBuildBootstrapScriptUsesPermanentWorkerFenceAndPrivateFile`: code is right,
  the test literal is stale. The expected `if ! mkdir '<path>'` is a prefix of the
  emitted line, so `2>/dev/null` is not the cause; the path is.
  `bootstrap.go` derives `Dir(Dir(AllocationRoot))/worker-state/<NodeID>`, which
  job identity (`KANDEV_WORKER_STATE_ROOT`) and captured fixtures already use.
  Fix the literal to `/var/lib/worker-state/node-b/active-environment`; do not
  change the derivation (it would change `NativeJobSpecSHA256`).
- `TestNomadCreateInstanceReattachesRetainedOperationWithoutRegisterOrRestage`:
  fixture is wrong. Add `WorkspaceInputPath: request.WorkspacePath` and
  `WorkspaceInputCaptured: true` (startup-continuation § workspace input). Keep the
  captured=false negative.

## Out of scope

- Rendered UI, Docker/VM execution, plugin implementation, provider SDK/protobuf changes, model-backed workloads, and production rollout.

## Acceptance

- A lost response or backend restart never produces a second environment or substitutes a workspace; recovery outcomes require exact original-operation evidence.
- Changed identity/generation/input or unknown history cannot mutate/delete the original or successor and retains retryable ownership.
- After a captured-input pre-ready crash and backend restart, the `nomad` task environment is `ready` with `workspace_path` equal to the retained inventory's `WorkspacePath`, one `task_environment_repos` row per repository, the session is still `CREATED` and no prompt was sent; workspace reads then succeed. A failed or terminal publication leaves workspace reads returning the guard code and never `ready`.
- In the second bullet, "retains retryable ownership" means the pending entry and guard are retained; an entry classed `operator_action` is not retried until backend restart.
- Ready restart restores authenticated control/instance/reverse API transport and one prompt/result sequence without re-running bootstrap or launching another controller.

## Verification

Run the focused behavioral checks and the strict attached-worker recovery
scenario. The Go cases below cover stateful protocol barriers; they do not
substitute for the real backend restart scenarios in the attached fixture.
No-tests-to-run or a skipped required real scenario fails the work order.

```bash
: "${KANDEV_ROOT:?Set absolute Kandev worktree path}"
: "${KANDEV_NOMAD_TEST_CONFIG:?Set absolute fixture-only JSON config path}"
(cd "$KANDEV_ROOT/apps/backend" && GOFLAGS=-p=1 go test ./internal/agent/nomad -run '^(TestBuildBootstrapScriptUsesPermanentWorkerFenceAndPrivateFile|TestNomad207ObservedResponseDefaultsAreExplicitAndStopRemainsBehavior)$' -count=1)
(cd "$KANDEV_ROOT/apps/backend" && GOFLAGS=-p=1 go test ./internal/agent/runtime/lifecycle -run '^(TestNomad(Create(ReconcilesCommittedRegistrationAfterResponseLoss|FailureRetainsDurableIntentWithoutClaimingAllocation|InstanceReattachesRetainedOperationWithoutRegisterOrRestage)|IntentRetryReconcilesCommittedJobWithoutRegisteringAgain|StagingBarrierRemainsUnknownAndDoesNotRegisterOnRetry|RegisteredAllocationBarrierResumesSameJob|BootstrapBarrierRetainsCleanupPendingAndRejectsRetry|RetainedOperationFailsClosedOnChangedInputOrMissingHistory|RetainedOperationRejectsUnexpectedAllocationSet|ReadyContinuation(AttachesExistingInstanceWithoutCreatingReplacement|RejectsMissingInstanceWithoutCreatingReplacement|RequiresAttachOnlyFlag))|TestCreatedNomadRecovery(PublishesWorkspaceOnlyExecutionForOriginalOperation|Publish_.*)|TestRetainedNomadExecutionIdentityRequiresExactCurrentOwnerAndRecoverablePhase|TestUnpinnedNomadRecoveryOwnerBlocksWorkspaceUntilSnapshotIsAvailable)$' -count=1)
(cd "$KANDEV_ROOT/apps/backend" && GOFLAGS=-p=1 go test ./internal/task/repository/sqlite -run '^(TestPublishRecoveredNomadEnvironmentReady_.*|TestReelectNomad_.*)$' -count=1)
(cd "$KANDEV_ROOT/apps/backend" && GOFLAGS=-p=1 go test ./internal/backendapp -run '^TestWrapSessionRecoveryGuardError_.*$' -count=1)
(cd "$KANDEV_ROOT/apps/backend" && GOFLAGS=-p=1 go test ./internal/orchestrator/executor -run '^TestPersistTaskEnvironment_PublishedNomadReady_.*$' -count=1)
(cd "$KANDEV_ROOT/apps/backend" && KANDEV_TEST_POSTGRES_DSN="${KANDEV_TEST_POSTGRES_DSN:?set a disposable Postgres DSN}" go test ./internal/task/repository/sqlite -race -run '^TestPublishRecoveredNomadEnvironmentReady_PostgresRacesCheckpointNomadExecutorInventory$' -count=1)
for spec in "./internal/task/repository/sqlite PublishRecoveredNomadEnvironmentReady_" "./internal/task/repository/sqlite ReelectNomad_" "./internal/agent/runtime/lifecycle CreatedNomadRecoveryPublish_" "./internal/orchestrator/executor PersistTaskEnvironment_PublishedNomadReady_"; do set -- $spec; n=$(cd "$KANDEV_ROOT/apps/backend" && go test "$1" -list "$2" | grep -c '^Test'); [ "$n" -ge 1 ] || { echo "no tests match $2 in $1" >&2; exit 1; }; done
(cd "$KANDEV_ROOT" && scripts/nomad-native-e2e run --require-real --scenario recovery --config "$KANDEV_NOMAD_TEST_CONFIG")
```

## Files likely touched

- `apps/backend/internal/agent/nomad/recovery.go and tests (new)`
- `apps/backend/internal/agent/runtime/lifecycle/executor_nomad_recovery.go and tests (new)`
- `apps/backend/internal/agent/nomad/integration/recovery_test.go and fault barriers (new)`
- `scripts/nomad-native-e2e`
- `apps/backend/internal/task/repository/sqlite/task_environment_nomad_recovery.go`, `task_environment.go`, `session.go` and tests
- `apps/backend/internal/agent/runtime/lifecycle/remote_recovery.go`, `manager_workspace_admission.go`, `manager_execution.go` (typed guard error), the narrow publish interface and its `backendapp` wiring (including `wrapSessionRecoveryGuardError` and its test), `internal/task/models` (`ErrNomadRecoveryNotReady`), `WorkspaceRepositorySpec.RepositoryURL` and the workspace info provider that fills it
- `apps/backend/internal/orchestrator/executor/` regression tests (`TestPersistTaskEnvironment_PublishedNomadReady_*`, slug parity) and `persistTaskEnvironment` only if the regression fails
- `docs/plans/nomad-native-executor/plan.md` (records the deliberate deviation and the owed startup-continuation design amendment), `docs/evidence/` (real-worker recovery evidence)
- Scrub targets: `internal/agent/nomad/testdata/nomad-2.0.7-allocation-stopped.json`, `task-04-real-session.md`, Nomad SSH-target test fixtures

## Dependencies

[Task 04](task-04-real-session.md).

## Risks

Timeout does not cancel a committed register. A single missing job/empty allocation query after possible dispatch is unknown, never absence. Do not copy plugin-specific recovery wiring blindly.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/nomad-native-executor.md), using frontmatter IDs.
- [System design](../../specs/executors/system-design/nomad-native-executor.md), matching implementation boundary and current source.
- [Plan](plan.md), including strict real-worker runner/prerequisites.
- [ADR](../../decisions/2026-10-24-native-nomad-cleanup-evidence.md).
- Read applicable scoped `AGENTS.md` before editing code.

## Results

The strict attached-fixture recovery runner passed for one committed registration
response loss and a backend restart from a ready allocation. It verified the
same task/session, execution, Nomad job and allocation, NodeID, server identity,
workspace, and process tree after restart. Two prior completed prompt/result
pairs remained single-delivery; the new post-restart inert prompt produced one
result. This is live evidence for the commit-response-loss and ready-restart
barriers only; it does not prove restart behavior at earlier startup phases.

Added deterministic lifecycle fault tests exercise five other outcomes:

- Staging interruption leaves typed `intent`; a retry with missing job history
  fails closed without another registration or instance creation. Although the
  test observed no registration call, a restarted production process cannot
  infer that fact from a missing GET, so it must retain the operation as
  unknown.
- A registered job with no allocation times out without changing job identity;
  when the exact allocation appears, retry reattaches and registers only once.
- Bootstrap rejection leaves the allocation in `cleanup_pending`; retries are
  rejected rather than generating a replacement credential or job.
- Changed worker/workspace/generation identity, missing job history, and
  multiple allocations all fail closed without dispatch or replacement.

These deterministic fixture tests are not live process-crash simulations. A
later diagnostic restart of the same retained pre-ACP operation reached
`ready` while the session remained `CREATED`, preserving the original
execution, job, allocation, NodeID, server identity, and workspace; the harness
submitted no prompt. That fixture had required an explicit pre-release
workspace-input repair, so this is operator-assisted continuation evidence,
not proof of automatic recovery for newly captured intents. The live restart
exposed a missing `nomad_environment_generation` projection in the prepared
request. The manager now derives that launch-only field from the canonical
validated workspace owner, rejects conflicting metadata, and keeps typed
checkpoint state authoritative. Lifecycle and SQLite DAO regressions cover the
corrected path. Live pre-ready crash recovery for a fresh, automatically
captured intent and the remaining identity/history negatives are still
required; until then the work order remains incomplete. A crash before durable
dispatch evidence can leave `intent` with no observable job; that state remains
unknown and is not treated as positively never submitted.

The Created-session recovery path now has a manager-owned workspace-only
continuation for exact nonterminal Nomad owners. It revalidates the current
owner, environment generation, immutable input digest, staged job identity and
credential references; it uses the original execution ID, publishes no ACP
session, and keeps workspace access guarded until publication and a final owner
read succeed. If publication succeeds but that final read is transiently
unavailable, a retry accepts only the exact already-tracked uninitialized
execution and does not call the runtime a second time. Ready-before-ACP
continuation is attach-only: the saved auth-token reference is required, the
runtime requires the exact existing agentctl instance, and a missing instance
fails without registration or replacement. Focused tests cover these rules
and an unpinned startup owner blocking workspace access. These are local
deterministic tests; the damaged live fixture has not yet been automatically
recovered by this new continuation path.

### Build results (2026-10-25): code complete, real proof passed (run 10)

Commits `4ea68182c2` (stale test literals fixed, infrastructure identifiers scrubbed), `b5fdb19e85` (SQLite `PublishRecoveredNomadEnvironmentReady` and the Nomad re-election guard), `2fec5a781c` (lifecycle publication, typed guard error, `backendapp` wiring). Targeted tests pass: sqlite `TestPublishRecoveredNomadEnvironmentReady_*`/`TestReelectNomad_*`, lifecycle `TestCreatedNomadRecoveryPublish_*`, orchestrator `TestPersistTaskEnvironment_PublishedNomadReady_*` and slug parity, `backendapp` `TestWrapSessionRecoveryGuardError_*`. The PostgreSQL race test is present but skipped here (no `KANDEV_TEST_POSTGRES_DSN`); it has not been run. Pre-existing failures are unchanged: `backendapp` and `orchestrator/executor` fail the same 55 tests at 73c8155caf and now; lifecycle fails the same 23 lines before and after.

Decisions on the round 5 findings: SA-028 agentctl 409/422 classes `operator_action`, everything else `starting`, agentctl untouched. SA-029 recovery fills only `RepositoryURL` and the branch identity; a task whose original launch used sparse/on-demand checkout, contribution or qualified-PR options is not reproducible from persisted rows, so reconcile fails closed on those fields and otherwise reuses a matching checkout (limitation recorded, not widened). SA-030 both codes keep `Retryable` true; only `Code` is exposed. SA-031 the 2m30s attempt bound is accepted and documented on the constant. SA-032 no agentctl change; the budget context reaches the client call. SA-033 test added to Verification.

Not done: the fresh real-worker proof. The runner's `recovery` scenario verifies a post-ready fixture; a fresh captured-input pre-ready crash that asserts environment `ready` and a normal workspace read before any prompt needs runner support and a live run. This work order is not complete until that evidence is in `docs/evidence`. The startup-continuation design amendment (step 4) is also still owed.

### Real proof (2026-10-25): passed

`scripts/nomad-native-e2e run --require-real --scenario pre-ready-recovery --create-fixture` passed (run 10, exit 0, all 11 steps; supersedes runs 7 and 9) against a throwaway local Nomad 2.0.7 fixture, runner `84c1e3f6fc`, backend and agentctl built from `0e73ef6a15`. The backend was SIGKILLed at the fixture-only pre-publish barrier and restarted. Before any prompt the environment went `creating` to `ready`, a normal workspace read returned the expected file, the recovered checkout HEAD `4c687ac69a70a67447ba936ed7b79ac861002683` (read over pinned read-only SSH, equal to the fixture git-daemon repository HEAD) matched, and the execution, job (`kandev-f2bedea5c9b1d0ab231da7576ffe2cce`, index 186), allocation, node and fence were unchanged. Evidence: `docs/evidence/nomad-wo05/`. Runs 1 to 8 found and fixed runner bugs only (run 8 failed solely because the HEAD assertion read git snapshots, which exist only after a turn) (the history is in the evidence README).

Cleanup was an operator teardown: `session.stop` retained the allocation and fence as the work order 04 contract requires, and the runner did not claim production cleanup. Production whole-job stop remains work order 07's.

Still owed: the startup-continuation design amendment (the named deviation in `plan.md`), and the PostgreSQL race test, which was not run.

### Review round 1 follow-up (2026-10-25)

- Attempt window ending before publication now classes `starting`: `createdNomadPublishAllowed` returns the plain error when the entry still owns the session but the attempt window has ended (`TestCreatedNomadRecoveryPublish_AttemptWindowEndingBeforePublishClassesStarting`).
- The fixture runner now requires the workspace read to report the selected repository HEAD and compares it to the fixture repository's base-branch commit captured before launch (negative tests: missing and mismatched HEAD). HEAD is read over the pinned read-only SSH channel because git snapshots exist only after an agent turn (offline test: empty snapshots, HEAD still asserted). Run 9 passed this assertion; run 7 did not cover it.
- The owed design amendment also covers `docs/specs/tasks/system-design/task-launch-failure-recovery.md` ("Abandoned claims", "Re-election" and the physical-evidence list): the Nomad re-election guard makes those sections inaccurate for Nomad environments with a retained inventory row. It blocks marking this work order complete.
