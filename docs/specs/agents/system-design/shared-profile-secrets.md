---
status: current
system: agents
requirements:
  - REQ-AGENTS-SHARED-PROFILE-SECRET-001
  - REQ-AGENTS-SHARED-PROFILE-SECRET-002
---

# Shared agent profile secrets system design

## Purpose and boundaries

This design adds an owner grant to agent-profile secret bindings. A secret
owner opts one binding into shared use, and launch-time resolution honors that
grant under fixed account checks. The design reuses existing parts and adds no
new mechanism for them:

- `authn.Identity` and the auth service's active-account lookup;
- the secrets store's per-user scoping;
- the strict and non-strict environment resolution in the lifecycle manager.

It does not change who may edit or start a profile, private secret visibility,
or executor-profile secrets. The delivery record is the
[implementation plan](../../../plans/shared-profile-account-boundaries/plan.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-SHARED-PROFILE-SECRET-001 | Persistence; Grant writes; API projection |
| REQ-AGENTS-SHARED-PROFILE-SECRET-002 | Owner-bound store reads; Launch resolution; Failure behavior |

## Persistence

`task/models.ProfileEnvVar` gains `SharedByUserID`
(`json:"shared_by_user_id,omitempty"`). Agent and executor profiles store
environment entries as a JSON column, so no schema migration is needed:

- older rows decode with an empty grant;
- older binaries ignore the field.

The grant belongs to one binding, identified by its environment key and secret
ID. It is never a profile-wide or user-wide flag.

## Owner-bound store reads

`secrets.OwnerGrantedSecretStore` (`internal/secrets/owner_grant.go`) adds
`GetGlobalOwnedBy` and `RevealGlobalOwnedBy`. Both are implemented by the
SQLite store and by `UserVisibleStore`.

- **Match condition.** A read matches only `WHERE id = ? AND user_id = ?` with
  a stored scope of `global`.
- **Never matched.** A blank owner, a blank ID, and internal secret IDs never
  match.
- **No authorization.** These methods ignore the request identity and do not
  authorize. Callers must have completed every grant check first.
- **Failure result.** A mismatch returns `secrets.ErrNotFound`, so a foreign,
  workspace, unowned, or deleted secret looks the same as a missing one.

## Grant writes

`agent/settings/controller` resolves grants in `CreateProfile` and
`UpdateProfile` through `resolveProfileSecretGrants` and
`resolveProfileSecretGrant`. Before that, the existing global-secret checks run
for every binding the grants do not cover. Each binding is resolved as follows:

| Request `shared` | Previous grant on same key+secret | Result |
| --- | --- | --- |
| `false` | any | no grant |
| omitted | none | no grant |
| omitted | grantor G | keep G after re-verifying G owns the global secret |
| `true` | grantor G | keep G after re-verifying G owns the global secret |
| `true` | none | caller must be a signed-in, non-synthetic user who owns the global secret; record the caller |
| `true` on a literal value | n/a | reject |

`verifyProfileSecretGrant` calls `GetGlobalOwnedBy` and returns fixed text that
names neither the secret nor its owner. `profileSecretGrants.apply` writes
grants only onto bindings whose key and secret still match, and clears forged
or stale `SharedByUserID` values on every other entry.

Other write paths:

- **Duplicate.** `cloneEnvVars` drops all grants.
- **Nested create.** `validateCreateProfileRequest` calls
  `rejectProfileSecretShares` to reject `shared: true` on profiles created
  inside a new-agent request.
- **Executor profiles.** In `task/service`, `CreateExecutorProfile` and
  `UpdateExecutorProfile` reject any grant with
  `ErrExecutorProfileSecretShareUnsupported`.

## API projection

`dto.ProfileEnvVarDTO` gains `Shared *bool` (`json:"shared,omitempty"`), and
`envVarsToDTO` sets it to `true` for a granted binding. The grantor ID and the
secret value are never serialized. The current web settings form omits the
field, so a save from the UI keeps existing grants.

## Launch resolution

`backendapp.wireSharedProfileSecretGrantor` wires `auth.Service.IdentityForUser`
into `lifecycle.Manager.SetSharedProfileSecretGrantorResolver`. That lookup
returns an identity only when all of these hold:

- auth is enabled;
- the user exists and is active;
- the user's organization is usable.

`Manager.revealAgentProfileSecret` (`shared_profile_secret.go`) runs these steps
in order:

1. Run the starter's normal scoped reveal (`revealGlobalSecret`). If it
   succeeds, if the binding has no grant, or if the error is anything other
   than `ErrNotFound`, return that result.
2. Require an authenticated, non-synthetic starter with a user ID.
3. Require both an `OwnerGrantedSecretStore` and a wired resolver.
4. Resolve the grantor. Require an active, non-synthetic identity whose user ID
   equals the recorded grantor.
5. Require the grantor and the starter to be in the same organization.
6. Call `RevealGlobalOwnedBy(secretID, grantor)`.

Two resolution paths use this function:

- **Non-strict path.** `resolveAgentProfileEnvVarsWithPartial` passes each
  binding's own grant. When `partial` is set, a denial drops only that key.
- **Strict path.** `buildEnvForExecution` uses `environmentRevealer`. It
  indexes agent-profile grants by key and secret, and routes a definition
  through the grant only when it has exactly that key and secret and no
  workspace ID. Every other definition uses `resolveEnvironmentDefinition`.
  Executor-profile entries pass through `withoutSecretShares` before
  resolution, so an executor binding never inherits an agent-profile grant.

Synthetic auth-disabled identities take step 1 with unscoped visibility, as
before. No grant is consulted for them.

These consumers call scoped `RevealSecret` directly and never consult grants,
so they fail closed for non-owners:

- the agent discovery probe;
- the GitHub credential bridge;
- the coordinator containment checker.

## Failure behavior

Grant denials return `*SharedProfileSecretError`, which reads "shared agent
profile secret is unavailable: <reason>". The reason is a fixed phrase for one
of these cases:

- unauthenticated starter;
- unsupported backend;
- inactive or mismatched grantor;
- different organization;
- grantor no longer owns a global secret for the binding.

How the reason reaches the user:

- **Strict path.** `agentProfileSecretError` appends the reason (or, for an
  ungranted `ErrNotFound`, the "missing, not global, or owned by another user"
  hint) and prefixes the profile name.
- **Non-strict path.** `profileSecretErrorWithHint` applies the same hint.

No message contains the secret's ID, value, or owner. Store and
context-cancellation errors propagate unchanged, so cancellation stays
observable.

## Security considerations

- **Explicit consent.** A grant represents the owner's consent. Other members
  can keep an existing grant on an unchanged binding, but cannot create one,
  move one to another key or secret, or revive one after the owner deletes the
  secret.
- **No escalation.** A grant does not widen any read path other than this
  binding. It confers no admin or owner role, and it does not impersonate the
  owner: the launch still runs as the starter, and only the one value is
  resolved by owner.
- **Value disclosure.** Granting is disclosure. The value enters every
  authorized starter's agent environment, and anyone who can edit the profile's
  other launch settings can influence that process. Owners should share only
  secrets intended for every profile starter.

## Related contracts

- [Per-user executor ownership](../../executors/system-design/per-user-ownership.md)
- [Prepared launch actor](../../executors/system-design/prepared-launch-actor.md)
