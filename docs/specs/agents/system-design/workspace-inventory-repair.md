---
status: current
system: agents
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008
---

# Preserved workspace inventory repair system design

## Purpose and boundaries

This design repairs a reusable task environment's durable
`task_environment_repos` inventory when it no longer matches the
repository/branch slots required by session resume or a fresh
additional-session launch, without ever touching the preserved checkout.

The orchestrator/executor owns admission (`validateReuseEnvironmentInventory`),
candidate selection, and preservation evidence. The task repository owns the
repair transaction, the idempotency ledger, and the append-only receipt. Both
the explicit session-recovery path and the ordinary fresh/additional-session
launch path share the same guarded repair function; neither ever deletes,
cleans, resets, reseeds, rematerializes, or broadly discovers a checkout.

## Admission guard

`validateReuseEnvironmentInventory` remains the admission guard for every
resume and additional-session launch. Zero rows and non-empty mismatches both
return `ErrWorkspaceReuseUnsafe`; neither state falls through to materialize a
new checkout.

A canonical row's `branch_slug` records the branch identity requested when the
slot was materialized, usually the base branch. A later `checkout_branch` on
the task repository, such as the branch restored by unarchive, names the
branch the preserved worktree already holds. Before matching, the guard binds
such a request to the canonical row whose `worktree_branch` equals the
requested checkout branch, because Git admits a branch in only one worktree.
The binding applies only to worktree launches without an exact slot match,
requires exactly one active holder in the same repository whose adopted
identity matches that row alone, and refuses a row another requested slot
already matches. The adopted slot then drives worktree reuse; the checkout
branch is unchanged.

The explicit recovery request names only a task, session, action, and
idempotency key. The orchestrator authorizes the task/session pair before any
environment, repository, runtime, or filesystem read. It then derives the
required repository slots from canonical task-repository records and resolves
the environment from the session. Caller-provided paths, repository IDs,
branches, and environment IDs are not accepted.

Explicit inventory repair bypasses the generic filesystem-recovery preflight.
Its executor path proves and attests the preserved checkout before normal
worktree recovery admission; clone relocation and missing-checkout restoration
cannot run before that proof. Other recovery actions retain their preflight,
including the error-stamp fence for managed-clone relocation.

## Candidate selection and checkout proof

For one mismatched slot, the executor builds a candidate from an existing stale
environment row, the target session's `executors_running` snapshot, or exactly
one checkout identity retained by a prior session bound to the same task and
environment. The prior-session fallback joins server-owned task-session and
runtime records; it does not discover paths from the filesystem. Local worktree
recovery validates all of these facts:

- task, workspace, environment, repository, and session ownership agree;
- the candidate path is the canonical path owned by the environment and is not
  a symlink;
- `git worktree list --porcelain`, the checkout's common Git directory, HEAD,
  symbolic branch, and repository path are reciprocal;
- the expected branch slot and observed branch/ref identity agree;
- no competing session or runtime claims a live writer. A durable environment
  recovery claim covers the owning task and every inherited/shared-group
  borrower, including a terminal session whose executor remains live. Session
  attachment/start, runtime writes, cleanup, and ownership changes use the same
  database admission guard. The executor retains the claim through proof,
  metadata repair, and post-repair attestation, then releases it even on failure
  or cancellation. Repair and attestation transactions validate claim authority.
  An interrupted process leaves the durable claim in place and fails closed.

Inspection disables Git fsmonitor and optional index writes for every Git
command. Status does not recurse into submodules, and configured external
clean/process filters are rejected before status can execute them. Repositories
requiring those filters need manual recovery; changing conversion semantics
would invalidate the status proof.

The inspector captures HEAD, ref containment, porcelain status counts and
hash, a bounded content hash over tracked, untracked, and ignored files using
typed, length-delimited path/mode/payload entries, a staged-index hash over
every index entry's path/mode/blob identity,
executor state, and source record revisions. The staged-index hash exists
because the content hash alone reads only working-tree bytes: a low-level
index write (for example `git update-index --cacheinfo`) can repoint a
path's staged blob without ever touching the working-tree file, which the
content hash and the porcelain status can both miss. The inspection itself
never mutates `.git/index`, including its own status call's normal
opportunistic index refresh. Host paths remain internal; public
receipts expose only a path hash.

Executor/runtime evidence is read through the authoritative runtime store. A
lookup failure or a runtime record whose task/session identity does not match
fails closed before either the repair transaction or a recovered attestation
write. A new session's typed not-found result is the expected proof that it has
no runtime yet; in the zero-row case, the unique prior environment-bound runtime
that supplies checkout identity is retained in the preservation receipt.

## Repair transaction and idempotency

