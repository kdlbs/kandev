---
status: current
system: tasks
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
created: 2026-09-27
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
