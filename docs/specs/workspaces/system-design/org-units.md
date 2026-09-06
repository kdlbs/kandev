---
status: draft
system: workspaces
requirements:
  - REQ-WORKSPACES-ORG-UNITS-001
  - REQ-WORKSPACES-ORG-UNITS-002
  - REQ-WORKSPACES-ORG-UNITS-003
  - REQ-WORKSPACES-ORG-UNITS-004
  - REQ-WORKSPACES-ORG-UNITS-005
---

# Organization Unit System Design

## Purpose and boundaries

This design owns the unit tree, unit membership, workspace placement, and the
resolution of a user's effective role on a workspace.

It does not own the scope vocabulary or the role-to-scope mapping, which stay in
`internal/authz` under the
[auth system](../../auth/requirements/roles-and-scopes.md). It does not own the
tenant boundary, which stays in `internal/org` under
[multi-tenancy](../../multi-tenancy/spec.md). Both are consumed here unchanged.

The tree replaces `workspaces.visibility` outright. There is no visibility
column, no organization-level default visibility, and no second reach path to
keep in agreement with this one.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-WORKSPACES-ORG-UNITS-001` | [Data and contracts](#data-and-contracts) |
| `REQ-WORKSPACES-ORG-UNITS-002` | [Resolution](#resolution) |
| `REQ-WORKSPACES-ORG-UNITS-003` | [Personal units](#personal-units) |
| `REQ-WORKSPACES-ORG-UNITS-004` | [Resolution](#resolution) |
| `REQ-WORKSPACES-ORG-UNITS-005` | [Placement and movement](#placement-and-movement) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `internal/orgunit` | Unit lifecycle, membership, tree invariants, materialized path maintenance. |
| `internal/authz` | Scope vocabulary and role-to-scope mapping. Gains `unit.manage`; otherwise unchanged. |
| `internal/authz.ResolveWorkspace` | Reads a precomputed ancestor role set instead of a visibility column. |
| `internal/task/service` | Workspace creation and movement, reach predicates for tasks and sessions. |
| `apps/web` settings tree | Unit tree management, unit membership, workspace placement. |

## Data and contracts

```
org_units
  id             string     PK
  org_id         string     FK -> orgs.id
  parent_id      string     FK -> org_units.id, '' for the root
  kind           enum       root | standard | personal
  owner_user_id  string     the owner for kind='personal', '' otherwise
  name           string
  path           string     materialized ancestor path, '/<id>/<id>/'
  created_at     timestamp
  updated_at     timestamp

unit_members
  unit_id    string   PK part, FK -> org_units.id (cascade delete)
  user_id    string   PK part, FK -> users.id     (cascade delete)
  role       enum     owner | collaborator | viewer
  added_by   string
  created_at timestamp

workspaces
  + unit_id  string  FK -> org_units.id, NOT NULL

