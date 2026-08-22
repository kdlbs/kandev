---
status: draft
created: 2026-08-22
owner: tbd
---

# Org Roles and Scopes

Extends [`spec.md`](spec.md). Consumed by
[workspace visibility and membership](../workspaces/membership.md) and
[multi-tenancy](../multi-tenancy/spec.md).

## Why

Kandev has two roles, `admin` and `member`, and they decide exactly one thing:
whether you can manage users and system settings. Everything else is decided by
sole ownership. A team that wants to work together therefore has no vocabulary
for "Ana runs agents, Bruno reviews the board, Carla administers the server but
should not be able to open a shell in someone's worktree." Roles need to carry
an explicit, auditable set of permissions rather than a single admin bit.

## What

- **Permissions are named scopes, not booleans.** Every guarded action maps to
  one scope. Authorization asks "does the caller hold scope X here?", never
  "is the caller an admin?".
- **Scopes live in one registry.** `internal/authz/registry.go` owns the scope
  identifiers, their human descriptions, and the role-to-scope mapping. A
  completeness test asserts every guarded action names a registered scope and
  every registered scope has at least one call site. There is no second
  permission map and no per-flag switch anywhere else.
- **Roles are fixed in v1; scopes are the extension point.** Kandev ships four
  org roles and three workspace roles with a static mapping. There is no custom
  role builder, no per-user grant table, and no UI for editing a role's scopes.
  Adding a capability means adding a scope and mapping it, which is a code
  change and a review.
- **Two role axes, resolved by one function.** An org role grants **org
  scopes** outright and a **default workspace role** used on workspaces that
  are visible to the whole org. A workspace role grants **workspace scopes** on
  one workspace. Effective scopes are resolved per (user, workspace) by a
  single function; nothing else derives permissions.
- **Org admin is still not a visibility role.** An admin sees org-visible
  workspaces because they are org-visible, exactly like any member, and sees
  nothing of a private workspace they are not in. Admin adds management scopes,
  never reach. This preserves today's hard-privacy guarantee rather than
  quietly dropping it.
- **Scopes are enforced server-side and mirrored to the client.** The API is
  authoritative. The boot payload and workspace DTOs carry the caller's
  effective scopes so the UI can hide what the caller cannot do, but hiding a
  control is never the enforcement.
- **A disabled account holds no scopes.** Role rows are retained; they grant
  nothing while the account is disabled, matching how sessions already fail
  closed.

## Data model

Roles are values, not rows.

```
users.role            org role:       owner | admin | member | guest
workspace_members.role workspace role: owner | collaborator | viewer
```

- `users.role` replaces today's `admin` / `member` values. The migration maps
  `admin -> admin` and `member -> member`; the first admin on an instance also
  becomes `owner`. `guest` is new and is never assigned by migration.
- Exactly one `owner` exists per org.
- Workspace roles are defined by
  [workspace membership](../workspaces/membership.md); this spec owns only what
  each one grants.

## Scope registry

**Org scopes** — granted by the org role, independent of any workspace.

| Scope | Grants |
|---|---|
| `org.members.manage` | invite, create, disable, and re-role users; mint invites |
| `org.settings.manage` | org name, org-level defaults, org visibility default |
| `org.config.manage` | org executors, executor profiles, environments, agents, agent profiles, editors, prompts, notification providers |
| `org.delete` | delete the org |

**Workspace scopes** — granted by the workspace role, per workspace.

| Scope | Grants |
|---|---|
| `workspace.read` | see the workspace, its board, tasks, workflows, transcripts, diffs |
| `workspace.manage` | rename, defaults, visibility, delete the workspace |
| `task.write` | create, edit, move, assign, archive, delete tasks |
| `session.prompt` | start or resume a session and message an agent |
| `session.control` | stop or cancel a running agent |
| `session.exec` | terminal, shell, file writes, port previews, VS Code, LSP |
| `repository.manage` | add or remove workspace repositories |
| `secret.manage` | workspace secrets and integration credentials |
| `member.manage` | add or remove workspace members, transfer ownership |

`session.exec` is deliberately separate from `session.prompt`. Prompting an
agent is bounded by the agent's own permissions; a shell in the worktree is
not. A `viewer` who can read a transcript must not thereby get a shell.

`secret.manage` is write-only in the sense that already holds today: secret
values are never returned by any API to any role.

## Role mappings

**Org role → org scopes and default workspace role**

| Org role | Org scopes | Default workspace role on org-visible workspaces |
|---|---|---|
| `owner` | all four | `collaborator` |
| `admin` | `org.members.manage`, `org.settings.manage`, `org.config.manage` | `collaborator` |
| `member` | none | `collaborator` |
| `guest` | none | none — reaches only workspaces they are explicitly a member of |

**Workspace role → workspace scopes**

| Workspace role | Scopes |
|---|---|
| `owner` | all nine |
| `collaborator` | `workspace.read`, `task.write`, `session.prompt`, `session.control`, `session.exec` |
| `viewer` | `workspace.read` |

