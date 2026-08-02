# ADR-2026-08-02-new-workspace-github-access-defaults: Bootstrap New Workspaces From Host GitHub Access

**Status:** accepted
**Date:** 2026-08-02
**Area:** backend, frontend, security

## Context

New workspaces currently start with managed task Git credentials but no workspace GitHub
connection. Launching a task with an attached GitHub repository therefore asks the credential
broker for a lease that cannot be issued, even when the Kandev host already has an authenticated
`gh` account and usable local Git credentials. The first task on a fresh self-hosted installation
can fail before the agent starts.

The host `gh` account is an operator credential. Kandev already prevents non-admin members from
selecting it, so workspace bootstrapping must preserve that boundary instead of granting the host
identity to every authenticated creator.

## Decision

Every newly created workspace attempts to persist **Inherit executor Git credentials** (`executor`)
as its initial task Git access policy. When that persistence succeeds, Local and Worktree tasks use
host-visible Git or SSH credentials; remote tasks use credentials configured in their executor. If
the settings write fails, workspace creation remains available and the existing `managed`
compatibility fallback applies until the workspace is configured or retried.

When workspace creation is performed by an internal trusted caller, an auth-disabled synthetic
administrator, or a real administrator, Kandev also snapshots the host's active authenticated
`gh` account as a named workspace automation connection. It stores the exact host/login selection,
not a token and not a live reference to whichever account becomes active later. If `gh` is absent,
unauthenticated, or cannot validate the selected account, workspace creation still succeeds with
executor task access and disconnected GitHub automation.

A non-admin member-created workspace receives executor task access but never inherits the server
operator's `gh` account automatically. The member or an administrator must configure an allowed
workspace automation identity explicitly.

The initial workspace seeded for a brand-new database follows the same rule. Upgrades do not
rewrite any existing workspace connection, persisted task policy, or legacy missing/invalid policy
fallback. Existing missing or invalid stored values continue to normalize to `managed` for
backward compatibility.

This decision amends
[ADR-2026-07-27-task-git-credential-policy](2026-07-27-task-git-credential-policy.md) only for the
default assigned to newly created workspaces, and amends
[ADR 0047](0047-github-authentication-ownership.md) only to allow an operator-authorized creation
flow to make an explicit named CLI selection automatically.

## Consequences

- A fresh local installation can launch its first task without configuring Kandev's credential
  broker first.
- Background GitHub automation works immediately when the host `gh` account is authenticated and
  the creator is allowed to use operator credentials.
- Workspace task access remains explicit and persisted; changing the host's active `gh` account
  later does not silently change an existing workspace.
- A settings persistence failure is logged and leaves the workspace on the existing managed
  fallback, so a caller must configure or retry task access before relying on executor inheritance.
- Remote executors do not inherit host credentials merely because the workspace default is
  `executor`; they still require executor-specific credentials.
- Member-created workspaces remain isolated from the server operator's host identity.
- Upgrade behavior is stable, at the cost of different defaults for pre-existing missing settings
  and newly created workspaces.

## Alternatives Considered

### Keep managed task access as the default

Rejected. A new disconnected workspace cannot issue the lease managed mode requires, so ordinary
task creation fails before the agent starts.

### Auto-bind the host account to member-created workspaces

Rejected. It bypasses the existing operator-only CLI binding rule and can expose repositories the
member was not intended to access.

### Resolve the currently active host account on every task launch

Rejected. A later `gh auth switch` would silently change workspace identity and recreate the
cross-workspace ambient-credential coupling that ADR 0047 removed.

### Migrate existing unconfigured workspaces

Rejected. Existing installations may rely on managed routing or deliberate disconnection. The new
defaults apply prospectively without changing saved or legacy behavior.
