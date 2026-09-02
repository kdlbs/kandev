# ADR-2026-08-25-required-worktree-refresh-fails-closed: Required Worktree Refresh Fails Closed

> The local-worktree admission boundary is superseded by
> [ADR-2026-08-31-local-worktree-refresh-best-effort](2026-08-31-local-worktree-refresh-best-effort.md).
> Required remote materialization remains fail-closed when no usable local base exists.
>
> Amended by
> [ADR-2026-08-30-empty-remote-bootstrap-publication](2026-08-30-empty-remote-bootstrap-publication.md):
> an authenticated remote that advertises zero refs uses a marked local baseline.
> Launch still performs no remote mutation.
>
> Amended on 2026-08-31: a pull-request base fetch that proves the requested
> remote base was deleted can use a different configured fallback only after
> that fallback is refreshed successfully. Other required PR refresh failures
> remain fail-closed.

**Status:** accepted (local-worktree boundary superseded by 2026-08-31-local-worktree-refresh-best-effort)
**Date:** 2026-08-25
**Area:** backend, security, operations

## Context

A repository can require Kandev to pull before it creates a worktree. Required
remote materialization and pull-request base refresh must not use an unverified
local fallback after a failed fetch. A successful-looking task could otherwise
edit an old copy of the repository and fail later when it pushes.

Kandev also supports an explicit offline mode. A repository with
pull-before-worktree disabled can use local state without contacting a remote.
The design must preserve that choice and must preserve local-only commits.

Provider-managed credentials and executor-inherited credentials have different
owners. A required refresh must use the route selected by the task Git policy.
It cannot force a managed HTTPS route over an executor-inherited SSH origin.

## Decision

Pull-before-worktree remains best effort for a host worktree with a usable local
base, as defined by the superseding local-worktree decision. Required remote
materialization and pull-request base refresh are admission gates. In those
paths, Kandev starts no agent until the configured remote refresh succeeds and
the selected base ref is verified.

A provider-managed checkout uses the backend's exact-scope credential refresh.
A host or executor checkout uses its reconciled origin and non-interactive Git
environment. A successful provider refresh marks remote sync as handled so the
worktree manager does not run a second unauthenticated fetch.

For a required pull-request base, fetch failure stops preparation except when
Git explicitly proves the requested remote base is absent and a different
configured fallback can be refreshed successfully. This exception records the
substituted base as a warning and never authorizes an unverified local ref.
After a successful fetch, Kandev preserves a local ref only when it contains the
fetched remote ref. It uses the remote ref when the remote contains the local
ref. Diverged refs and failed ancestry checks stop required PR preparation
without changing either ref.

Pull-before-worktree disabled remains the explicit offline opt-out. This path
can use available local refs and makes no freshness guarantee.

Required-refresh errors use the existing durable task launch-error projection.
They identify the affected repository and failure class without exposing
credential material.

## Consequences

- Authentication, SSH, network, and timeout failures become visible before an
  agent starts when remote materialization or PR refresh is required.
- Users do not lose agent time on a checkout that Kandev knows it failed to
  refresh.
- Existing offline workflows continue when pull-before-worktree is disabled.
- Local-only commits remain available when the local branch contains current
  remote state.
- Diverged branches require a user to reconcile history before retrying launch.
- Stacked pull requests can continue after GitHub retargets them away from a
  deleted parent branch, provided the replacement base is refreshed.
- The executor and worktree APIs become fallible at the base-refresh boundary.
- Multi-repository launch stops before runtime startup when one required
  repository cannot refresh.

## Alternatives Considered

### Continue with a local ref and show a warning for required PR refresh

Rejected. A warning does not prevent the agent from producing changes against
known stale state. Host local-only worktrees use the separate best-effort
decision, but required PR refresh remains fail-closed.

### Fail every missing remote base without consulting provider state

Rejected. GitHub can retarget a stacked pull request after its parent branch is
merged and deleted. The pull-request head remains fetchable, and a separately
refreshed current or default base preserves the freshness gate.

### Require remote access for every worktree

Rejected. Kandev supports local and offline repositories. The existing setting
is the explicit user choice between required freshness and local availability.

### Use a setup script or agent prompt as the gate

Rejected. Setup-script failures are nonfatal, and an agent prompt runs after
worktree selection. Neither boundary can prevent launch on stale state.

### Reset the local base to the fetched remote base

Rejected. A reset can discard local-only commits. Kandev can select a safe
containing ref or stop without mutating user history.
