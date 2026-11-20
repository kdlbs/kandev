---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-WORKSPACE-RESOLUTION-001
  - REQ-INTEGRATIONS-LINEAR-WRITE-SCOPE-001
---

# Linear Workspace Operations System Design

## Purpose and boundaries

The integration system owns the workspace used for provider calls and the
Linear issue state mutation. The auth system owns workspace roles and scopes.
This design uses `task.write` from
[`roles-and-scopes.md`](../../auth/requirements/roles-and-scopes.md).

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| `AC-INTEGRATIONS-WORKSPACE-RESOLUTION-001.1` | Default workspace resolution |
| `AC-INTEGRATIONS-LINEAR-WRITE-SCOPE-001.1` | User issue state changes |
| `AC-INTEGRATIONS-LINEAR-WRITE-SCOPE-001.2` | WebSocket and broker paths |

## Components and responsibilities

- `workspacescope.DefaultResolver` chooses a workspace for legacy integration
  operations and singleton configuration migration.
- `linear.Service` resolves a request workspace, checks access, and selects the
  workspace-owned configuration, secret, and client.
- The Linear HTTP controller and legacy WebSocket handler call the
  user-facing state-change method.
- `taskservice.AuthorizeWorkspaceScope` enforces the user's `task.write`
  permission. `backendapp` wires this check into the Linear service.
- The plugin issue broker calls the existing workspace writeback method. It
  keeps its current authorization policy.

## Default workspace resolution

`DefaultResolver` checks for the `workspaces` table, then reads workspace IDs
from user settings. A valid selected ID takes precedence over the oldest
workspace. If no valid selection exists, the resolver uses the oldest
workspace. It uses `default` when the table is missing or has no rows.

SQLite checks `sqlite_master`. PostgreSQL checks `pg_catalog.pg_class` and
`pg_table_is_visible`. The visibility check follows the active `search_path`,
which matches the later unqualified workspace and user queries. The resolver
reads the settings on each call.

## User issue state changes

The HTTP route receives a workspace ID in the query. The legacy WebSocket route
has no workspace ID, so it resolves the default first. Both routes call
`SetIssueStateForWorkspaceAsUser`.

The service normalizes an empty ID before it calls the write authorizer. The
authorizer calls `AuthorizeWorkspaceScope` with `task.write` for that resolved
ID. If the check succeeds, the existing `clientFor` path applies the
`workspace.read` check, loads the workspace's Linear config and secret, and
returns its client. The client sends the `issueUpdate` mutation. If the write
check fails, the client is not selected and Linear receives no mutation.

The HTTP route maps missing or unreadable workspaces to 404 and a missing write
scope to 403. The WebSocket route returns `NOT_FOUND` or `FORBIDDEN` for the
same cases.

## WebSocket and broker paths

The legacy WebSocket transition uses the same user-facing write check as HTTP.
The plugin issue broker continues to call `SetIssueStateForWorkspace` directly.
That method keeps its existing workspace access check and does not use the new
user-only write authorizer.

## Data and failure behavior

`linear_configs` stores the connection by workspace ID. The secret store keeps
the provider key for that workspace. The service caches each client by
workspace ID. Linear owns the resulting issue state.

Kandev rejects a state change before the provider call when the caller lacks
`task.write`. The caller cannot use a readable workspace's saved credential to
write to Linear. Kandev does not change the saved credentials on a denied
request.

## Related decisions

- [Workspace-scoped integration settings](../../../decisions/0030-workspace-scoped-integration-settings.md)
