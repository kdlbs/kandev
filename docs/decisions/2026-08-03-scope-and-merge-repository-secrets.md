# ADR-2026-08-03: Scope and Merge Repository Secrets

**Status:** accepted
**Date:** 2026-08-03
**Area:** backend, frontend, security, protocol

## Context

Kandev currently stores user-managed secrets in one encrypted catalog. With authentication enabled,
that catalog is user-owned and visible across the owner's workspaces; with authentication disabled,
it behaves as an install-wide catalog. Agent and executor profiles can refer to those secrets by ID.
Repository records are already workspace-owned, but they cannot declare environment credentials.

A task can attach multiple repository rows and select an executor profile. Simply appending each
repository's environment values would make the result depend on repository order and could silently
replace an executor credential. Copying workspace credentials into global profiles would weaken the
workspace ownership boundary. Remote runtimes add another constraint: SSH deliberately forwards a
small credential allowlist instead of copying the control-plane process environment.

The product needs repository defaults that are inherited by every task attaching that repository,
including multi-repository tasks, while keeping secret values encrypted at rest and producing one
predictable task environment for setup scripts, agents, shells, and terminals.

## Decision

### Two user-visible secret scopes

User-managed secrets have an immutable scope:

- **Global** means available across all workspaces visible to the current Kandev user. When
  authentication is disabled this is effectively install-global. Existing user-managed secrets are
  migrated to Global.
- **Workspace** means available only to repositories in one named workspace. Creating, listing,
  revealing, updating, or deleting one requires access to that workspace. Deleting the workspace
  deletes its workspace-scoped secrets.

Instance-shared consumers may reference Global secrets only. This includes executor profiles and
agent profiles; allowing either shared profile type to retain a workspace secret ID would create a
cross-workspace capability. Repository bindings may reference either a Global secret owned by the
caller or a Workspace secret belonging to the repository's own workspace.

Backend-owned integration credentials remain internal, hidden, and outside the user-selectable
scope contract. Their current deterministic IDs and service-specific ownership are not converted
into repository credentials.

### Repository-owned, secret-only bindings

A repository owns an ordered-independent set of bindings:

```text
environment key -> secret ID
```

Bindings contain no literal values. They are stored in a normalized
`repository_secret_bindings` relation with one key per repository. Repository create/update treats
the submitted set as an authoritative replacement in the same transaction as the repository
mutation.

The binding relation has a cascading repository foreign key but intentionally does not have a
secret foreign key. Secrets are hard-deleted today; preserving the secret ID allows a deleted or
otherwise unavailable secret to remain a visible broken binding and makes the next launch fail
closed. Cascading the binding would silently remove required authority, while restricting secret
deletion would prevent the requested deletion behavior.

Repository environment keys use the same POSIX identifier, length, duplicate, and reserved-name
rules as profile environment keys. `TASK_DESCRIPTION` and every `KANDEV_*` key remain reserved.

### Scope-specific resolution APIs

Secret metadata includes `scope` and `workspace_id`. The secret layer exposes separate resolution
operations for different consumers:

- Global-only resolution for agent and executor profiles.
- Global-or-same-workspace resolution for a repository binding.
- User-authorized reveal for the settings UI.

Runtime callers must not use the broad settings reveal operation as an injection shortcut. This
keeps stale or manually inserted workspace-secret IDs in shared profiles fail-closed even if save
validation was bypassed.

### Origin-aware task environment merge

Environment construction is modeled as a tree rather than a precedence loop:

```text
Task runtime environment
├── Kandev/runtime-owned values
├── Selected executor profile
│   ├── literal bindings
│   └── Global secret bindings
└── Attached repositories (all branches, independent of position)
    ├── repository A: Global or same-workspace secret bindings
    ├── repository B: Global or same-workspace secret bindings
    └── ...

Agent profile environment: existing fill-missing defaults applied after the
task environment is resolved; Global secrets only.
```

The task-environment resolver preserves binding identity until validation is complete. It compares
environment key plus source identity, not decrypted plaintext:

- the same key bound to the same secret ID by multiple executor/repository origins is deduplicated;
- the same key bound to different secret IDs blocks launch;
- an executor literal and repository secret using the same key block launch, even if their current
  plaintext happens to match;
- a repository key colliding with a managed runtime value blocks launch rather than replacing it;
- repository order and task-repository position never choose a winner.

Only after the shape is conflict-free does the resolver reveal each distinct repository secret.
Missing, deleted, unreadable, unauthorized, or wrong-workspace bindings block launch before
executor provisioning or repository setup. Errors identify the environment key and all repository
or executor origins needed to fix configuration, but never include secret values. Secret IDs are
omitted from user-facing errors and routine logs.

The existing agent-profile contract remains a lower-priority default: it fills keys absent from the
resolved task environment. It does not turn repository ordering into precedence and avoids changing
the established profile behavior as part of this feature.

### Provisioning snapshot and runtime propagation

Repository bindings are resolved when Kandev provisions or freshly recreates the task environment.
The effective map is retained in the in-memory `AgentExecution` snapshot and is supplied to:

- repository setup scripts and executor preparation that already consume the launch environment;
- the agent process and its child shell commands;
- new terminal-panel shells opened for that execution;
- Docker, Sprites, standalone, and other launch transports that receive the full request map.

An already-running process or open terminal does not change when a binding or secret value changes.
Warm resume keeps the provisioned environment; Reset Environment, cold recreation, or another fresh
launch resolves current bindings again.

SSH receives the explicitly resolved repository binding map in addition to its existing managed
credential allowlist. The transport forwards those named keys because repository association is the
authority grant; it still does not forward arbitrary control-plane or unrelated profile environment
variables. Remote agent processes and remote terminal instances receive the same approved map.

## Consequences

- Users can keep broadly reusable credentials Global and constrain project-specific credentials to
  one workspace.
- A repository becomes the durable authority grant. Every task using it inherits the binding set;
  v1 has no task override.
- Multi-repository launches are deterministic and fail visibly instead of depending on attachment
  order.
- Shared profile selectors and backend validation must filter to Global secrets.
- Secret and repository settings need scope-aware caches so a workspace list cannot leak into a
  global profile selector.
- Environment failures move earlier in launch, before expensive or externally visible executor
  setup.
- SSH's security boundary remains explicit: repository-associated keys expand the per-launch
  allowlist, not the host environment.
- Existing global secrets and existing profiles preserve their behavior after migration.

## Alternatives Considered

1. **Keep every secret Global.** Rejected because repository credentials could not be constrained to
   a workspace and shared selectors would expose unnecessary authority.
2. **Add workspace secrets but let executors use them.** Rejected because executors and their
   profiles are instance-shared, so a workspace-scoped reference would cross the ownership model.
3. **Store bindings on tasks.** Rejected for v1 because users asked for repository defaults inherited
   by every task. Task overrides also introduce another precedence tier and audit surface.
4. **Pick a winner by primary repository or attachment order.** Rejected because adding or reordering
   a repository could silently change credentials.
5. **Namespace variables per repository.** Rejected because tools and setup scripts expect canonical
   names such as `NPM_TOKEN`, and automatic renaming would not configure those consumers.
6. **Compare decrypted values and deduplicate equal plaintext.** Rejected because identity and
   rotation semantics matter; two independent credentials that happen to match today are still a
   conflict.
7. **Cascade bindings when a secret is deleted.** Rejected because launch would silently lose a
   required credential instead of exposing broken configuration.
8. **Forward the complete launch environment over SSH.** Rejected because it would undo SSH's
   deliberate control-plane environment boundary.
