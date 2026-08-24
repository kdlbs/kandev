---
status: active
system: workspaces
created: 2026-08-03
owners:
  - tbd
---
# Repository and Workspace Secrets Requirements

## Overview

Repository setup and agent work often need credentials that are specific to a project: package registry tokens, deployment keys, test service credentials, or API keys. Today users must place those values on a shared agent or executor profile, even when only one workspace or repository needs them. This broadens access and becomes ambiguous when a task attaches several repositories.

## Requirements

### REQ-WORKSPACES-REPOSITORY-SECRETS-001: Repository and Workspace Secrets

**Intent:** Repository setup and agent work often need credentials that are specific to a project: package registry tokens, deployment keys, test service credentials, or API keys. Today users must place those values on a shared agent or executor profile, even when only one workspace or repository needs them. This broadens access and becomes ambiguous when a task attaches several repositories.

#### Acceptance criteria

- **AC-WORKSPACES-REPOSITORY-SECRETS-001.1:** Secrets can be created as **Global** or **Workspace**.
- **AC-WORKSPACES-REPOSITORY-SECRETS-001.2:** Global secrets are available across the current user's workspaces. Workspace secrets belong to exactly one workspace.
- **AC-WORKSPACES-REPOSITORY-SECRETS-001.3:** Existing secrets become Global during migration.
- **AC-WORKSPACES-REPOSITORY-SECRETS-001.4:** Agent and executor profiles can select Global secrets only.
- **AC-WORKSPACES-REPOSITORY-SECRETS-001.5:** A workspace-owned repository can bind environment variable names to Global secrets or secrets from that same workspace.
- **AC-WORKSPACES-REPOSITORY-SECRETS-001.6:** Every task attaching the repository inherits its bindings. A multi-repository task inherits all attached repositories' bindings plus its selected executor profile environment.
- **AC-WORKSPACES-REPOSITORY-SECRETS-001.7:** Repository bindings contain secret references only, never literal values.
- **AC-WORKSPACES-REPOSITORY-SECRETS-001.8:** The effective repository environment is available to repository setup, the agent process, child shells, and terminal-panel terminals in every supported executor, including explicitly approved forwarding to SSH.

## Migrated source detail

## Why

Repository setup and agent work often need credentials that are specific to a project: package
registry tokens, deployment keys, test service credentials, or API keys. Today users must place
those values on a shared agent or executor profile, even when only one workspace or repository
needs them. This broadens access and becomes ambiguous when a task attaches several repositories.

Kandev needs a repository-level authority grant with workspace-scoped storage and a deterministic,
fail-closed way to build one task environment.

## What

- Secrets can be created as **Global** or **Workspace**.
- Global secrets are available across the current user's workspaces. Workspace secrets belong to
  exactly one workspace.
- Existing secrets become Global during migration.
- Agent and executor profiles can select Global secrets only.
- A workspace-owned repository can bind environment variable names to Global secrets or secrets
  from that same workspace.
- Every task attaching the repository inherits its bindings. A multi-repository task inherits all
  attached repositories' bindings plus its selected executor profile environment.
- Repository bindings contain secret references only, never literal values.
- The effective repository environment is available to repository setup, the agent process, child
  shells, and terminal-panel terminals in every supported executor, including explicitly approved
  forwarding to SSH.

## Scope and permissions

Global means user-global when authentication is enabled and install-global when authentication is
disabled. It does not make one authenticated user's secret visible to another user.

A caller may:

- manage their own Global secrets;
- manage Workspace secrets only after Kandev authorizes that workspace;
- bind a repository only to a visible Global secret or a secret whose `workspace_id` equals the
  repository's `workspace_id`;
- view and edit bindings only through a repository they are authorized to access.

Cross-user and cross-workspace IDs return not found or a generic invalid-binding response without
revealing that the target secret exists. Administrators do not bypass workspace privacy.

All instance-shared profile types, including agent and executor profiles, reject Workspace secret
references both when saving and when resolving legacy or manually inserted data.

## Data model

### Secret

User-visible secret metadata adds:

```text
scope: "global" | "workspace"
workspace_id: string | null
```

Rules:

- `global` requires no workspace ID.
- `workspace` requires an existing, authorized workspace ID.
- scope and workspace are immutable after creation; users create a replacement to move authority.
- existing user-visible rows migrate to `global`.
- deleting a workspace deletes its Workspace secrets.
- secret values remain encrypted with the existing AES-256-GCM store and never appear in list,
  repository, task, event, or error payloads.

