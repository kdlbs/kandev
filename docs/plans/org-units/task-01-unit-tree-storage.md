---
id: "01-unit-tree-storage"
title: "Unit tree storage and placement"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/requirements/org-units.md"
---

# Task 01: Unit Tree Storage and Placement

## Outcome

`org_units` and `unit_members` exist, every organization has a root unit, every
user has a personal unit, and every workspace has a `unit_id`. Reach still comes
from visibility, so behaviour is unchanged at the end of this order.

## In scope

- `org_units` and `unit_members` tables with the indexes named in the design.
- `workspaces.unit_id`, not null.
- `internal/orgunit`: unit lifecycle, membership rows, tree invariants (single
  parent, no cycles, one organization, empty before delete), and materialized
  path maintenance including subtree reparenting.
- Root unit created with an organization; personal unit created with a user.
- The one-shot data migration described in the design's Persistence section.
- Carrying `unit_id` through every migration that recreates `workspaces` or
  `tasks`.

## Out of scope

- Reading the tree for authorization; that is order 02.
- HTTP surface; that is order 03.
- Removing `workspaces.visibility`; that is order 05.

## Requirements

`REQ-WORKSPACES-ORG-UNITS-001`, `REQ-WORKSPACES-ORG-UNITS-003`.

Acceptance criteria: `AC-WORKSPACES-ORG-UNITS-001.1` through `.7`,
`AC-WORKSPACES-ORG-UNITS-003.1`, `.5`, `.6`.

## System design

[Organization units](../../specs/workspaces/system-design/org-units.md), sections
Data and contracts, Personal units, Persistence.

## Implementation acceptance

1. A fresh database has one root unit per organization, one personal unit per
   user, and no workspace with an empty `unit_id`.
2. Reparenting a unit rewrites the path of every descendant, and a move that
   would create a cycle is refused.
3. The migration places every previously private workspace in its owner's
   personal unit, and no user reaches a workspace they could not reach before.

## Verification

```bash
cd apps/backend
go test ./internal/orgunit/... ./internal/task/repository/sqlite/... ./internal/office/repository/sqlite/...
go test ./internal/task/service/ -run 'TestUnitPlacement|TestMigrationDoesNotWidenReach'
```

## Likely files

- `apps/backend/internal/orgunit/` (new)
- `apps/backend/internal/task/repository/sqlite/` schema and migrations
- `apps/backend/internal/office/repository/sqlite/base_migrations.go` rebuild lists
- `apps/backend/internal/user/store/` personal unit creation
- `apps/backend/internal/org/service.go` root unit creation

## Results

Done. `internal/orgunit` owns the tree, its membership, and its invariants:
single root and single personal unit per user enforced by unique indexes, a
cycle refused on move, protected units refused on move and delete, a personal
unit refusing members, and delete failing closed when the occupancy seam is
unwired. Ancestry is a materialized path, and reparenting rewrites the whole
subtree in one transaction.

`workspaces.unit_id` is added with its index, and the one-shot backfill gives
every organization a root, every account a personal unit, and every existing
workspace a home. The placement rule is the one that keeps an upgrade from
widening access: an owned workspace lands in its owner's personal unit and never
under the root, including when its owner's account is gone.

New workspaces are placed at creation through a lazy seam rather than a
user-creation hook, so an account created by any path gets a personal unit the
first time it needs one, with no ordering to get wrong.

Verified on a fresh instance: an unowned pre-authentication workspace sits at
the root, and both workspaces created by a signed-in user sit in her personal
unit. Every test is mutation-checked; two mutants that only broke the build were
rerun as valid mutations rather than counted as caught.

RED/GREEN:

- RED: removing the owner branch from the backfill fails
  `TestBackfillDoesNotWidenReach` and
  `TestBackfillIsolatesWorkspacesOfMissingOwners`.
- RED: dropping the descendant rewrite from `Reparent` fails
  `TestReparentRewritesDescendantPaths`.
- GREEN: `go test ./internal/orgunit/... ./internal/task/... ./internal/office/...`
