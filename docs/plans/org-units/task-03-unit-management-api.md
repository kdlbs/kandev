---
id: "03-unit-management-api"
title: "Unit management API"
status: done
wave: 3
depends_on: ["02-reach-resolution"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/org-units.md"
---

# Task 03: Unit Management API

## Outcome

Units, their members, and workspace placement are manageable over HTTP, with
authorization on every route and live revocation when reach narrows.

## In scope

- The routes listed in the design's HTTP surface section, each gated on
  `unit.manage`, `member.manage`, or `workspace.manage` as stated there.
- Refusals that name their blocking condition: a non-empty unit on delete, a
  cycle on move, an organization mismatch on either.
- Publishing the reach-changed event on membership removal and on workspace
  move, reusing the existing revocation path.
- Workspace DTOs carrying `unit_id` and the unit display path.

## Out of scope

- Interface work, order 04.
- Removing the visibility routes, order 05.

## Requirements

`REQ-WORKSPACES-ORG-UNITS-001`, `REQ-WORKSPACES-ORG-UNITS-002`,
`REQ-WORKSPACES-ORG-UNITS-005`.

Acceptance criteria: `AC-WORKSPACES-ORG-UNITS-001.5`, `.6`, `.8`,
`AC-WORKSPACES-ORG-UNITS-002.1`, `.5`, `.6`, `AC-WORKSPACES-ORG-UNITS-003.2`,
`AC-WORKSPACES-ORG-UNITS-005.1` through `.5`.

## System design

[Organization units](../../specs/workspaces/system-design/org-units.md), sections
Data and contracts, Placement and movement, Failure and recovery.

## Implementation acceptance

1. A caller without `unit.manage` receives a refusal on create, rename, move and
   delete, and nothing is written.
2. Adding a member to a personal unit is refused.
3. Moving a workspace out of a unit drops the WebSocket subscriptions of users
   who no longer reach it.

## Verification

```bash
cd apps/backend
go test ./internal/orgunit/... ./internal/task/handlers/...
go test ./internal/task/service/ -run 'TestUnitRevocation|TestWorkspaceMove'
make lint
```

## Likely files

- `apps/backend/internal/orgunit/controller.go` (new)
- `apps/backend/internal/task/handlers/`
- `apps/backend/internal/backendapp/helpers.go` route registration

## Results

Done. `/api/v1/units` serves the tree, its members, and workspace placement.
Reading needs only an identity, because a member has to see the shape of their
organization to understand why they reach what they reach; every write needs
`unit.manage`, a new scope held by the organization administrator.

A unit in another tenant answers 404 rather than 403, so one organization's tree
is not enumerable from another, and moving a workspace validates the
destination's organization before it changes anything.

RED/GREEN:

- RED: removing the scope gate from the write group fails three assertions in
  `TestUnitWritesRequireUnitManage`.
- GREEN: `go test ./internal/orgunit/... ./internal/authz/... ./internal/task/...`
- GREEN: `make lint` reports 0 issues.
