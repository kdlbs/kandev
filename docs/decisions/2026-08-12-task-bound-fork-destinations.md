# ADR-2026-08-12-task-bound-fork-destinations: Bind Fork Push Destinations to Tasks

**Status:** accepted
**Date:** 2026-08-12
**Area:** backend, frontend, workflow, security, GitHub

## Context

A contribution task created against a canonical GitHub repository has two repository identities before
a pull request exists. The canonical repository owns issues, the future pull request, the base branch,
and Kandev's repository row. A contributor-owned fork is only the destination for publishing the task
branch.

Replacing the repository row or `origin` with the fork makes issue lookup and pull-request detection use
the wrong repository. Keeping the canonical identity without recording the fork makes managed task Git
credentials reject the push, because their helper and `gh` shim accept only repository identities covered
by task-bound leases. Switching the workspace to executor-owned credentials would make the flow work, but
it can expose a host credential with access to unrelated repositories.

Kandev already solves the equivalent post-PR problem with a server-authored `remote_contribution`
binding. That binding keeps the target repository as `origin`, adds a dedicated source remote, and permits
one exact additional managed credential scope. A pre-PR contribution needs the same trust shape without
claiming that a pull request, source branch, or head SHA already exists.

Managed credential leases are fixed when the task runtime starts. Preparing a fork only when the task
enters its PR workflow step is therefore too late for a long-lived managed session unless the whole
environment is recreated. Managed fork resolution must complete before the first task launch.

## Decision

Kandev will represent a pre-PR fork as a versioned, server-authored
`contribution_destination` binding on the canonical task-repository attachment. The binding contains only
the provider, the exact credential-free source repository identity and URL, and provider-owned stable IDs.
The attachment's normal `repository_id` remains the target identity. No token, lease, provider title, or
caller-supplied Git remote is trusted or persisted.

When the workspace selects managed task credentials, the Improve Kandev task-creation path resolves the
destination through the selected workspace automation connection before the task is settled or its first
agent starts. Direct target write access requires no second binding. Otherwise Kandev reuses or creates the
automation actor's fork, verifies that its parent is exactly `kdlbs/kandev`, verifies write access, and
persists the binding. A same-name repository with a different parent fails closed.

Fork ownership must match the identity that supplies managed task credentials. A human PAT or named GitHub
CLI automation connection may own and create a fork. A GitHub App may contribute directly when its
installation can write the target; otherwise automatic fork preparation fails with configuration guidance.
Kandev does not create a fork with a personal connection and then attempt to push it with an unrelated App
installation.

Runtime materialization preserves canonical `origin` and adds a collision-resistant dedicated remote for
the bound fork. The task branch tracks that remote for ordinary `git push`; pull, base comparison, issue
lookup, and pull-request creation continue to target the canonical repository explicitly. Managed GitHub
credentials add a second lease only when its owner/repository exactly matches a valid destination binding
on the same task attachment. The broker authorizer accepts that scope alongside the attachment's canonical
identity and rejects every other repository.

The managed Improve Kandev PR path does not run `gh repo fork` or rename `origin`. It pushes `HEAD` through
the configured contribution remote and creates the pull request with explicit canonical `--repo`, base,
and fork-owner/head arguments. Launch, resume, and managed-origin reconciliation reconstruct this
arrangement idempotently.

Executor-owned task credentials remain an explicit unmanaged compatibility path. Kandev does not infer or
persist a fork from an opaque local, SSH, container, or cloud executor identity. The PR workflow may retain
its existing agent-managed fork setup only when no managed destination is present and the workspace policy
is executor-owned; it must never use that fallback after a managed preparation failure.

## Consequences

- GitHub issue and pull-request integration continue to use `kdlbs/kandev`.
- Managed mode exposes at most the canonical repository plus one server-verified fork to the task, instead
  of the executor's ambient GitHub authority.
- Fork resolution or creation becomes required synchronous task-creation work for Improve Kandev
  implementation tasks that use managed credentials. Failure leaves no partly launched task.
- The binding and remote setup must be projected through every supported executor and reconstructed on
  resume.
- Bootstrap capability reporting must use the workspace automation connection, not ambient host `gh`, so
  the displayed actor and the eventual managed credential source cannot disagree in managed mode. The
  executor-owned compatibility probe remains explicitly separate.
- Existing `remote_contribution` bindings remain the authority for tasks attached to an already-open pull
  request. The new binding covers only the period before a pull request exists.
- User-managed local repository rows retain their existing remote exemption. Kandev-managed provider rows
  continue to reconcile `origin` to canonical HTTPS.

## Alternatives considered

### Replace `origin` with the contributor fork

Rejected. It makes the fork look canonical and breaks issue lookup, pull-request detection, base fetches,
and managed-checkout reconciliation.

### Allow any fork of the canonical repository at credential time

Rejected. Discovering a relationship at lease redemption is less deterministic, makes authorization depend
on a live provider query, and can silently broaden a task after creation. The exact destination must be
server-verified and task-bound before launch.

### Prepare the fork when entering the PR step

Rejected. Existing sessions keep their launch-time credential snapshot. A context reset does not reliably
rebuild the task environment or issue new leases.

### Attach the fork as a second task repository

Rejected. The fork is not a second workspace source. Materializing it independently would duplicate the
checkout and confuse repository ownership, diffs, and change-request association.

### Require executor-owned credentials for fork contributors

Rejected. It restores functionality by abandoning the managed trust boundary and may expose credentials
for unrelated personal or work repositories.

### Pre-bind an executor-owned fork from workspace automation

Rejected. Executor credentials can belong to a different host or account and are intentionally opaque to
Kandev. A workspace-authored fork would not prove that the executor can write it.
