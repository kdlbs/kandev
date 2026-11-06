# ADR-2026-10-13-archived-task-directory-cleanup: Remove archived task containers through task lifecycle

**Status:** accepted
**Date:** 2026-10-13
**Area:** backend

## Policy replacement

[Immediate archive removal](2026-10-17-immediate-archive-worktree-removal.md)
replaces this decision's dirty-checkout retention policy. Lifecycle ownership,
borrower protection, and filesystem identity boundaries remain applicable. The text below records the original
choice and its rationale.

## Context

Archive removes managed Git worktrees but only attempts to remove their parent
task directory when empty. Agent-created files beside a checkout leave the
task directory behind despite a successful archive cleanup job. The user
expects that directory and its contents to disappear after worktree cleanup.

The existing archive contract preserves dirty checkouts and active borrowers.
Individual worktree removal also serves partial cleanup and other lifecycle
operations. Making its parent cleanup recursive would delete protected siblings.

## Decision

Task archive owns a final recursive removal of its managed task directory once
every contained recorded worktree has been reclaimed and the existing runtime,
ownership, archive identity, and shared-environment guards permit destruction.
The same final step runs when delayed reclamation removes the last retained
worktree. Directory cleanup errors remain a durable cleanup obligation.

Individual worktree removal keeps its empty-only parent cleanup. Recursive
removal uses the existing pinned, no-follow directory primitives, and never
follows a root replacement or an internal symlink to another location.

This supersedes the separate-storage-policy consequence in
[Archived worktree reclamation](2026-09-24-archived-worktree-reclamation.md)
for contents of an admitted archived task root. Unrelated orphan roots retain
the separate storage policy.

## Consequences

- Agent-created files beside managed checkouts no longer prevent completed
  archive cleanup from reclaiming the directory.
- A dirty or shared checkout continues to protect its whole task root until
  safe reclamation; clean siblings can still be removed independently.
- Cleanup must retain root identity and retry evidence after the last physical
  worktree is removed, including across restart and unarchive races.
- The archive documentation must explain removal of files beside checkouts.

## Alternatives Considered

- **Recursively delete the parent after each worktree removal.** Rejected
  because it can delete another repository or branch still owned by the task.
- **Leave these files to optional storage maintenance.** Rejected because
  archive would continue to leave the directory behind when files are present.
- **Delete only a list of known generated files.** Rejected because agent-created
  names and nested directories are not a closed inventory.