workspace_members    unchanged, now meaning a direct grant only
```

Removed by this design: `workspaces.visibility`, the organization's
`default_workspace_visibility` setting, and `POST /api/v1/workspaces/visibility/bulk`.

`path` is a materialized path of ancestor ids, maintained on insert and on move.
It is chosen over a recursive query because both SQLite and Postgres are
supported and a prefix match is portable across them; it is a denormalization
whose only writer is `internal/orgunit`.

Indexes: `org_units(org_id, parent_id)`, `org_units(path)`,
`unit_members(user_id)`, `workspaces(unit_id)`.

### HTTP surface

```
GET    /api/v1/units                        tree for the caller's org
POST   /api/v1/units                        {parent_id, name}      unit.manage on parent
PATCH  /api/v1/units/{id}                   {name?, parent_id?}    unit.manage on both
DELETE /api/v1/units/{id}                   unit.manage, refuses when not empty
GET    /api/v1/units/{id}/members            unit.read
PUT    /api/v1/units/{id}/members/{userId}  {role}                 member.manage on unit
DELETE /api/v1/units/{id}/members/{userId}  member.manage on unit
PATCH  /api/v1/workspaces/{id}              {unit_id} to move      workspace.manage + unit.manage
```

Workspace DTOs carry `unit_id`, the unit's display path, and the caller's
effective `viewer_role` and `scopes`. They no longer carry `visibility`.

## Resolution

`ResolveWorkspace(subject, workspace)` decides in this order:

1. Cross-organization: if the subject's org and the workspace's org are both set
   and differ, the result is no reach. This is evaluated first and is unchanged.
2. Unscoped subject (authentication disabled): owner.
3. Otherwise the effective role is the highest of:
   - the role the subject's organization role grants at the root unit,
   - every `unit_members` row for the subject whose unit is an ancestor of the
     workspace's unit, including that unit itself,
   - any `workspace_members` row for the subject on this workspace.

Ancestors come from the workspace unit's `path`, so the membership lookup is a
single query against `unit_members` filtered by the ids in that path. Roles rank
`viewer < collaborator < owner` and the maximum wins, which is why no grant can
lower an inherited role.

A personal unit contributes nothing to any subject except its owner, because it
holds no membership rows.

A disabled account resolves to no reach before the union is computed.

## Personal units

A personal unit is created in the same transaction as its user, is `kind =
'personal'`, and carries `owner_user_id`. `internal/orgunit` refuses membership
writes against it, refuses to move or delete it while its owner exists, and
places workspaces there when a creator names no unit.

Deleting a user deletes the personal unit and its workspaces through the
existing workspace deletion path.

## Placement and movement

A move updates `workspaces.unit_id`, then recomputes reach. Because reach is
derived rather than stored, no rows need rewriting beyond the single column.

Moving a unit rewrites the `path` of that unit and of every descendant in one
transaction, and refuses when the destination is a descendant of the unit being
moved.

Both operations publish a workspace reach-changed event so the WebSocket
gateway can drop subscriptions held by users who no longer reach the workspace,
reusing the revocation path that membership removal already uses.

## Failure and recovery

A refused tree operation names the blocking condition: the remaining children or
workspaces for a delete, the ancestor cycle for a move, the organization
mismatch for a cross-tenant parent. None of these are retried automatically.

A reach lookup that fails to read `unit_members` returns no reach rather than a
permissive default, matching the fail-closed rule the current resolver already
applies to workspace lookups.

## Persistence

Schema changes arrive as idempotent migrations per
[ADR 0027](../../../decisions/0027-replayable-schema-migrations.md). The one-shot data
migration:

1. Creates a root unit per organization.
2. Creates a personal unit per user.
3. Places every existing workspace with a non-empty `owner_id` in that owner's
   personal unit, and every workspace with an empty `owner_id` in the root unit,
   which preserves the pre-authentication behaviour of a workspace nobody owns.
4. Retains existing `workspace_members` rows as direct grants.
5. Drops `workspaces.visibility`.

The migration is idempotent and safe to replay. It never widens reach: a
workspace that was private lands in a personal unit, and a workspace that was
organization-visible lands under the root unit only when it had no owner.

Table rebuilds that recreate `workspaces` or `tasks` must carry the new columns
in both the replacement `CREATE TABLE` and the `INSERT ... SELECT` list, which
is the failure mode recorded in `apps/backend/AGENTS.md`.

## Security

The tenant check runs before any role resolution and is unaffected by the tree.
A unit, a membership, and a workspace all carry `org_id`, and every write
validates that the parent, the destination, and the subject share it.

Reach absence is reported as not found rather than forbidden, so the tree does
not disclose the existence of a workspace or unit that the caller cannot reach.

`unit.manage` is a new scope in the `internal/authz` registry, held by the
organization administrator role and by a unit `owner`. The completeness test
that asserts every guarded action names a registered scope covers the new
endpoints.

## Observability

Unit creation, movement, deletion, and membership changes emit structured logs
carrying the acting user, the unit, and the organization. The reach-changed
event carries the workspace and the set of users whose reach was withdrawn, so a
revocation that fails to disconnect a client is visible.

## Related decisions

- [ADR 0027: replayable migrations](../../../decisions/0027-replayable-schema-migrations.md)
