# ADR-2026-11-12-scope-ssh-reclamation-to-deleted-storage: Scope SSH reclamation to the storage being removed

**Status:** accepted
**Date:** 2026-11-12
**Area:** backend
**Amends:** [Opt-in SSH task-directory reclamation](2026-08-24-opt-in-ssh-task-directory-reclamation.md), specifically its ignored-file policy and linked-worktree safety scope.

## Context

[Issue #5295](https://github.com/kdlbs/kandev/issues/5295) reports that SSH
reclamation preserves every task directory in a shared-worktree setup.
Ignored build output triggers the dirty check. Unpublished branches and shared
stashes block unrelated worktrees. Recursive removal also leaves stale registrations.

Disposable Git 2.47.3 fixtures reproduced each cause against the command forms
in checkout `94e35cd2234608185643c91e9f1aeebf1f1956ca`.
The existing safety policy deliberately includes ignored files and every local
branch. The repair therefore changes that policy rather than correcting an
implementation that violates it.

## Decision

The reclamation opt-in treats all Git-ignored content as disposable. This rule
includes ignored secrets and runtime data. Tracked changes and non-ignored
untracked files still block removal.

Safety scope follows the storage that reclamation removes. A verified linked
worktree whose common repository survives uses its own `HEAD` for publication
checks. It uses the stash's recorded branch name for stash checks.
An unknown stash description, or any stash with detached `HEAD`, requires
preservation. A standalone repository retains checks across all branches and stashes.

A standalone clone with additional registered worktrees is preserved. Linked
worktrees need common Git storage outside the entire task directory. Other
layouts, including a repository nested below the primary or direct-child
checkout layout, are preserved rather than requiring a general
repository-dependency model. Discovery checks `.git` entries at every depth and
rejects deeper repositories before mutation.
Narrow checks require Git's linked-worktree identity, not merely a `.git` file.
Known worktree locks and all registered submodules cause safety skips before
mutation. No lock override or submodule teardown belongs in this correction.

Git removes linked worktrees and their registrations without force. Recursive
removal is available only for a verified standalone target after required child
removals succeed. No Git error permits a recursive-removal fallback or broad prune.
Git owns the registration update. Real-Git tests prove its final state without
an additional production verification or captured-identity revalidation protocol.

The original profile opt-in, task ownership, terminal-only trigger, cleanup-hook
ordering, pinned connection, and cleanup-job outcome contracts remain applicable.
The [fix package](../plans/ssh-worktree-task-dir-reclamation/plan.md) tracks implementation.

## Consequences

Shared worktrees can release ignored build output without unrelated branch or
stash activity blocking every task. Standalone clones retain protection for
repository data that disappears with the directory.

Existing enabled profiles receive the ignored-file policy change. Operators must
keep required ignored files outside reclaimable task directories.
Stash descriptions contain branch names rather than durable worktree identities.
Custom descriptions and detached checkouts can therefore cause conservative skips.
Default SSH preparation creates a standalone primary checkout and can reuse an
existing checkout. Linked worktrees come from custom preparation or previously prepared
repositories. Physical path resolution and the required Git command capabilities
must succeed before reclamation proceeds.

Old orphan directories and arbitrary stale registrations receive no automatic
sweep. Unknown `probe_failed` causes in the issue remain separate diagnostic work.

## Alternatives Considered

- **Keep ignored files as blockers:** preserves ignored secrets, but defeats the
  requested reclamation policy for routine build output.
- **Check only `HEAD` for every checkout:** risks removal of unpublished branches
  stored inside standalone repositories.
- **Ignore every shared stash:** permits removal despite a stash explicitly tied
  to the task branch, contrary to the requested behavior.
- **Add persistent stash ownership records:** cannot attribute existing user
  stashes and adds a new lifecycle outside this repair.
- **Remove directories, then prune all worktrees:** can change unrelated stale
  registrations in the shared repository. Git supports targeted removal instead.
- **Force Git removal:** overrides locks and weakens the final clean-tree guard.
  The reproduction removed ignored-only worktrees without force.

## References

- [Git worktree removal and shared refs](https://git-scm.com/docs/git-worktree).
- [Git stash descriptions and storage](https://git-scm.com/docs/git-stash).
- [Git directory discovery](https://git-scm.com/docs/git-rev-parse).
- [Owning requirements](../specs/executors/requirements/remote-task-directory-reclamation.md).
- [Owning system design](../specs/executors/system-design/remote-task-directory-reclamation.md).
