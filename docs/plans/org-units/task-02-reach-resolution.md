---
id: "02-reach-resolution"
title: "Reach resolution on the tree"
status: done
wave: 2
depends_on: ["01-unit-tree-storage"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/org-units.md"
---

# Task 02: Reach Resolution on the Tree

## Outcome

`internal/authz.ResolveWorkspace` derives the effective role from the tree
instead of from `workspaces.visibility`. Roles combine by maximum, and no path
lowers an inherited role.

## In scope

- Ancestor role lookup from the workspace unit's path.
- Effective role as the maximum of the root-unit role, ancestor unit
  memberships, and any direct workspace grant.
- Cross-organization refusal evaluated before any role, unchanged.
- Disabled accounts and unreachable workspaces answering as not found.
- Registering the `unit.manage` scope and mapping it to the organization
  administrator role and to a unit `owner`.

## Out of scope

- The HTTP surface for units, order 03.
- Deleting the visibility column, order 05. This order stops consulting it.

## Requirements

`REQ-WORKSPACES-ORG-UNITS-002`, `REQ-WORKSPACES-ORG-UNITS-004`.

Acceptance criteria: `AC-WORKSPACES-ORG-UNITS-002.2`, `.3`, `.4`,
`AC-WORKSPACES-ORG-UNITS-004.1` through `.5`.

## System design

[Organization units](../../specs/workspaces/system-design/org-units.md), section
Resolution.

## Implementation acceptance

1. A member of a department reaches a workspace under one of its teams at the
   department role, with no direct grant present.
2. A direct `viewer` grant on a workspace whose unit already grants
   `collaborator` leaves the effective role at `collaborator`.
3. A workspace in another user's personal unit resolves to no reach for an
   organization administrator.

## Verification

```bash
cd apps/backend
go test ./internal/authz/...
go test ./internal/task/service/ -run 'TestReach|TestResolveWorkspace'
make lint
```

## Likely files

- `apps/backend/internal/authz/resolve.go`, `scopes.go`, `roles.go`
- `apps/backend/internal/task/service/service_access.go`

## Results

Done. `authz.WorkspaceRef` carries `InheritedRole` instead of `Visibility`, and
`ResolveWorkspace` returns the maximum of the inherited role, any direct grant,
and ownership. Nothing subtracts, so a grant can only raise what a unit gives.

Reach is resolved in two queries for a list rather than one per row: the
caller's unit roles and the units' paths are fetched once and matched by
ancestry in memory. Both the single and the bulk path fail closed, and the bulk
path fails closed for the whole list, because a partial answer would hide
workspaces the caller can reach and read as data loss rather than as a
permission error.

The service tests now build a real tree instead of setting a flag, which also
means the wiring is exercised: a resolver nothing calls protects nothing.

RED/GREEN:

- RED: making `highestRole` return the last non-empty role instead of the
  strongest fails `TestRolesCombineByMaximum/a direct grant cannot lower it`.
- GREEN: `go test ./internal/authz/... ./internal/task/... ./internal/orgunit/...`
- GREEN: `make lint` reports 0 issues.