`workspace_inventory_recovery_receipts` appends each receipt row as an audit and
idempotency record. Its post-repair evidence, match result, and verification
timestamp are monotonic attestation updates on that immutable receipt identity.
The repair transaction locks and rechecks the source rows,
uses their expected revisions, writes exactly one environment-repository row,
and appends one receipt. A unique `(task_id, idempotency_key)` identity returns
the receipt when its request hash matches and conflicts otherwise. The task is
the receipt's lifecycle owner; session and environment identifiers remain
immutable audit facts rather than cascading foreign keys, so routine lifecycle
deletion and the legacy environment-ownership cutover cannot erase receipts.
On PostgreSQL the task row lock (`SELECT ... FOR UPDATE`, a no-op under
SQLite's single-writer serialization) is taken before the existing-receipt
check, not after: locking second would let two concurrent transactions for
the same idempotency key both observe "no receipt exists" before either
commits, so the loser's insert collides with the winner's already-committed
row and surfaces as an occupied-slot conflict instead of a deduplicated
success.

An explicit repair request validates its key even when inventory is already
valid. A same-idempotency-key retry is resolved before candidate selection runs:
the executor looks up an existing receipt for `(task_id, idempotency_key)`
first. When one exists, the executor reconstructs the stable request identity
from the current server-owned task, session, environment, repository slot, and
repaired row while retaining the receipt's original preservation snapshot. It
returns the receipt only when that reconstructed request hash matches, including
on a retry issued after the canonical inventory now matches (the ordinary case
once a prior repair already committed), which would otherwise make candidate
selection see zero provable mismatches and misreport a conflict. A changed
session, environment, repository slot, branch identity, worktree identity, or
ambiguous current row is a genuine idempotency-key reuse and conflicts. An
existing valid slot, stale revisions, or any ambiguous proof returns a typed
result without changing inventory. A request-hash match here only short-circuits
*candidate selection* — it is not itself a launch success. The matched receipt
still passes through the durable post-repair attestation gate described next
(completing it now when a prior crash or failed write left it unattested)
before this retry can be handed back as launchable.

## Before-and-after checkout attestation

The receipt's `preservation` field is the before-repair evidence, captured
immediately before the repair transaction and persisted as part of the same
receipt row. After the transaction commits, the executor re-inspects the same
candidate checkout and persists that result as the receipt's post-repair
attestation (`post_repair_evidence`, `post_repair_matched`,
`post_repair_verified_at`) via a dedicated repository call, whether the
checkout still matches or not. A mismatch is recorded before the repair call
returns its conflict error, so an unexpected concurrent write during a
metadata-only repair is itself part of the durable audit trail rather than a
transient in-memory check that leaves no trace. Persisting the post-repair
attestation is required before any launch can use the repaired inventory. A
failure to persist it is logged and leaves the already-committed row/receipt
retryable, but the repair result is not launchable until a later retry durably
records positive matching evidence. Attestation persistence is monotonic for
divergence: once a negative observation is durable, a concurrent or later
positive observation cannot overwrite it.

This attestation gate is not scoped to the session or idempotency key that
performed the repair: launch admission for a row that already canonically
matches (no mismatch left for `validateReuseEnvironmentInventory` to find) is
looked up by the row's own `environment_repo_id`, independent of which
session or idempotency key produced it. A session-scoped lookup alone would
only ever find a receipt the *current* session's own repair attempt
produced, so a different session observing a row an earlier session repaired
could otherwise launch without the earlier repair's post-repair attestation
ever being checked — exactly the case where that other repair committed but
crashed, or its attestation write itself failed, before attestation landed.
Any session reaching the already-valid branch instead completes (or is
blocked by, if already negative) the same durable attestation a same-session
retry already requires, before it is ever handed back as an admitted launch.
Before accepting that row-scoped receipt, admission also binds it back to the
environment owner's task, workspace, environment, owner task-repository,
repository, branch slot, worktree ID, and canonical path hash. Observed branch/ref
identity follows the completed-repair lifecycle below. A recorded
positive result is launchable only when its durable post-repair evidence exists
and exactly matches the receipt's pre-repair checkout evidence; incomplete or
divergent positive records are conflicts rather than deduplicated successes.
Both entry points that can reach the already-valid branch are covered
end-to-end, through the real `LaunchPreparedSession` and resume call sites
(not only the attestation helper directly), with a genuinely different
session ID than the one whose repair crashed: a fresh/additional-session
launch retried by a different session, and a resume of a different session
after an earlier launch's repair committed but crashed before attestation.

## Branch continuity after completed repair

This extension implements AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.11 and
008.12. Its delivery status is tracked in the
[fix package](../../../plans/attested-worktree-branch-continuity/plan.md).
The [branch continuity decision](../../../decisions/2026-10-23-attested-worktree-branch-continuity.md)
distinguishes repair evidence from current checkout identity.

`workspaceInventoryReceiptCheckoutMatches` checks physical checkout identity
and the internal consistency of the historical branch/ref. The branch continuity
helper requires the original branch while a repair lacks completed positive
attestation. After repair succeeds, the current checkout proof permits a later
branch change. Live Git branch observations use
`UpdateTaskSessionWorktreeBranch`, `UpdateTaskSessionWorktreeBranchByRepository`,
or `UpdateTaskSessionWorktreeBranchByWorktree` to update the current inventory.
These operations must not rewrite historical repair receipts.

Keep two separate proofs at the row-scoped admission boundary:

1. Historical repair proof: the original receipt has a verification timestamp,
   a positive result, and complete post-repair evidence matching its original
   preservation evidence. Validate this proof before considering branch drift.
2. Current checkout proof: the environment owner's task, workspace, environment,
   task-repository slot, repository, branch slot, worktree ID, and canonical path
   still match. Prove the current branch/ref against the actual checkout and
   reciprocal Git registration, including when the row still names the original
   branch. A matching historical branch cannot substitute for current proof.

Only a completed positive receipt can use the changed-branch path. Retain the
full preservation attestation and strict historical comparisons for unfinished
attestation. Negative or inconsistent receipts never use current observations
to replace their historical result. Explicit same-key repair retries retain
the original request-hash contract; a changed derived repair request still
conflicts. Ordinary Resume and additional-session admission are separate from
replaying an explicit repair request.

The completed-repair checkout proof uses a narrow executor helper beside
`executor_workspace_inventory_recovery.go`. Reuse existing session launch
serialization, canonical inventory validation, and normal runtime admission.
This path only reads the completed receipt and current checkout. It does not
repair metadata or complete attestation, so it adds no environment-wide claim
and does not prohibit an otherwise valid shared session. Existing claims still
protect actual repair and incomplete attestation.

Use `validateWorkspaceInventoryRepairCandidate` and
`worktree.InspectCheckoutIdentity` for the selected current row. This inspector
reuses the callback-safe reciprocal Git identity checks from full preservation
inspection. It does not scan checkout content, status, or the staged index.
Full preservation inspection remains required for repair and unfinished
attestation.
Compare its current worktree ID and path hash with the historical physical
identity, and its branch/ref with the current canonical row. Do not compare
current HEAD, content, index, or status hashes with the repair-time values:
normal work after a completed repair can change them. Preserve all existing
root ownership, source common-directory, repository, explicit branch selection,
and runtime admission checks. Unsupported proof shapes fail closed.

All required rows must pass. The shared row gate covers
`admitResumeWorkspaceInventory` and `admitLaunchWorkspaceInventory`, including
prepared-workspace launches and inherited borrowers. A historical receipt is
returned unchanged after successful admission. Do not persist a synthetic
receipt, delete an old receipt, mutate the workspace, or clear delivery blocks.
No schema migration or offline metadata edit is needed for existing positive
receipts. A second, independent delivery failure can still block Resume.

Regression coverage must exercise actual branch switches and renames, current
branch disagreement (including a stale original branch), unreadable ignored
content, unchanged receipt bytes, dirty/staged/untracked files,
missing and negative attestations, owner/borrower scope, competing consumers,
and a mixed multi-repository inventory. Exercise Resume and
`LaunchPreparedSession` through real Git and SQLite fixtures with a stubbed
agent launcher. A predicate-only unit test is insufficient.

## Fresh and additional-session launch integration

`LaunchPreparedSession` (the path used by a brand-new session, including
`spawn_session_kandev`, on-entry auto-start, and the "New Session" launch)
runs the same admission guard as resume. On an `ErrWorkspaceReuseUnsafe`
failure while workspace reuse is required, it attempts the same guarded
repair function with a deterministic, session-scoped idempotency key (derived
from the session ID) rather than a caller-supplied one: there is no session
identity for the caller to key on until the session row already exists, and
the session row is created before this launch path runs. Repair failure
propagates the original admission error unchanged (no regression versus the
pre-repair behavior). Repair success re-validates once, then proceeds through
the single existing launch attempt; the session-scoped `sessionLock` already
serializes concurrent launches for that session, and the repair function's own
durable environment claim rejects competing writers across all borrowing tasks. No
primary-session flag is set or changed by this path: `SetSessionPrimary` runs
earlier, at session creation, independent of whether the launch that follows
repairs or launches cleanly.

## Related decisions

- [Preserve checkouts during inventory repair](../../../decisions/2026-09-01-preserve-checkouts-during-inventory-repair.md)
