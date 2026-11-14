---
status: active
system: agents
created: 2026-11-08
owners:
  - kandev
---

# Shared agent profile secrets requirements

## Overview

[Issue #5167](https://github.com/kdlbs/kandev/issues/5167) reports that, with
authentication enabled, an agent profile is shared by every member while
its global secret bindings resolve only for the user who owns each secret.
When a different signed-in member starts a task with the shared profile, the
launch fails before the first prompt with an unactionable profile-secret lookup
error. The workaround was to route every start through the owning account.

The agent system owns this contract because it owns agent profile data and the
environment those profiles contribute to a launch. Identity, organizations, and
account state remain owned by [auth](../../auth/README.md). Secret storage and
private secret visibility are unchanged.

## Terms

- **Binding:** One agent-profile environment entry that injects a global secret
  (by secret ID) under one environment key.
- **Secret owner:** The user that a global secret is stored under.
- **Owner grant:** The secret owner's explicit consent, recorded on one binding,
  that any authorized starter of the profile may resolve that binding.
- **Starter:** The authenticated identity a launch runs as. This can be a human
  session or an agent using a personal access token of that user.

## Requirements

### REQ-AGENTS-SHARED-PROFILE-SECRET-001: Owner grant on a binding

**Intent:** A secret owner can explicitly allow a shared profile to use one of
their global secrets, and only for that binding.

#### Acceptance criteria

- **AC-AGENTS-SHARED-PROFILE-SECRET-001.1:** When a signed-in user creates or
  updates an agent profile with `shared: true` on a binding, the system shall
  record a grant only if that user owns the referenced global secret. Otherwise
  the system shall reject the request and persist nothing. This applies to any
  other member, an auth-disabled synthetic identity, and an internal caller
  without identity.
- **AC-AGENTS-SHARED-PROFILE-SECRET-001.2:** When `shared: true` is sent on a
  literal value, or on a profile created together with a new agent, the system
  shall reject the request.
- **AC-AGENTS-SHARED-PROFILE-SECRET-001.3:** When a profile update omits
  `shared` on a binding whose key and secret are unchanged, the system shall
  keep the existing grant. This holds when another member edits the profile.
  When the owner sends `shared: false`, the system shall remove the grant.
- **AC-AGENTS-SHARED-PROFILE-SECRET-001.4:** When a binding's key or secret
  changes, the system shall not carry the grant to the new binding. Each
  retained grant shall be re-verified against current ownership of the secret.
- **AC-AGENTS-SHARED-PROFILE-SECRET-001.5:** When a profile is duplicated, the
  copy shall carry no grants.
- **AC-AGENTS-SHARED-PROFILE-SECRET-001.6:** Profile responses shall report
  `shared: true` on a granted binding. They shall never expose the grantor's
  identity or the secret value.
- **AC-AGENTS-SHARED-PROFILE-SECRET-001.7:** When an executor profile is saved
  with a grant on any environment entry, the system shall reject the save.

### REQ-AGENTS-SHARED-PROFILE-SECRET-002: Launch-time resolution

**Intent:** Authorized starters can use a granted binding. Every other use
fails closed before any runtime start, with a reason that reveals nothing
about the secret.

#### Acceptance criteria

- **AC-AGENTS-SHARED-PROFILE-SECRET-002.1:** When a starter resolves a binding,
  the system shall first apply the starter's own secret visibility. The grant
  shall be considered only when the secret is not visible to the starter.
- **AC-AGENTS-SHARED-PROFILE-SECRET-002.2:** The system shall resolve a granted
  binding only when all of the following hold:
  - the starter is an authenticated, non-synthetic user;
  - the grantor is an active, non-synthetic account in the starter's
    organization;
  - the grantor still owns the secret, and the secret is global.
- **AC-AGENTS-SHARED-PROFILE-SECRET-002.3:** When any condition in .2 fails, the
  system shall fail the strict launch before the agent process starts. The
  error shall name the profile and give a fixed, actionable reason. It shall
  not contain the secret's ID, value, or owner.
- **AC-AGENTS-SHARED-PROFILE-SECRET-002.4:** A grant shall apply only to its
  exact key and secret. Another environment definition that names the same
  secret, such as one from an executor profile, shall resolve with the
  starter's own visibility only.
- **AC-AGENTS-SHARED-PROFILE-SECRET-002.5:** When a non-strict profile resolution
  denies a granted binding, the system shall drop only that entry.
- **AC-AGENTS-SHARED-PROFILE-SECRET-002.6:** Resolution shall honor caller
  cancellation.
- **AC-AGENTS-SHARED-PROFILE-SECRET-002.7:** With authentication disabled, the
  synthetic identity's unscoped secret resolution shall be unchanged.
- **AC-AGENTS-SHARED-PROFILE-SECRET-002.8:** When a binding without a grant
  fails because the secret is not visible to the starter, the error shall say
  that the secret is missing, not global, or owned by another user. It shall
  also say that the owner can enable shared use for the binding.

## Exclusions

- **No implicit ownership.** No global, admin, or service-account ownership is
  inferred, and a configuration manager cannot share another user's secret.
- **No widening of scope.** Grants cover global secrets only. Grants across
  organizations, per-user allowlists, and grants on workspace secrets are out
  of scope.
- **Executor profiles.** Executor-profile secret bindings remain the
  launching user's own.
- **Other secret consumers.** Agent discovery probes, the GitHub credential
  bridge, and coordinator containment checks keep scoped resolution and do not
  honor grants.
- **No per-account copies.** Copying profiles or secrets per account is not
  provided.
- **API only.** There is no web UI control in this delivery; `shared` is
  set through the profile API. The settings UI omits the field, which keeps
  existing grants.
- **No confidentiality from starters.** A grant does not keep the value
  confidential from starters: the value enters the agent process environment
  of every authorized starter.

## Delivery

[Implementation plan](../../../plans/shared-profile-account-boundaries/plan.md).
