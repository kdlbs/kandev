# ADR-2026-11-26-automation-credential-authority: Automation credential authority

**Status:** proposed
**Date:** 2026-11-26
**Area:** backend

## Context

An enabled definition can reference a secret the automation creator cannot resolve.
A shared profile can refer to a Global secret that belongs to its author.
A repository can bind a Workspace secret that the store also associates with one user.
The repository contract supplies the binding reference to every attached task.
The runtime checks the launch account's authority before it supplies the value.
The v0.103.0 reproduction confirmed both conditions with two authenticated accounts.

A credential that enters an automation environment is available to its executable work.
The grant therefore needs an explicit recipient and a defined runtime boundary.
Profile and repository settings have a wider scope than one automation.
Related work targets shared profiles and ordinary tasks. [Issue #5167](https://github.com/kdlbs/kandev/issues/5167), [draft PR #5184](https://github.com/kdlbs/kandev/pull/5184).

## Decision

Use the active automation creator for credential checks, including unattended launches.
Check the current workspace and executor permissions before checking credentials.
Give a creator a way to choose their own Global secret for one automation.
Limit a Workspace grant to one automation, its fixed creator, and one existing repository binding.
Require the current workspace owner and the secret owner to be the same account.
Agent and executor profiles keep Global secret references. The repository owns the Workspace reference.
Keep grant administration and runtime use as distinct actions.

The first release applies credential choices to a new task in automation history.
The access restriction applies to these runs. It is part of the credential authority contract.
The creator has interactive access to an environment that uses a credential choice.
The workspace owner can stop that run. Shared history supplies its status.
Workspace-wide execution and secret reveal use their existing permission checks.

## Consequences

The editor can report an unusable configuration before enablement.
Profiles and repository bindings can serve several users with different credential choices.
Grant revocation affects future admission. An admitted process can retain its environment until it stops.
Existing definitions get no bindings or grants during migration.
The automation system needs a durable run identity and a current credential check before environment admission.

## Alternatives considered

- Use the workspace owner's identity. This gives workspace membership authority over the owner's unrelated credentials.
- Resolve repository-bound Workspace secrets for every workspace member with `session.prompt`. That would grant use for existing bindings without an owner's approval of the automation.
- Allow Workspace grants on profile fields. That adds a new Workspace source to an instance-shared profile path. The first release grants use at existing repository bindings.
- Grant use on the shared profile itself. This has a broader lifetime and scope than a grant for one automation. Draft PR #5184 explores that contract.
- Validate only at save. A change to membership, profiles, or grants can occur between save and launch.
- Require duplicate profiles for each creator. This requires shared configuration maintenance and gives each copy an independent lifecycle.

## References

- [Automation credentials requirements](../specs/workspaces/requirements/automation-credentials.md)
- [Automation credentials design](../specs/workspaces/system-design/automation-credentials.md)
- [Repository secrets requirements](../specs/workspaces/requirements/repository-secrets.md)
- [Repository secret scopes](2026-08-03-scope-and-merge-repository-secrets.md)
- [Private executor admission](2026-10-15-private-executor-admission.md)
- [Profile authentication admission](2026-11-08-profile-auth-admission-evidence.md)
