# ADR-2026-08-25-required-worktree-refresh-fails-closed: Required Worktree Refresh Fails Closed

**Status:** accepted
**Date:** 2026-08-25
**Area:** backend, security, operations

## Context

A repository can require Kandev to pull before it creates a worktree. The
current worktree manager treats fetch and pull failures as warnings. It then
uses a local fallback ref and starts the agent. An authentication or network
failure can therefore produce a successful-looking task that edits an old copy
of the repository. The later push fails, after the agent has already spent time
on work based on stale state.

Kandev also supports an explicit offline mode. A repository with
pull-before-worktree disabled can use local state without contacting a remote.
The design must preserve that choice and must preserve local-only commits.

Provider-managed credentials and executor-inherited credentials have different
owners. A required refresh must use the route selected by the task Git policy.
It cannot force a managed HTTPS route over an executor-inherited SSH origin.

## Decision

Pull-before-worktree is an admission gate for every new or recreated worktree.
When the setting is enabled, Kandev starts no agent until the configured remote
refresh succeeds and the selected base ref is proven to include the fetched
remote base.

A provider-managed checkout uses the backend's exact-scope credential refresh.
A host or executor checkout uses its reconciled origin and non-interactive Git
environment. A successful provider refresh marks remote sync as handled so the
worktree manager does not run a second unauthenticated fetch.

Fetch failure stops preparation. After a successful fetch, Kandev preserves a
local ref only when it contains the fetched remote ref. It uses the remote ref
when the remote contains the local ref. Diverged refs and failed ancestry
checks stop preparation without changing either ref.

Pull-before-worktree disabled remains the explicit offline opt-out. This path
can use available local refs and makes no freshness guarantee.

Required-refresh errors use the existing durable task launch-error projection.
They identify the affected repository and failure class without exposing
credential material.

## Consequences

- Authentication, SSH, network, and timeout failures become visible before an
  agent starts.
- Users do not lose agent time on a checkout that Kandev knows it failed to
  refresh.
- Existing offline workflows continue when pull-before-worktree is disabled.
- Local-only commits remain available when the local branch contains current
  remote state.
- Diverged branches require a user to reconcile history before retrying launch.
- The executor and worktree APIs become fallible at the base-refresh boundary.
- Multi-repository launch stops before runtime startup when one required
  repository cannot refresh.

## Alternatives Considered

### Continue with a local ref and show a warning

Rejected. A warning does not prevent the agent from producing changes against
known stale state. The failure can remain hidden until push.

### Require remote access for every worktree

Rejected. Kandev supports local and offline repositories. The existing setting
is the explicit user choice between required freshness and local availability.

### Use a setup script or agent prompt as the gate

Rejected. Setup-script failures are nonfatal, and an agent prompt runs after
worktree selection. Neither boundary can prevent launch on stale state.

### Reset the local base to the fetched remote base

Rejected. A reset can discard local-only commits. Kandev can select a safe
containing ref or stop without mutating user history.

