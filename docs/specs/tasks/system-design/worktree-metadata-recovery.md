---
status: current
system: tasks
created: 2026-09-10
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-001
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-002
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
---

# Worktree metadata recovery system design

## Status and ownership

This design describes the compatibility work implemented for PR #3137. The
initial reviewed head was `22c57ef94fa24209855097cbc70ace5a5d2d09a2`.
Runtime verification status is recorded in the linked implementation plan.

The task system owns environment selection, lifetime, and physical inventory.
The worktree manager supplies Git inspection and file recovery. Executor providers
retain authority over their own filesystems.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-WORKTREE-METADATA-RECOVERY-001` | Selected environment admission |
| `REQ-TASKS-WORKTREE-METADATA-RECOVERY-002` | Classification and preservation, Multiple repositories |
| `REQ-TASKS-WORKTREE-METADATA-RECOVERY-003` | Recovery authority, Restart and failure, Response path |

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

Only an absent administrative directory is eligible for automatic replacement.
The `.git` pointer must identify the expected repository's worktree namespace.
Existing administrative directories require valid common-directory and backlink
relationships. Permission errors and malformed pointers are not absence.

The recorded branch must resolve in the correct repository. An empty or lost
recorded branch does not fall back to `BaseBranch`. The separate explicit
branch-loss flow remains authoritative for unavailable code.

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

## Related contracts and decisions

- [Additional-session reuse](additional-session-workspace-reuse.md) remains
  read-only. Recovery is a separate guarded operation before that validation.
- [Detached continuity](detached-workspace-continuity.md) owns environment generations.
- [Explicit new-branch recovery](../../../decisions/2026-08-31-explicit-new-branch-session-recovery.md)
  remains authoritative for lost branches.
- [Proposed metadata-recovery boundary](../../../decisions/2026-09-10-worktree-metadata-recovery-boundary.md)
  records the narrower automatic-recovery case.
- [Implementation plan](../../../plans/worktree-metadata-recovery/plan.md)