`repository.manage`, `secret.manage`, `member.manage`, and `workspace.manage`
belong to the workspace owner alone. A team that wants a second person able to
do those transfers ownership or adds them as a workspace `owner` — a workspace
may have more than one `owner` role row even though `workspaces.owner_id` names
a single accountable one.

## Resolution

Effective scopes for (user `U`, workspace `W`), evaluated in order:

1. `U` is disabled, or `U`'s org is suspended → **no scopes**.
2. Org scopes = the mapping for `U.role`, always.
3. Workspace scopes:
   1. `W.owner_id == U.id` → the `owner` workspace role.
   2. A `workspace_members` row for (`W`, `U`) exists → that row's role.
   3. `W.visibility == 'org'` and `U` is in `W`'s org → the default workspace
      role for `U.role`.
   4. otherwise → **none**, and `W` is unreachable (404, no existence leak).

This function is the only place permissions are derived. Step 3.2 outranks 3.3
in both directions: it is how a `guest` gets into one workspace, and how a
member is narrowed to `viewer` on a sensitive one.

## API surface

```
GET  /api/v1/auth/me                -> adds "org_scopes": [...]
GET  /api/v1/workspaces             -> each item adds "viewer_role" and "scopes": [...]
GET  /api/v1/workspaces/{id}        -> adds "viewer_role" and "scopes": [...]
PATCH /api/v1/users/{id}            -> accepts "role" (requires org.members.manage)
GET  /api/v1/authz/scopes           -> the registry: id + description, for UI labels
```

The boot payload carries `org_scopes` and, for the active workspace, its
`scopes`. Absence of a scope in the response is a UI hint; the server rejects
the action regardless.

## Permissions

Changing a user's org role requires `org.members.manage`. Three guards hold
regardless of scope:

- The last `owner` cannot be demoted or disabled.
- No user may change their own org role, matching the existing self-actions
  guard ([`self-actions-guard.md`](self-actions-guard.md)).
- `org.delete` is never granted to anyone but the `owner`.

## Failure modes

- **A guarded action names an unregistered scope** — build failure via the
  registry completeness test, not a runtime grant.
- **A registered scope has no call site** — build failure. A scope nobody
  enforces is a permission that silently does not exist.
- **A caller lacks a workspace scope but holds `workspace.read`** — 403, not
  404. Existence is already known to them, so hiding it leaks nothing and a 404
  would be confusing.
- **A caller lacks `workspace.read`** — 404. Existence is not disclosed.
- **Scope resolution cannot load the workspace or the membership row** — fail
  closed with no scopes; never fall back to the org default role, because that
  is the branch that would silently widen access on a transient DB error.
- **The client is stale about its scopes** — the server rejects the action and
  the client refreshes from the response; there is no scope cache with an
  independent TTL.

## Persistence guarantees

- Roles survive restart and survive disabling `features.auth` (they become
  inert, exactly as ownership does today).
- The registry is code, not data: it cannot drift per-instance and needs no
  migration when scopes are added.
- No effective-scope result is cached across requests. Resolution is cheap and a
  cache is how a removed member keeps their access for another minute.

## Scenarios

- **GIVEN** a guarded action, **WHEN** it names a scope absent from the
  registry, **THEN** the completeness test fails and names the action.
- **GIVEN** a registered scope with no enforcement call site, **WHEN** the
  completeness test runs, **THEN** it fails and names the scope.
- **GIVEN** an org admin and a private workspace they are not a member of,
  **WHEN** they request it, **THEN** the response is 404 — admin is not a
  visibility role.
- **GIVEN** an org admin and an org-visible workspace, **WHEN** they open it,
  **THEN** they hold `collaborator` scopes there, the same as any member.
- **GIVEN** a `viewer` on a workspace, **WHEN** they read a task transcript,
  **THEN** it succeeds; **WHEN** they open a terminal or send a prompt,
  **THEN** the response is 403.
- **GIVEN** a `guest`, **WHEN** they list workspaces, **THEN** they see only
  workspaces they are explicitly a member of, even ones marked org-visible.
- **GIVEN** a `member` narrowed to `viewer` by an explicit membership row on an
  org-visible workspace, **WHEN** they attempt `task.write`, **THEN** the
  response is 403 — the explicit row outranks the org default.
- **GIVEN** the only `owner` of an org, **WHEN** an admin attempts to demote or
  disable them, **THEN** the request is refused.
- **GIVEN** any user, **WHEN** they attempt to change their own org role,
  **THEN** the request is refused.
- **GIVEN** a user whose account is disabled, **WHEN** any request is made with
  their session or token, **THEN** it fails closed and no scope is granted.
- **GIVEN** a transient failure loading a membership row, **WHEN** scopes are
  resolved, **THEN** no scopes are returned and the request fails closed rather
  than falling back to the org default role.

## Out of scope

- **Custom roles and a role editor.** Roles are fixed; scopes are the
  extension point.
- **Per-user scope grants** outside a role.
- **Scopes on personal access tokens.** A PAT carries its user's full scope
  set; narrowing a token is a separate feature.
- **Task-level or step-level human permissions.** Existing workflow
  participants name agent profiles, not people.
- **Time-bound or conditional grants.**
