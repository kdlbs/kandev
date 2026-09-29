---
status: current
system: tasks
created: 2026-09-10
updated: 2026-09-29
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-001
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-004
---

# Worktree metadata recovery system design

## Status and ownership

This design describes the compatibility work implemented for PR #3137 and the
missing-checkout extension for issue #4052. The initial reviewed head was
`22c57ef94fa24209855097cbc70ace5a5d2d09a2`. Runtime verification status is
recorded in the linked implementation plans.

The task system owns environment selection, lifetime, and physical inventory.
The worktree manager supplies Git inspection and file recovery. Executor providers
retain authority over their own filesystems.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKTREE-METADATA-RECOVERY-001` | Selected environment admission |
| `REQ-TASKS-WORKTREE-METADATA-RECOVERY-002` | Classification and preservation, Multiple repositories |
| `REQ-TASKS-WORKTREE-METADATA-RECOVERY-003` | Recovery authority, Restart and failure, Response path |
| `REQ-TASKS-WORKTREE-METADATA-RECOVERY-004` | Missing canonical checkout recovery |

## Implemented integration

`prepareSession`, `LaunchPreparedSession`, `resumeSession`, model-switch restart,
and lifecycle workspace creation build admission from the selected environment.
Production wiring calls `Manager.AdmitRecovery` only after effective executor
selection. The legacy task-wide callback remains only for compatibility adapters
and is bypassed when selected-environment admission is installed.

The executor supplies canonical environment repository rows, owner generation,
session identity, and the effective executor type. Empty inventories and every
non-Worktree executor return before host filesystem inspection. A remote origin
does not change this host-filesystem boundary.

`RecoverWorktree` retains the snapshot protocol, validates the recorded branch,
and publishes through the durable environment claim and guarded compare-and-swap
when called by production admission. The executor refreshes the selected
environment rows before lifecycle startup.

## Selected environment admission

Admission moves to the boundary after effective executor and environment selection,
but before workspace reuse or agent startup. Preparation must not mutate all
worktrees of a task before its environment selection is known.

The internal request carries the requesting task and session, physical owner,
environment ID, ownership generation, and effective executor type. Repository slots
come from the selected environment's canonical inventory, not cached session paths.

The resolver honors an explicit session environment and existing shared-workspace
authorization. It does not assume that the requesting task owns an inherited
environment. Executor mismatch keeps the existing executor-transition path.

Admission returns immediately for an empty repository inventory or a non-Worktree
executor. This check precedes host `Lstat`, Git commands, or recovery record access.
A repository's remote URL does not select a remote executor.

The same boundary covers prepared launches, normal resumes, and workspace-only
restoration. The lifecycle reuse path must not bypass it through a cached runtime
or workspace path. Fresh materialization without an existing selected environment
retains its normal path.

## Recovery authority

The task repository owns a durable, environment-scoped recovery claim. This is a
new repository capability. It follows the existing task-row locking order and
ownership-generation contract. A process-local mutex is not the authority.

The proposed claim records environment ID, owner task ID, ownership generation,
requesting session ID, and operation ID. A unique environment key prevents two
operations from holding authority. A replayable migration supports SQLite and
PostgreSQL without changing existing environments.

Claim acquisition runs in one transaction. It locks the owner task and environment,
then rejects active cleanup, ownership mismatch, executor mismatch, or a busy
environment. Busy detection joins sessions by environment ID, including borrowed
sessions from other tasks. It includes every `executors_running` row and other
nonterminal sessions. A WAITING session or idle runtime is not proof of quiescence.

Session attachment, runtime-start reservation, workspace restoration, owner
transfer, reset, and cleanup consult this claim under the same serialization rule.
The claim remains held from before snapshot creation through inventory publication.
It must prevent external runtime startup, not merely its later database update.

Existing session locks and `taskEnvLocks` remain local coordination mechanisms.
`CountActiveWorktreeReferences` is not a busy check: its cross-task reference count
does not cover same-task sessions or runtime lifetime.

Publication requires the claim operation ID and current owner generation, in
addition to the existing exact-slot compare-and-swap. Production stores must
implement this contract. An absent claim capability refuses recovery, rather than
falling back to an unguarded update.

The claim does not reuse `ClaimTaskEnvironmentReset` as a shortcut. Reset claims
belong to destructive cleanup jobs. Recovery must not acquire cleanup semantics
for the original checkout or retained snapshot.

## Classification and preservation

The manager retains bounded Git inspection, managed-root validation, and path
identity checks. It distinguishes healthy metadata, absent checkout, absent Git
administrative directory, and ambiguous metadata.

For a surviving checkout, only an absent administrative directory is eligible
for automatic replacement. Absent checkouts use the separate path defined below.
The `.git` pointer must identify the expected repository's worktree namespace.
Existing administrative directories require valid common-directory and backlink
relationships. Permission errors and malformed pointers are not absence.

The recorded branch must resolve in the correct repository. For a missing
checkout, an absent local branch and absent tracking ref trigger an exact
`origin` probe. A probe failure is an inspection failure; confirmed remote
absence retains the existing explicit branch-loss action. An available remote
branch is fetched to its advertised commit and checked against that commit
before materialization. An empty or lost branch never falls back to
`BaseBranch`.

Recovery retains the PR's manifest-checked snapshot and deterministic sibling
replacement. The root `.git` entry is excluded. Tracked, untracked, and ignored
content, deletions, file modes, and symbolic links remain part of the snapshot.
Symbolic links are copied as links. Unsupported special files stop recovery.

The new branch retains the existing `<branch>-recovered-<operation-prefix>` form.
The original directory and snapshot remain available. Recovery does not restore
the old index or unavailable commits.

## Multiple repositories

Admission first classifies the complete selected inventory without mutation.
An ambiguous slot stops admission before another slot begins replacement.

Under the environment claim, eligible slots recover in stable order. Each
publication targets `(environment ID, repository ID, branch slug)` and its old
worktree identity. No transaction pretends that multiple filesystem operations
are atomic.

If a later recovery fails, earlier successful replacements remain authoritative.
All original checkouts remain. A retry reloads the inventory and reuses completed
slots. Agent startup requires a valid complete inventory.

For one repository, the effective workspace follows the replacement path. For
multiple repositories, the task root remains the workspace and slot paths change.
Runtime requests and current projections must reload this inventory after recovery.

## Restart and failure

The existing adjacent JSON record owns snapshot progress:
`snapshotting`, `rematerializing`, `blocked`, and `complete`. The operating-system
claim file excludes simultaneous file recovery. The database claim owns environment
authority. Both records use the same operation identity.

A restart cannot clear a claim because its timestamp is old. Continuation requires
the same owner generation, no runtime consumers, and exclusive file-claim ownership.
It also requires matching snapshot and replacement identities.

If these conditions fail, the claim blocks continuation and the original data
remains. Cleanup workers must not interpret a recovery claim as a deletion job.
Claim release uses the exact operation and generation. A stale release cannot
remove another operation's authority.

Missing-checkout replay also takes a nonblocking operating-system lock in the
repository's common Git directory, keyed by the canonical worktree ID. Acquire
locks in stable slot order before adopting an existing same-session/same-operation
database claim. Hold them through restoration and the existing external-start
boundary, then release them only with admission release. This distinguishes an
active process from a restarting process even though database claim acquisition
is idempotent for the same operation.

If all operation records are complete but a process stopped before releasing its
database claim, ordinary selected-environment admission takes the matching
operation locks, re-reads and validates each record and checkout, confirms the
exact owner, generation, session, executor, and operation claim, and releases
only that claim. It does not infer staleness from claim age. A still-active
process retains its OS lock and prevents reconciliation.

## Response path

The existing frontend action reaches the orchestrator through its normal request
handler. The executor resolves the environment, performs admission, then supplies
the refreshed workspace inventory to lifecycle and agentctl.

`WorktreeRecoveryError` and existing launch-failure classification report a refusal
through the current response path. No new frontend state store or WebSocket action
is required. Error classification must not offer destructive retry actions for
ambiguous metadata.

Structured diagnostics identify the operation, session, environment, repository
slot, generation, and outcome. They do not include file contents or credentials.
Retained snapshots can contain ignored secrets and require the same protection
as the original workspace.

## Missing canonical checkout recovery

`Manager.AdmitRecovery` owns restoration before attach-only validation.
`reuseRequiredWorktree` stays read-only and continues to reject an absent path
when called without a completed recovery. This extends the eligibility boundary
in [the missing-checkout ADR](../../../decisions/2026-09-29-missing-worktree-checkout-recovery.md).

### Classification and authority

`inspectRecoverySlot` distinguishes a missing canonical checkout from metadata
loss. An `Lstat` absence alone is insufficient. Validate the selected active
row, environment owner and generation, repository identity, recorded branch,
managed task-root identity, and every existing ancestor without following links.
A missing tasks base can be rebuilt only from the persisted managed-root identity.
Legacy paths outside managed roots remain refused by this automatic path.

Preflight covers every selected active slot. It resolves each recorded branch
and inspects conflicting registrations before mutation. Failed or deleted rows
do not authorize a substitute. Existing complete-inventory validation still
requires exactly one active row for every requested repository/branch slot.

A missing slot sets recovery work and acquires the existing durable environment
claim with `AllowCurrentSessionRuntime: false`. No new lock table or schema is
needed. Re-read canonical identities and repeat preflight under the claim.
Hold authority through the existing external-start boundary. A live consumer
from any session or borrowing task blocks restoration, even if its agent is idle.

### Exact restoration

Add a focused missing-checkout operation under `internal/worktree`. Do not call
`recreate` wholesale: that helper can remove paths, prune registrations, refresh
branches, and perform initial-materialization work.

The operation uses the repository lock and pinned no-follow directory handles.
Resolve the recorded local branch to an exact commit first. When only the
recorded origin branch survives but its tracking ref is absent, use a bounded
exact-ref probe and fetch only the advertised commit to a private operation ref.
Verify that fetched head before restoring the same local branch. For an
interrupted remote-only operation, each retry must probe the exact origin branch
again and persist its newly advertised head before fetching. If origin advances
between the probe and fetch, refuse that attempt; the next retry can record and
fetch the latest advertised head. If the local branch exists, keep its recorded
head and do not replace it with a newer remote head. Prefer persisted compaction
recovery identity when applicable. Probe failure is not confirmed branch loss.

Atomically reserve only the recorded absent path through its pinned parent
directory. Pass Git the pinned target identity throughout `worktree add`; do not
resolve the target path again after validation. Never use `worktree remove`,
`--force`, reset, `-B`, base-branch fallback, contribution setup, or copy-file
scripts. A path that appears before reservation blocks creation. Any
administrative-entry cleanup must target only the exact proven stale registration
for this slot. Do not run repository-wide `worktree prune`. A branch checked out
elsewhere, a locked registration, or an ambiguous backlink remains a refusal.

Validate the result against the expected common directory, branch, and captured
commit. Keep the environment row, worktree ID, path, branch slug, and branch name.
Use the existing claim-aware store boundary for any necessary publication.
Do not replace inventory ownership or update rows through an unguarded write.

### Interruption and partial success

Use an operation record adjacent to the checkout, protected like existing recovery
records. Record the claim operation ID, environment generation, exact slot identity,
branch head, and expected path before materialization. Give this operation its own
record format, separate from the surviving-file snapshot protocol. The OS lock
serializes active work and adoption across backend processes.

Git must mutate the target through its pinned directory identity. On platforms
where the descriptor path cannot be a worktree destination, enter the inherited
pinned directory descriptor as the Git process working directory and add `.`
using the repository's exact common directory. Before Git succeeds, failure
cleanup may remove the pinned target only while it is empty. After Git succeeds,
retain it even when later validation fails. Remote operation refs use exact
compare-and-swap deletion; absence is already-clean, and an interrupted replay
retries cleanup after validating the completed checkout and before marking the
record complete.

The admission path must detect an unfinished missing-checkout record before its
healthy-path fast return. On restart, adopt only a matching operation under the
same exclusive claim contract. Validate a completed checkout against its recorded
identity before finishing the operation. Reconcile a completed record left before
claim release only after acquiring its OS lock and revalidating the exact checkout
and claim. Never accept unrelated files, expire claims by age, or discard a
partially written checkout. Ambiguous partial results remain blocked and preserved.
A completed record permits ordinary later attachment.

For multiple slots, use one claim operation ID and stable slot ordering. Preserve
completed slots after a later failure. Retry only unfinished eligible slots.
Existing metadata recovery and clone relocation can share the claim, but their
operation records and eligibility rules remain distinct.

### Integration and compatibility

Existing selected-environment admission covers preparation, prepared launch,
resume, fresh-start preflight, and lifecycle workspace restoration. Reuse its
inventory refresh and claim propagation. Verify each route with the real manager
and a real SQLite claim, not only a recording callback. No new UI state, control,
translation, public API, or provider conversation identity is required.

Local, Docker, SSH, Sprites, Kubernetes, repo-free tasks, and executor transitions
retain their existing behavior. Host recovery applies only to selected Worktree
environments. A source-clone mismatch follows clone-relocation rules or refuses.
It must not make the missing-path shortcut bypass repository identity checks.

Diagnostics identify the operation, slot, and refusal category without file
contents or credentials. Public Git guidance explains that deleted uncommitted
content cannot be reconstructed and that another session cannot bypass a busy claim.

## Related contracts and decisions

- [Additional-session reuse](additional-session-workspace-reuse.md) remains
  read-only. Recovery is a separate guarded operation before that validation.
- [Detached continuity](detached-workspace-continuity.md) owns environment generations.
- [Explicit new-branch recovery](../../../decisions/2026-08-31-explicit-new-branch-session-recovery.md)
  remains authoritative for lost branches.
- [Proposed metadata-recovery boundary](../../../decisions/2026-09-10-worktree-metadata-recovery-boundary.md)
  records the narrower automatic-recovery case.
- [Metadata implementation plan](../../../plans/worktree-metadata-recovery/plan.md)
- [Missing-checkout fix package](../../../plans/missing-worktree-checkout-recovery/plan.md)