### Repository secret binding

```text
repository_id: string
key: POSIX environment identifier
secret_id: string
created_at: timestamp
updated_at: timestamp
```

The pair `(repository_id, key)` is unique. The repository foreign key cascades on repository or
workspace deletion. The secret reference deliberately permits a dangling ID so deleting a secret
leaves a broken binding that blocks future launch.

Keys follow profile environment rules: at most 100 bindings per repository, maximum key length 256,
POSIX identifier syntax, no duplicates, and no `TASK_DESCRIPTION` or `KANDEV_*` keys.

## API surface

The existing HTTP and WebSocket secret CRUD contracts become scope-aware.

- Creating a secret accepts `scope` and, for Workspace scope, `workspace_id`. Omitting `scope`
  remains backward-compatible and creates Global.
- Secret metadata responses include `scope` and `workspace_id`.
- The default list returns Global secrets. A Workspace-filtered list returns that workspace's
  secrets, with an explicit option to include visible Global secrets for a repository selector.
- Update changes name and/or value only; scope is immutable.
- Reveal and delete preserve existing response shapes while applying scope authorization.

Repository create, get, list, update, and repository events include:

```json
{
  "secret_bindings": [
    { "key": "NPM_TOKEN", "secret_id": "..." }
  ]
}
```

Create/update treats the supplied list as the complete desired set and persists repository fields
and bindings atomically. Omitting the field on update preserves the existing set; an explicit empty
list clears it. Automated find-or-create/backfill paths that do not submit bindings preserve them.

## Environment resolution

Kandev resolves one task environment from named origins:

1. Validate all repository keys and collect the selected executor profile entries plus every
   attached repository binding without decrypting repository values.
2. Reject collisions with Kandev/runtime-owned values.
3. For a repeated key:
   - deduplicate exact references to the same secret ID;
   - reject different secret IDs;
   - reject literal-versus-secret or different-literal definitions.
4. Reveal each unique repository secret using Global-or-same-workspace authorization.
5. Reject any missing, deleted, unreadable, unauthorized, or wrong-workspace reference.
6. Flatten the validated result and continue normal launch. Agent-profile values retain their
   existing fill-missing behavior and may not overwrite this task environment.

Repository/task order never selects a winner. Failure occurs before executor provisioning,
worktree/container/sandbox creation, repository setup, or agent start.

User-facing conflict errors contain the environment key and every relevant repository/executor
origin. Broken-binding errors contain the key and repository origin. Neither contains plaintext or
secret IDs. Routine logs follow the same redaction rule.

## Runtime lifecycle

Bindings are evaluated when a task environment is provisioned or freshly recreated. The flattened
map is the execution's environment snapshot.

- Repository setup scripts see it through the existing launch environment.
- Agents and their child shell commands inherit it.
- A newly opened terminal for that execution receives the same snapshot.
- Local, Worktree, Docker, Remote Docker, Sprites, and standalone transports receive the resolved
  map through their normal launch path.
- SSH forwards the explicit repository-approved keys to remote agent and terminal instances in
  addition to its managed credential allowlist; it does not forward arbitrary host variables.

Editing a binding or rotating a secret does not mutate an already-running process or terminal. Warm
resume retains the provisioned environment. Reset Environment, cold recreation, or a fresh task
environment resolves the current configuration. A terminal already open keeps its startup
environment; a new terminal uses the current execution snapshot.

## Settings experience

### Global secrets

The existing **Settings > General > Secrets** page becomes explicitly Global. It lists and creates
Global secrets only. Existing create, edit, reveal, and delete interactions remain available.

### Workspace secrets

Each workspace gains **Settings > Workspaces > [workspace] > Secrets**. It uses the same management
surface but creates and lists only secrets for that workspace. The page explains that these secrets
can be selected by repositories in this workspace and cannot be used by shared profiles.

### Repository bindings

The repository editor gains an **Environment secrets** section. Each row has an environment key and
a secret selector containing Global plus same-workspace secrets, with scope labels. Users can add,
remove, and replace rows as part of the repository's existing manual-save flow. A deleted secret is
shown as a missing reference rather than silently dropping the row.

The selector never reveals values. Executor and agent profile selectors show Global secrets only.

