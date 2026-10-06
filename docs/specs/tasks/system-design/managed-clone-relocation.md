---
status: current
system: tasks
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
created: 2026-09-27
updated: 2026-10-04
owners:
  - kandev
---

# Managed clone relocation system design

## Purpose and boundaries

`repositories.local_path` is a mutable source-clone location. It is not proof
that an existing task worktree was created from that clone. The task environment
and its `task_environment_repos` rows own physical worktree identity. This design
adds a guarded relocation phase before the existing read-only launch admission.
It does not weaken the Git common-directory check or change the workspace
system's clone placement.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-MANAGED-CLONE-RELOCATION-001` | Detection, clean relocation, authority, and publication |
| `REQ-TASKS-MANAGED-CLONE-RELOCATION-002` | Dirty relocation, failure, and restart |
| `REQ-TASKS-MANAGED-CLONE-RELOCATION-003` | Error and UI contract |

## Detection and clean relocation

The executor passes the selected task environment and its complete canonical
repository inventory to a relocation admission step before lifecycle launch,
resume, or workspace-only reconstruction. The step runs only for a host Worktree
executor. It compares each recorded worktree's resolved Git common directory
with the current workspace-scoped repository clone. Matching slots do nothing.
An unmatched slot is eligible only when all of these are proven:

1. The recorded worktree ID, path, branch, task environment, and owner
   generation still match the canonical inventory and Git's own worktree list.
2. The prior common directory resolves under Kandev's managed clone root and
   its origin resolves to the same provider, host, owner, and repository as the
   selected repository. A matching name or URL string alone is insufficient.
3. The destination is the current clone of that same workspace repository.
   Neither a foreign workspace clone nor an arbitrary local path is eligible.
4. The recorded branch and exact HEAD commit are available in the source.
   Inspect the complete selected inventory before changing any slot.

The managed source candidates are the provider/origin layout and the older
owner/name layout under the same managed root. Either candidate is accepted
only when the selected worktree's Git common directory and registration match
it, and its origin proves the exact provider, host, owner, and repository.
After a worktree has moved to the destination, admission verifies that
destination first and no longer depends on the retained source clone existing.

For automatic relocation, inspect Git status including ignored and untracked
files and verify the checkout is clean. Refuse unsupported checkout modes,
including sparse checkout, submodules, and encrypted worktree content, until
their preservation has explicit proof. Fetch the recorded branch and exact
commit from the verified local source into the destination clone using the
classified Git subprocess path. Create a sibling worktree at the exact commit.
Do not reset or alter the original worktree. Revalidate branch and HEAD before
publishing the replacement. The ordinary admission check then validates the
new worktree against the current source clone before the agent can start.

## Unchanged legacy source admission

For AC-TASKS-MANAGED-CLONE-RELOCATION-001.5 and .001.6, distinguish a layout
candidate from the repository's registered source. The computed workspace path
alone does not establish that a source-clone change occurred.

Before requiring `ExpectedDestinationPath` to exist, the shared worktree
inspection checks whether `Worktree.RepositoryPath` resolves to either exact
legacy candidate in `ManagedCloneRelocationProof` (`ExpectedSourcePath` or
`LegacyOwnerNameSourcePath`). Resolve existing paths, require containment under
the managed root, verify the provider origin, and require the checkout's actual
Git common directory to be that clone's `.git`. Keep ordinary linked-worktree
registration, branch, and ownership validation in place. Main-checkout admission
uses the same source selection and directory identity rule, retaining ordinary
main-checkout HEAD validation, including valid detached commits. Only linked
worktrees require the persisted branch registration proof. Cancellation and
operational inspection errors propagate without falling back to reuse.

This verified match is unchanged-source reuse. Do not require, create, or inspect
the computed workspace destination, acquire a relocation claim, rewrite database
paths, or run transfer-only filter/submodule/cleanliness checks. Preserve the
normal credentials policy. Canonicalize filesystem paths without treating every
case-insensitive string match as the same directory on case-sensitive systems.

If that exact legacy match is absent, keep the existing destination validation
and relocation path, including refusal when the registered workspace destination
is missing. Never substitute a legacy clone for a missing current destination.
Every selected slot must pass: an unchanged legacy slot cannot hide a foreign,
missing, or relocation-required sibling. GitHub and GitLab share this proof;
other provider types retain their existing handling.

This restores the existing source-change boundary in the relocation ADR; it
introduces no repository migration or new credential ownership rule. The repair
of a live installation is not an implementation template for automatic recovery.

## Authority and publication

