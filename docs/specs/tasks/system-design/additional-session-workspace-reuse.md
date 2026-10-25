---
status: current
system: tasks
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004
created: 2026-08-30
updated: 2026-10-22
owners:
  - kandev
---

# Additional Session Workspace Reuse System Design

## Purpose and boundaries

The task system owns the canonical task environment, its physical worktree
inventory, and the execution lifecycle that reuses those resources. The
workspace system supplies repository and worktree resources, while the agent
runtime creates the process that consumes the resolved task workspace.

This design preserves one effective workspace identity across initial launch,
additional-session attachment, backend restart, workspace-only reconstruction,
executor transition, and agent promotion or resume. It follows
[ADR-2026-08-08-task-owned-worktree-lifetime](../../../decisions/2026-08-08-task-owned-worktree-lifetime.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001` | [Canonical environment and inventory](#canonical-environment-and-inventory) |
| `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002` | [Recovery and path authority](#recovery-and-path-authority) |
| `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003` | [Executor transition](#executor-transition), [Launch admission](#launch-admission) |
| `REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-004` | [Concurrent session visibility](#concurrent-session-visibility) |

## Canonical environment and inventory

`task_environments` owns the task-level execution environment.
`task_environment_repos` owns the ordered physical repository and worktree
inventory. `task_sessions.task_environment_id` attaches a session to that
environment; an execution ID or repository source path is not workspace
identity.

The initial materializer publishes a ready environment and complete repository
inventory. Additional sessions validate and attach to that environment without
running repository preparation or creating another physical owner. Missing,
creating, inconsistent, or unsupported environments fail through the existing
typed reuse errors.

No session row, sibling execution, or cached frontend projection is a workspace
authority. In particular, `task_sessions.workspace_path` and lifecycle
`WorkspaceInfo.WorkspacePath` cannot recover an environment that fails the
canonical attach checks.

## Recovery and path authority

`task/service.GetWorkspaceInfoForSession` reconstructs the runtime workspace
from the canonical environment-repository inventory. The recorded layout and validated environment path define the effective workspace.
Repository count alone cannot select a root for explicit parent or nested layouts.
Legacy environments retain their validated recorded paths. See the
[repository placement design](workspace-repository-placement.md). The lifecycle manager uses that value as the recovered
`AgentExecution.WorkspacePath` and as the agentctl working directory.

The lifecycle adapter returns both the execution workspace and any optional
worktree metadata. Workspace-only reconstruction can legitimately return the
correct `WorkspacePath` without a `worktree_path` metadata key when the
execution is promoted to an agent execution.

The orchestrator persists an effective workspace using this authority order:

1. The lifecycle response's non-empty `WorkspacePath`, which is the actual
   execution working directory.
2. The response's non-empty `WorktreePath` for legacy adapters that do not
   expose an execution workspace.
3. The request's `RepositoryPath` only when no materialized runtime workspace
   is available.

The source repository path is preparation input. It must not overwrite a
materialized task workspace reported by the lifecycle response.

## Comparison base and existing checkout identity

For AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.4, a mutable comparison
base is not the identity of a materialized checkout. Before canonical inventory
validation and worktree-ID reuse, resolve an unambiguous existing Worktree slot
from the canonical environment rows by repository and workspace-relative path.
The retained row must carry its worktree ID, path, checkout branch, recorded
source clone, common directory, and nonempty branch key. Incomplete legacy rows
retain the existing guarded inventory-repair path and attestation requirements.
Only a single logical requested slot and a single eligible physical row owned
by the requesting task may use this rule. Inherited environments retain their
existing exact-selection contract. Pin its recorded `branch_slug` and worktree ID into the launch
request while retaining its current `BaseBranch` and comparison metadata.
Do not rewrite stored slugs, Git metadata, files, or task/session states.
An explicit inventory-repair request still validates its requested branch key
without this preservation rule, so normal reuse cannot replace the repair
receipt, idempotency, and post-repair attestation contract.

Explicit checkout branches retain exact selection. Duplicate logical slots,
multiple eligible rows, conflicting supplied worktree IDs, failed/deleted rows,
and mismatched placements cannot grant fallback authority. A valid row does not
hide an invalid selected sibling. Filesystem, origin, registration, ownership,
executor, and recovery-attestation checks still run after selection. Local and
remote executor contracts are unchanged. The materializing session's resume and
attach-only launch/resume must consume the same selected canonical key.

Delivery: [Workspace identity reuse fixes](../../../plans/recorded-source-path-identity/plan.md).

## Executor transition

An executor-type mismatch changes the environment selection outcome; it is not
merely a request-metadata filtering decision. Environment reuse correctly skips
runtime handles when the executor type changes, but a later lifecycle path can
still create a local execution from the old persisted `workspace_path`. A
missing or non-Git directory can therefore be reported as ready and receive an
agent process even though it is not the selected task repository. Before
lifecycle preparation, the orchestrator must either:

1. select a ready environment owned by the requested executor and validate its
   complete repository inventory; or
2. elect one fresh materializer for the requested executor under the existing
   task and repository identities.

The previous environment stays recorded for diagnostics and cleanup policy, but
none of its workspace path, repository rows, worktree identifiers, execution
identifiers, container identifiers, or runtime metadata is forwarded to the new
launch. The durable environment row is rebound to the new executor only after
the target executor has launched successfully, with backend-specific handles
cleared before response values are applied.

The transition does not create a second task and does not silently repair,
delete, move, or rewrite the old workspace.

## Launch admission

For a repo-backed local or worktree execution, readiness requires both durable
inventory validation and filesystem validation:

- the selected environment executor type equals the requested executor type;
- every repository and branch slot has exactly one active canonical inventory
  row;
- the workspace path exists and resolves to a Git working tree whose repository
  identity matches the selected slot; and
- the validated path belongs to the selected environment, not a stale session
  projection.

The canonical inventory match for a slot is scoped per repository. The legacy
empty-branch fallback stands in for a repository's branch slot only when that
repository has no row carrying a non-empty branch slug; branch scoping does not
depend on a worktree identifier, so a local-executor row with a branch and no
worktree ID is already branch-scoped and suppresses the fallback for its own
repository. A slot therefore never matches more than one row.

Validation is read-only and uses the existing bounded Git inspection path. It
does not read file contents or alter the index, HEAD, branch, tracked files, or
untracked files. Remote executors validate through their executor-owned
inventory contract rather than host filesystem inspection.

The [worktree recovery](worktree-metadata-recovery.md) operation can restore a
missing canonical checkout before attachment under its exclusive recovery claim.
Attach-only preparation remains read-only.

The [managed clone relocation](managed-clone-relocation.md) operation can
establish a new canonical worktree before this admission check. It holds its own
environment claim and leaves admission read-only. A failed relocation cannot
make an invalid checkout ready.

A session binding that loses the materializer election is refused with
`workspace_preparing`, which names the elected environment. For a shared group
that environment belongs to another member task. Session preparation waits for
that environment with the same bounded wait the launch path uses for a creating
sibling, then retries the binding once. Before the retry it re-reads the task's
sessions. The repository rechecks primary ownership under the task row lock on
PostgreSQL or the SQLite writer transaction, alongside the first-session origin
and initial runtime seed. This decision is returned on the inserted session;
the caller uses it for primary notifications and does not promote a retry again
after insertion. Concurrent retries for one shared-group member cannot replace
each other's primary or consume the initial runtime seed twice. Explicit manual
primary selection keeps its existing behavior. A failed environment, a wait that ends
without readiness, or an environment owned by the same session keeps the
original refusal, so no session or workspace is created.
Cancellation before or after the wait also keeps the refusal without invoking
the retry. The existing recovery admission remains held through binding and is
released on both successful and failed preparation; retry does not grant a new
recovery authority.

The lifecycle manager retains a defense-in-depth guard: if a repo-backed
`WorkspaceInfo` reaches execution creation without the validated-environment
marker and exact selected environment identity, it refuses to create the
execution. It never treats a non-empty path as sufficient proof.

## Concurrent session visibility

Multiple sessions of one task attach to a single canonical environment and its
single physical worktree. Concurrent writing is permitted by design; this
section adds no lock, no refusal, and no ordering between those writers. It
makes the condition observable.

Two seams start an agent process against an already-attached workspace:
`Executor.LaunchPreparedSession` when it is asked to start the agent, and
`Executor.resumeSession`. Both already hold the per-session lock and both know
the task ID. Immediately before the process starts, each reads the task's
sessions and counts siblings in a working state, excluding the session being
started. The existing `sessionstate.IsWorking` predicate defines that state, so
observation and the executor's own review reconciliation agree on one
definition.

A non-zero count emits a structured warning naming the admitting site, the
session being started, and the sibling session IDs, and increments an expvar
counter under `session_coresidency_*`. Labels carry the admitting site and, for
skips, the reason; identifiers stay in the log entry and never become label
values, so counter cardinality is bounded.

The sibling read can fail. A failed read records a skip with its reason instead
of a zero count, so a degraded observer is distinguishable from a task that
genuinely has one live session. The read never blocks admission: an observation
error is logged and the launch proceeds.

The workflow profile-switch park policy reaches this condition without any user
request. Parking leaves the source session in `WAITING_FOR_INPUT` with its
runtime stopped but its conversation answerable, while the destination step's
session runs. A later message to the parked session starts a second agent in
the shared worktree. That is the path this observation is calibrated for.

## Persistence and projection

After successful launch or resume, the orchestrator refreshes
`task_environments.workspace_path` with the authoritative effective workspace.
No schema migration is required. A successful resume under this contract also
self-heals an environment path written incorrectly by the former fallback
order, provided the canonical worktree inventory remains valid.

Successful resume also persists the session's raw workspace binding after
`persistTaskEnvironment` supplies the final environment identity and path.
`bindSessionToTaskEnvironment` changes an in-memory value. That change requires
a subsequent guarded repository write before resume reports success or starts
the agent process. This applies to agent resume and workspace-only resume.

The credential boundary still persists `STARTING` before credential issuance.
The final binding write uses the current state and startup-attempt guards. If
`LaunchAgent` returns after the same attempt advances from `STARTING` to
`RUNNING` or `WAITING_FOR_INPUT`, persistence may retry against that observed
state only while the captured attempt still owns it. The atomic state and
attempt predicate rejects cancellation, terminal states, and a replaced
attempt. Failure prevents agent startup and uses the existing owned-execution
cleanup and resume rollback path.

The environment-projected path cannot prove that this write occurred.
`GetTaskSessionWorkspacePathsByTaskEnvironment` reads the raw session column
for Git source validation. Verification therefore inspects persisted values
independently of the mutable launch object and effective session projection.
The canonical root and exact active repository worktrees remain the only
eligible paths. Missing raw bindings remain unavailable until an authorized
successful lifecycle operation records them.

Task-session reads project the linked environment workspace before the legacy
session-local path. The Files panel and other workspace consumers use that
effective `workspace_path`, with `worktree_path` retained as the primary
repository and compatibility fallback.

The task is not projected as environment-ready until launch admission passes.
Files, changes, terminal, and agent-start surfaces use only the selected and
validated environment. Cached changes from a rejected workspace may remain in
historical records but cannot become the current task change projection.

## Kubernetes inherited environment admission

The [Kubernetes task-pod design](../../executors/system-design/kubernetes-task-pod.md)
permits independent sessions within one task and deliberately excludes cross-task
pod attachment. A ready parent environment therefore does not prove that a
different task can consume it, even when both select the same Kubernetes profile.

For `inherit_parent` and `shared_group`, resolve the canonical environment and
effective executor before persisting or publishing a destination session. When
the selected runtime is Kubernetes and the environment owner differs from the
destination task, return the typed `workspace_reuse_unsupported` failure required
by `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.4`. Do not rewrite ownership,
allocate replacement compute, switch executors, or change workspace mode.
Missing or inconsistent ownership remains unsafe reuse rather than unsupported
reuse. Existing executor-mismatch admission keeps its own failure category.

Use the common preparation/start admission seam in `internal/orchestrator/` so
workspace-only prepare, ensure, explicit start, and MCP session launch agree.
The shared-group binding transaction checks the selected environment against
the candidate executor and destination owner before saving the session, including
when the group becomes materialized after preflight. Compare known environment
and candidate executor types for every executor. Parent and group propagation
also recheck admission using the prepared session's resolved executor before
binding an environment that became available after preflight; an admission
failure uses the existing session compensation path. Recheck the resolved
binding before lifecycle attachment for already-persisted legacy sessions.
Retain Kubernetes lifecycle's owner/generation checks as the final guard; orchestrator preflight does not authorize foreign resources.

Map unsupported reuse to `CONFLICT` with bounded recoverability details in
`session.launch`, `session.ensure`, and `spawn_session_kandev`. Guidance tells
the caller to create a separate task with `workspace_mode="new_workspace"`
using the intended executor profile. Repeating unchanged inheritance or restoring
the parent workspace cannot make cross-task Kubernetes sharing supported.
Deferred task creation remains successful and keeps its parent-dependent mode
default; this admission applies when a workspace or agent launch is requested.

The browser's admitted `session.ensure` request keeps ownership through
same-task session-list and workflow hydration. Its success or typed rejection
must settle the pending state and reach the existing recovery presentation.
Task identity generations, retry identity, and mount lifetime exclude stale
results. Explicit disable, unknown or archived eligibility, and pending task
removal revoke response ownership and reset the latch. Later re-enablement
admits a fresh request without reviving the previous response. Session-list
and workflow hydration alone do not invalidate the admitted request.

The [repair plan](../../../plans/kubernetes-inherited-workspace-admission/plan.md)
records the regression and paired live reproduction. This is conformance to the
existing unsupported-reuse and task-pod contracts, not a new sharing policy.

## Failure and recovery

- An empty execution workspace and empty worktree path preserve the existing
  source-repository fallback for legacy or non-materialized local sessions.
- A recovered execution workspace always wins over source checkout input,
  including when optional worktree metadata is absent.
- Missing or ambiguous physical inventory fails closed. This path does not
  inspect the filesystem to guess a replacement owner.
- Executor mismatch or invalid workspace admission returns a typed, recoverable
  `workspace_reuse_unsafe` result before agent process startup. The safe
  response identifies the failure category and retry eligibility but does not
  disclose filesystem paths, branch names, credentials, or executor secrets.
- Failure preserves all existing task, session, environment, repository, and
  filesystem objects. A later launch may retry only after the control plane has
  a valid selected environment; it does not perform automatic cleanup or
  fallback to a repository path outside that environment.
- Persistence failure follows the existing launch/resume rollback and task
  environment failure handling.

## Verification

Resume coverage includes an initially empty raw session path and both agent
and workspace-only materialization. Frozen write snapshots and repository
integration checks prove that the final binding reaches persistent storage.
Persistence-error and superseded-attempt cases prove that no agent starts
through a failed final write. Existing credential-boundary checks retain
their ordering assertions.

Desktop and phone Changes regressions resume a disposable failed session,
create a dirty file, reload the task, and verify fresh file membership.
The browser checks use real backend refresh responses. They do not inject a
successful status or rely on a later file mutation to recover the panel.
The [resume binding repair plan](../../../plans/resume-workspace-binding/plan.md)
records implementation scope and exact commands.

The orchestrator unit boundary covers all workspace-path precedence cases,
including workspace-only recovery with a source repository present. A session
resume Playwright scenario uses the Worktree executor, restarts the backend,
allows automatic promotion/resume, and verifies that the Files path remains
the original worktree path.

Executor-transition coverage adds:

- Prove an executor-type transition cannot forward the previous environment
  workspace path or runtime handles.
- Prove a missing and a non-Git local workspace fail before execution creation
  and before agent process startup.
- Prove a valid canonical local repository and a valid worktree attachment keep
  current behavior.
- Prove rejection does not mutate Git state, filesystem contents, environment
  inventory, or the canonical repository.
- Prove current task projections do not select cached changes from the rejected
  workspace.

The Files path component is shared by desktop and phone layouts. This repair
changes backend state normalization, not layout, navigation, scrolling, or
touch behavior, so the focused desktop recovery scenario plus existing shared
component coverage provides mobile parity.