## Mobile design contract

- Entry point: the existing mobile Settings navigation exposes Workspace Secrets alongside
  Repositories, Workflows, Integrations, and Automations.
- Hierarchy: secret list and create/edit forms use the existing settings page/card composition;
  repository bindings stay within the repository editor rather than opening a desktop-only side
  panel.
- Interaction: rows stack to one column at phone width, selectors and key inputs use touch-sized
  controls, destructive actions retain confirmation, and save/cancel remain reachable without
  horizontal scrolling.
- Scroll ownership: the Settings content area remains the single page scroll owner. Repository
  editing must not introduce nested vertical scrolling or a clipped fixed footer.
- Parity: mobile can create, edit, reveal, and delete Workspace secrets and add/remove repository
  bindings, matching desktop capability.
- Accessibility: scope is conveyed by text/badge as well as color; icon-only actions have localized
  accessible labels; missing references and validation errors are announced in text.

## Failure modes

- Invalid scope/workspace combinations are rejected with 400.
- A foreign or nonexistent workspace/secret is not disclosed.
- A shared profile containing a Workspace secret cannot be saved and cannot resolve it at runtime.
- Duplicate or reserved repository environment keys are rejected on repository save.
- A deleted or unreadable bound secret blocks launch and names the affected key/repository without
  exposing secret material.
- Conflicting executor/repository or repository/repository bindings block launch and list all
  origins; no partial environment is provisioned.
- Database mutation of repository fields and bindings is atomic.
- Secret decryption failures never fall back to a literal, stale value, or another same-named
  secret.
- SSH forwarding includes only managed credential keys and the launch's explicitly approved
  repository keys.

## Persistence guarantees

- Existing secrets retain IDs, ciphertext, names, owners, and timestamps and are marked Global.
- Repository bindings survive restart and are inherited by new sessions and tasks.
- Secret plaintext is never persisted in repository, task, session, event, or task-environment rows.
- The effective decrypted map is process memory only and is cleared with the execution store.
- Repository deletion removes its bindings. Secret deletion preserves broken binding IDs. Workspace
  deletion removes workspace repositories, their bindings, and Workspace secrets.
- Schema changes and migrations are replayable on SQLite and PostgreSQL.

## Scenarios

- **GIVEN** a Global secret and a Workspace secret in workspace A, **WHEN** a repository in A is
  edited, **THEN** both are selectable, while a repository in workspace B sees only the Global
  secret and B's own Workspace secrets.
- **GIVEN** an executor profile editor, **WHEN** secret options load, **THEN** only Global secrets are
  available, and a direct request naming a Workspace secret is rejected.
- **GIVEN** two attached repositories bind `NPM_TOKEN` to the same secret ID, **WHEN** a task launches,
  **THEN** the binding is deduplicated and the task provisions once with that value.
- **GIVEN** two repositories bind `NPM_TOKEN` to different secret IDs, **WHEN** launch is requested,
  **THEN** launch fails before provisioning and names both repository origins without exposing IDs
  or values.
- **GIVEN** an executor literal and repository secret both define `NPM_TOKEN`, **WHEN** launch is
  requested, **THEN** launch fails instead of comparing or choosing plaintext.
- **GIVEN** a repository binding whose secret was deleted, **WHEN** a task using that repository
  launches, **THEN** launch fails with the repository and key while the stored binding remains
  visible as missing.
- **GIVEN** a successful local, Docker, Sprites, or SSH launch, **WHEN** repository setup, the agent,
  an agent child shell, and a new terminal read the bound key, **THEN** each receives the same
  provisioned value for that execution.
- **GIVEN** a running execution, **WHEN** a secret is rotated, **THEN** the existing process and open
  terminals keep their snapshot and a freshly recreated environment receives the new value.
- **GIVEN** a phone viewport, **WHEN** a user manages a Workspace secret and binds it to a repository,
  **THEN** the complete flow is usable without horizontal overflow or a desktop-only control.

## Out of scope for v1

- Per-task secret bindings or overrides.
- Workspace secrets on agent or executor profiles.
- Per-branch bindings when a task attaches multiple branches of one repository.
- Automatic environment-key namespacing.
- Order-based precedence or “primary repository wins.”
- Live mutation of running process or terminal environments.
- Secret-value audit logs, version history, or automatic rotation.