Reuse `task_environment_recovery_claims`, the owner-generation fence, the
worktree manager's filesystem claim, and the exact-slot compare-and-swap from
[worktree metadata recovery](worktree-metadata-recovery.md). This is a separate
operation kind and recovery record. It must not be mistaken for missing Git
metadata or destructive cleanup. A live runtime, competing launch, restore,
cleanup, or ownership transfer prevents relocation. Existing runtime consumers
continue using their original checkout until they stop naturally.

Publish one replacement only after its Git proof passes. In a multi-repository
task, preflight every selected slot first, process eligible slots in stable
order, reload inventory after each publication, and start no agent until every
slot validates. Earlier completed replacements remain authoritative if a later
slot fails. Originals and snapshots remain. The current task root remains the
multi-repository workspace. A single-repository task follows its replacement
worktree path.

New worktree materialization records source-clone identity alongside the
physical slot in `task_environment_repos`. Add a nullable source path and
common-directory identity through an idempotent migration. Legacy rows are
resolved lazily only by the authenticated Git and ownership proof above.
Never infer a legacy source from `repositories.local_path` after it has moved.
The repository path update remains allowed for future tasks, and the existing
task slot retains its own physical source identity until relocation publishes.

## Dirty relocation

A dirty checkout does not relocate during an ordinary Resume or Start fresh.
Classify the mismatch before clearing a provider resume token. Return a typed
`managed_clone_relocation_required` failure with an operation stamp. The
explicit `relocate_and_resume` action rechecks the stamp, authorization,
selected environment, owner generation, source and destination identities, and
busy state. A prior refusal is never authorization by itself.

After the claim, reuse the bounded, manifest-checked snapshot and sibling
replacement machinery from metadata recovery. Import the exact branch/commit
from the verified source, create the replacement, and copy tracked, untracked,
and ignored content without following symlinks. Preserve deletions, modes, and
file bytes. The original checkout and snapshot remain. Index staging choices
are not reconstructed. The confirmation and completion message state this
limit. Unsupported special files, unreadable entries, conflicts, and changed
snapshots stop before publication. Do not silently fall back to a remote or
task base branch when the exact commit is unavailable.

## Permission-only blocked snapshot continuation

