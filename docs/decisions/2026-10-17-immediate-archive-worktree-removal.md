# ADR-2026-10-17-immediate-archive-worktree-removal: Delete managed worktrees on archive regardless of local changes

**Status:** accepted
**Date:** 2026-10-17
**Area:** backend, frontend

## Context

The prior archive policy preserved dirty worktrees and reconsidered them after
24 hours. The completed task-folder cleanup fix therefore still leaves an
archived task folder whenever its checkout has uncommitted work. The user now
explicitly requests removal of that protection and deletion during archive.

The observed task cd7b40b7-03d7-46bc-806e-f8f35b01e0c2 was archived on
2026-10-17 at 22:25:50 +0100. Cleanup logged dirty-worktree preservation and
deferred reclaim. Its checkout contained two modified documents and an
untracked plan directory. This was the prior policy working as designed.

## Decision

Archive authorizes destruction of tracked, untracked, and ignored local work
in eligible managed Git worktrees. Dirtiness is no longer an admission filter,
a mutation-time veto, or a reason for delayed reclamation. Existing retained
archive jobs adopt the new policy through the normal bounded cleanup worker.

The task lifecycle continues owning archive generation, runtime stop ordering,
borrower protection, retries, and final managed-root cleanup. Filesystem and
Git ownership checks, no-follow deletion, branch safety, and unarchive fencing
remain mandatory. Ordinary local repository folders and independent adopted
checkouts do not gain new destruction authority.

This supersedes the dirty-retention and cleanliness-delay portions of
[Archived worktree reclamation](2026-09-24-archived-worktree-reclamation.md)
and dirty-sibling retention in
[Archived task directory cleanup](2026-10-13-archived-task-directory-cleanup.md).
Their lifecycle/storage ownership and filesystem safety decisions remain in
force. Requirement 004 in the
[archive requirements](../specs/tasks/requirements/dirty-worktree-archive.md)
defines the replacement; the
[implementation package](../plans/immediate-archive-worktree-removal/plan.md)
records the completed delivery and verification.

## Consequences

- Archive intentionally loses uncommitted files, including those in older
  archived worktrees retained by the previous version. No automatic commit or
  push is introduced.
- Existing archive confirmation surfaces describe deletion in every supported
  language. The existing preference to skip confirmation remains effective.
- Committed branch recovery and integrated-branch compaction keep their
  existing rules. Task deletion retains its separate discard-consent contract.
- Immediate means part of the archive cleanup attempt after runtime stop;
  ownership conflicts and I/O failures can still require a later retry.

## Alternatives Considered

- **Retain dirty checkouts indefinitely or until clean.** Rejected because the
  explicit user request makes archive responsible for immediate disk cleanup.
- **Add an archive discard toggle or another prompt.** Rejected because the
  requested behavior is the archive default, with the existing confirmation
  preference controlling prompts.
- **Delete task roots by an unchecked recursive path removal.** Rejected
  because deleting another task's resources is outside archive authorization.
