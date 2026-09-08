# ADR-2026-08-31-explicit-new-branch-session-recovery: Require Explicit User Action Before Continuing a Session on a Replacement Branch

**Status:** accepted
**Date:** 2026-08-31
**Area:** backend, frontend, protocol

## Context

A stopped or failed task session can retain a valid provider resume token after
its Git worktree branch disappears locally and on the configured remote. The
conversation can still contain valuable context, but the commits and files
that existed only on the lost branch cannot be recovered.

Normal resume currently fails when the worktree manager returns
`ErrBranchUnrecoverable`. The web client also hides several resume errors. An
automatic branch switch would preserve the conversation, but it could make the
user believe that the prior code also returned.

The recovery path must keep task-environment worktree ownership, normal branch
naming, and provider session identity. It must distinguish confirmed branch
loss from network, authentication, and transient Git failures.

## Decision

Kandev requires an explicit user action before it continues a session on a
replacement branch.

A normal `resume` first runs the existing worktree recovery checks. If the
error chain matches `worktree.ErrBranchUnrecoverable`, the WebSocket response
contains typed recovery details and the UI offers **Continue on a new branch**.
Kandev does not replace the branch during that failed request.

The new `resume_new_branch` action authorizes replacement only after the same
worktree checks confirm that the branch is unrecoverable. The worktree manager
creates a unique branch from the configured task base branch with the normal
branch template and suffix helpers. It updates the task environment repository
record and then resumes the same `TaskSession` with the stored provider resume
identity.

The action does not clear the resume token. **Start fresh** remains the only
user recovery action that intentionally discards the stored conversation
identity before launch.

After successful replacement, the orchestrator persists an idempotent warning
status message. The message states that conversation history continues and
that code changes from the lost branch were not recovered. Structured metadata
records the old branch, new branch, base branch, session, and repository.

## Consequences

- A user sees the branch-loss cause before any irreversible recovery choice.
- The provider conversation can continue without claiming that lost code was
  restored.
- Recovery requires one additional user action.
- Existing clients can display the descriptive error without reading the
  structured details.
- Task environments remain the single owner of physical worktrees.
- Multi-repository tasks can replace one confirmed lost branch while retaining
  valid worktrees.
- Branch replacement cannot hide authentication, network, or transient remote
  failures.

## Alternatives Considered

- **Automatically create a replacement branch.** Rejected because the user
  could mistake conversation recovery for code recovery.
- **Keep branch loss as a hard failure.** Rejected because a valid provider
  conversation can still contain useful history.
- **Use Start fresh for branch loss.** Rejected because it clears the resume
  identity and loses the provider conversation that this recovery must keep.
- **Restore only a read-only workspace.** Rejected as the only option because
  it does not let the existing conversation continue agent work.
- **Reconstruct commits from session history.** Rejected because conversation
  history is not an authoritative or complete Git object store.