This extension implements AC-TASKS-MANAGED-CLONE-RELOCATION-002.5. It uses the
[shared snapshot mode policy](worktree-metadata-recovery.md#snapshot-permission-preservation)
and the [retry boundary decision](../../../decisions/2026-10-04-permission-only-snapshot-retry.md).
Implementation and verification are recorded in the
[permission fix package](../../../plans/workspace-recovery-permissions/plan.md).

Ordinary resume, restore, fresh start, and automatic metadata recovery must not
reopen a blocked snapshot. A new `relocate_and_resume` request supplies the
existing current error stamp and authorization. No new UI control, action,
runtime flag, session state, or database table is required.

`recoveryOperationIDsForSlot` can recognize a provisional retry candidate only
for explicit dirty relocation. The adjacent recovery record must have `blocked`
state, an empty manifest, and the historical reason
`recovery snapshot does not match original checkout`. Its task, worktree,
original path, and valid operation ID must match the canonical slot and the
managed-clone relocation record. The latter must still be `materialized`.
The reason string is a format discriminator, never sufficient authorization.
Read-only candidate recognition cannot change records or release claims.
Merge operation IDs through the existing selected-inventory rule.
Conflicting operations remain refused.

After acquiring the existing durable environment claim and both filesystem
operation locks, reread the records and current failure authorization.
Require unchanged owner generation, session binding, complete canonical
inventory, verified source/destination clones, recorded branch, and exact HEAD.
Require the recorded replacement to remain a clean, unpublished worktree at
its captured branch and commit. Refuse edits to that replacement.
Generic `adoptRecoveryRecord` must continue to reject blocked records.
The specialized dirty-transfer entry owns the retry proof under the same
snapshot lock. It must not release the lock between proof and transition.

Compare the original and failed snapshot through pinned no-follow directory
handles. Exclude only the checkout root `.git`. Require identical relative
entry sets, entry types, regular-file bytes, and symlink targets. Permit only
special bits absent from the failed snapshot and ordinary permission bits
removed from regular files. Directory ordinary permissions must match.
No snapshot entry can add permissions. At least one permission difference
must exist. Historical copied ownership does not authorize source ownership.
Read the required set-ID identity from the current verified original.
Repeated source checks include mode and required UID/GID to detect drift.
Unreadable entries, special files, path substitution, cancellation, content
changes, and unrelated reasons refuse without changing either retained copy.
Stream comparison with bounded memory, existing I/O deadlines, and cancellation.
Do not add an arbitrary file-count limit that rejects normal build trees.

Add one optional `mode_retry` object to `recoveryRecord`. It records format
version 1, the previous snapshot path, previous error and timestamp, and a
new deterministic path: `<original>.kandev-recovery-<operation-id>-modes-v1`.
The record retains its existing operation ID. Atomically replace the blocked
record with `snapshotting` and this retry provenance only after all proofs pass.
Keep the old snapshot untouched. Reject an occupied new path unless the durable
retry record already owns it. A crash before the record write leaves the old
blocked record. A crash afterward follows the existing snapshotting protocol,
which can rebuild only the new incomplete snapshot. Never remove the old copy.

`prepareRecoverySnapshot` then creates a fresh snapshot from the original.
Keep full manifest checks, the set-ID identity checks, restoration, and the
claim-aware inventory compare-and-swap. Resume the existing deterministic
replacement rather than creating another branch or operation. A second blocked
failure with `mode_retry` present does not authorize another automatic rebuild.
Rematerializing retries retain their existing verified manifest rule.
Completed and unrelated blocked records retain their existing treatment.

Every selected slot must pass preflight before any record transition.
Successful earlier slots remain authoritative after a later failure.
No agent starts until the complete inventory validates. A successful relaunch
retires only its matching error stamp and preserves the provider resume token.
A stale or failed request leaves the current actionable error visible.
Record reason categories through the existing sanitized failure and relocation
outcome paths. Never log file bytes, UID/GID details, or absolute paths in public
errors. No install-wide startup scan or claim expiry is added.

## Inspection contention and explicit preflight

This section defines admission for AC-TASKS-MANAGED-CLONE-RELOCATION-001.1,
.001.3, .002.1, and .003.1. The implementation is recorded in the
[convergence package](../../../plans/managed-clone-recovery-convergence/plan.md).

Ordinary Resume reaches `PreflightSessionWorktreeRecovery` before the lifecycle
singleflight. Its inspection can collide with Files, Changes, or commit requests
that reconstruct workspace access inside that flight. An occupied inspection
mutex does not establish checkout corruption.

`RecoveryAdmissionRequest` has a separate inspection-wait policy. Only manual
recovery preflight outside lifecycle execution creation selects it. This policy
grants no dirty-relocation permission. `RelocateDirty`, operation stamps, and
durable claims retain their current meanings. Lifecycle launch and workspace
reconstruction return immediate lock refusal. They do not wait for an admission
that can later join their own singleflight.

Manual preflight waits for at most 15 seconds, or the caller's remaining
deadline, whichever is shorter. It uses cancellation-aware acquisition in
stable slot order and releases earlier locks on cancellation or timeout.
The existing 30-second ordinary recovery client deadline remains unchanged.
Capture the selected session binding, environment owner and generation, and the
canonical membership of every active environment-repository row before waiting.
Include unmaterialized slots and their registered repository identity and path.
Replacement-session preparation can occur before the session row is inserted.
Record that session as absent and verify it remains absent after the wait; still
bind its intended session identity to the captured environment. Existing sessions
must remain persisted with the same environment binding.
After acquiring the original worktree locks, reread this complete snapshot and
reject any drift before inspection or mutation. Do not add a newly discovered
slot to the waiting operation; it requires a new admission with its own locks.
Then re-resolve the original canonical worktree slots and verify their branch,
path, source, and repository identity.

Represent inspection contention with a distinct typed internal error.
Ordinary background callers fail promptly. A manual wait that expires returns
a bounded, path-free conflict response and leaves its existing error active.
Do not report metadata corruption, evict a claim, stop a runtime, or authorize
transfer because a mutex is occupied. An acquired mutex does not replace
the existing durable claim and liveness checks.

## Durable workspace recovery error projection

This section implements AC-TASKS-MANAGED-CLONE-RELOCATION-002.1, .002.4,
and .003.1 through the existing session error contract.

Dirty relocation refusal can originate in manual resume preflight,
`launchRestoreWorkspace`, or lifecycle workspace reconstruction used by background
read requests. All three paths must converge on the same durable session error.
Keep filesystem classification in the worktree manager. Keep session persistence
and event publication in the task service. Lifecycle must not import that service.

Provide a narrow injected lifecycle reporter for verified
`ManagedCloneRelocationRequiredError` results. The task service supplies it during
backend composition. Capture session, selected environment, owner generation,
the complete canonical active repository inventory, state, execution identity,
and current error stamp before workspace inspection.
Manual preflight and restore use the same task-service projection operation.
The reporter does not inspect unrelated environments or retry Git operations.

Reuse `last_agent_error`, `managed_clone_relocation_required`,
`relocate_and_resume`, and the existing stamp contract. Record no new lifecycle
state and add no schema or runtime flag. A CANCELLED session remains CANCELLED.
An absent typed error, including a legacy `error_message` alone, is a supported input.

Use a conditional repository write to preserve successor state. Extend the
existing metadata compare-and-swap machinery only where its predicates are
insufficient. Match the captured state, execution identity, selected environment,
owner generation, complete selected slot membership, each worktree identity and
path, and the registered repository identity and path in the write transaction.
Use the task-before-environment and slot/repository row-locking conventions used
by environment inventory mutation. Inventory drift defeats both a new write and
reuse of an active relocation stamp.
Do not reuse bootstrap failure settlement, which changes a session to FAILED.
If a newer error, resumed execution, or ownership change wins, publish nothing.
Repeated detections for the same unchanged inventory reuse the active relocation
stamp and do not append another history entry or notification.

Publish `TaskSessionErrorChanged` after a successful write, using the existing
status-summary rebuild path. Preserve unrelated metadata, retained error history,
provider resume tokens, and task-scoped errors. A persistence failure returns
a sanitized error without an actionable stamp or agent startup. Event-publication
failure follows existing logging policy; reload still reads the durable record.

`wsLaunchSession` must preserve relocation conflict details for restore failures,
as `wsRecoverSession` already does. The restore action hook must consume those
details with its existing operation fence. A late restore response must not
replace a newer card or create a second recovery surface.

The current projection, response, reload, and reconnect must expose the same
stamp and eligible action. Inspect every selected slot before publishing recovery
success. Retire only the matching error after relocation and relaunch succeed.
For mixed inventories, an unchanged healthy slot cannot hide a dirty mismatch
or an invalid sibling. Refusal leaves all original checkouts available.

## Failure and restart

The durable operation record holds a stable ID, selected environment and owner
generation, slot identity, source/destination Git identities, branch, HEAD,
snapshot state, and replacement path. A restart resumes only the same operation
under the same claim and unchanged proofs, or refuses with both worktrees
retained. Claim release is fenced by operation ID and generation. The error
stays active until every selected slot validates and the relaunch succeeds.
Neither a cancelled session nor a fresh agent profile bypasses the check.
Publication also re-reads the current repository path so a second clone move
cannot redirect an in-flight operation.

## Security

The legacy clone is a read-only object source for this operation. Relocation
does not refresh it, change its origin, or write credentials to its Git config.
The destination uses the current workspace credential scope. Snapshots remain
private to the task owner and retain the existing no-follow file protections.
No origin string, path, file content, or credential enters public error details.

## Error and UI contract

The backend exposes a stable category and bounded reason, without absolute
paths, credentials, Git URLs, or cross-workspace metadata. The recovery action
is offered only for a verified relocatable dirty checkout. Ambiguous or
unverifiable sources receive guidance to preserve files and seek manual repair.
The branch-loss recovery action remains distinct. The shared recovery model
suppresses Resume, Start fresh, and Restore read-only workspace for this
specific mismatch until relocation completes.

Reuse the existing desktop session recovery card and phone recovery view. The
phone view keeps one vertical scroll owner and a visible full-width action.
The explicit confirmation states the staging limit before submission. Both
viewports use the same hook and action state, with localized copy in all
supported locales. The existing phone recovery card is the closest mobile
exemplar. The confirmation uses a desktop dialog and a phone inset drawer.
Focus returns to the recovery card on cancel or failure.

## Verification and observability

Backend tests reproduce two clones of one provider repository, a worktree
linked to the old clone, and a repository row pointed to the new clone. Cover
clean automatic relocation, dirty refusal and explicit recovery, unpushed
commits, ignored files, symlinks, multi-repository partial failure, wrong
origin, foreign workspace, busy runtime, competing claim, restart, and stale
publication. Check original bytes, branch, HEAD, index, and inventory before
and after refusal. Desktop and mobile Playwright prove the typed failure,
correct action set, confirmation, and successful continuation.

Emit a bounded relocation outcome counter by closed-set reason and structured
logs with task/environment IDs only where existing diagnostic policy permits.
Never include file contents, credentials, or absolute paths in UI errors or
metric labels.

## Related decisions

- [Managed clone relocation boundary](../../../decisions/2026-09-27-managed-clone-relocation-boundary.md)
- [Worktree metadata recovery boundary](../../../decisions/2026-09-10-worktree-metadata-recovery-boundary.md)

## Implementation plans

- [Snapshot permissions and blocked retry](../../../plans/workspace-recovery-permissions/plan.md)

- [Original relocation package](../../../plans/managed-clone-relocation/plan.md)
- [Unchanged legacy clone admission](../../../plans/legacy-clone-resume/plan.md)
- [Resume and workspace recovery convergence](../../../plans/managed-clone-recovery-convergence/plan.md)
