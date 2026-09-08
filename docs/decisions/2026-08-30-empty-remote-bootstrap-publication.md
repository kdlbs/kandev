# ADR-2026-08-30-empty-remote-bootstrap-publication: Keep Empty Remote Bootstrap Local Until Publication

**Status:** accepted
**Date:** 2026-08-30
**Area:** backend, frontend, protocol, security

## Context

Git can clone a remote repository that has no refs. The resulting checkout has an unborn branch and no commit that can anchor a task worktree.

Kandev previously treated the missing base as a required-refresh error. This behavior stopped the agent before creation of the first project files.

Kandev can create a local baseline commit, but remote publication needs a separate authority. Clone credentials can be read-only, and task launch is not publication consent.

## Decision

Kandev treats an authenticated zero-ref advertisement as an empty-remote state. It does not treat this result as stale local fallback.

Kandev creates a deterministic empty baseline commit and a local marker ref under the repository lock. It creates the task worktree from that local baseline.

Task launch, resume, and worktree recreation never publish the baseline. They do not call a provider mutation API.

An explicit Push or Create PR action can publish the baseline. The action uses task runtime Git credentials and pushes the baseline before the task branch.

The baseline push never uses force. If another actor initializes the remote first, Kandev stops and preserves both histories.

Kandev identifies the baseline through an exact local marker ref. It does not infer bootstrap ownership from commit metadata or tree contents.

## Consequences

- Users can start isolated task worktrees for empty remote repositories.
- Read-only clone access remains sufficient for local task work.
- Remote mutation occurs only after an explicit publication action.
- Push and change-request paths must understand the local marker contract.
- A base push can succeed before a task-branch push fails. Kandev reports this partial result and preserves the task branch.
- Concurrent initializers can create a recoverable history conflict. Kandev never resolves that conflict with force or automatic history changes.
- The decision narrows the zero-ref case in the required-refresh fail-closed rule. Authentication, network, timeout, and missing-branch errors still fail closed.

## Alternatives Considered

### Publish an initial commit during task launch

Rejected. Task launch does not prove write authority or user intent to change the remote.

### Create an orphan task branch without a base

Rejected. The remote still lacks a change-request base. The first feature push can also become the unintended default branch.

### Initialize the remote through each provider API

Rejected. This path duplicates the provider-neutral Git credential boundary. It also excludes pasted Git remotes.

### Keep rejecting empty remotes with a clearer error

Rejected. Kandev can create safe local state and delay the external write until an explicit action.
