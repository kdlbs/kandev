---
spec: docs/specs/workspaces/requirements/org-units.md
created: 2026-08-24
updated: 2026-08-24
status: draft
---

# Organization Units Plan

## Scope

Implement [organization units](../../specs/workspaces/requirements/org-units.md)
against its [system design](../../specs/workspaces/system-design/org-units.md):
a per-organization tree of units that carries child units, workspaces, and
members; membership that inherits downward; personal units in place of private
visibility; and an effective role resolved as the maximum of inherited and
direct grants.

This **replaces** workspace visibility. `workspaces.visibility`, the
organization default-visibility setting, and the bulk visibility endpoint are
removed rather than kept alongside the tree. There is no compatibility path and
no second reach mechanism.

Unchanged and consumed as they are: the tenant boundary in `internal/org`, and
the scope registry and role-to-scope mapping in `internal/authz`, which gains
one scope (`unit.manage`).

## Current state

The branch currently implements visibility (`org` / `private`) with membership
as an exception, resolved in `internal/authz.ResolveWorkspace`. Reach is one
column plus one membership table. The human assignee and actor attribution are
implemented and are unaffected by this plan; they move to
[human assignee](../../specs/tasks/requirements/human-assignee.md) in the task
system, which owns them.

## Delivery order

Each wave leaves a working product. The tree is built first and made
authoritative before the mechanism it replaces is removed, so no wave depends on
a half-migrated reach model.

| Wave | Work orders | Outcome |
| --- | --- | --- |
| 1 | 01 | The tree exists and every workspace has a placement. Reach still comes from visibility. |
| 2 | 02 | Reach comes from the tree. Visibility is no longer consulted. |
| 3 | 03, 04 | Units and placement are manageable through the API and the interface. |
| 4 | 05 | The replaced mechanism is gone from schema, API, interface, and docs. |
| 5 | 06 | End-to-end evidence and public documentation. |

## Work orders

- [x] [01 — Unit tree storage and placement](task-01-unit-tree-storage.md)
- [x] [02 — Reach resolution on the tree](task-02-reach-resolution.md)
- [x] [03 — Unit management API](task-03-unit-management-api.md)
- [x] [04 — Unit tree and placement interface](task-04-unit-interface.md)
- [x] [05 — Remove workspace visibility](task-05-remove-visibility.md)
- [x] [06 — End-to-end coverage and public documentation](task-06-e2e-and-docs.md)

## Risks

- **A table rebuild drops the new columns.** `internal/office`'s
  priority-to-TEXT migration recreates `tasks`, and other migrations recreate
  `workspaces`. Any rebuild must carry `unit_id`. This already caused a silent
  column loss on this branch once, so wave 1 owns a regression test at the
  rebuild itself.
- **The migration widens reach.** Placement must map a private workspace to its
  owner's personal unit, never to the root. Wave 1 owns a test asserting that no
  workspace becomes reachable by a user who could not reach it before.
- **Stale reach on connected clients.** Moving a workspace or removing a
  membership changes reach for people who are already subscribed. Waves 2 and 3
  reuse the existing revocation path rather than adding a second one.
- **Path denormalization drifts.** `org_units.path` has exactly one writer. A
  move rewrites descendants in one transaction, and wave 1 owns a test that
  reparents a subtree and asserts every descendant path.

## Verification strategy

- Wave 1 and 2 are covered by Go unit and repository tests, including a
  migration test that asserts no reach widening and a rebuild-survival test.
- Wave 3 and 4 are covered by handler tests for authorization and refusal codes,
  and by frontend unit tests for the tree and placement controls.
- Wave 5 is covered by a repository-wide assertion that no `visibility`
  reference remains in schema, API, or interface code.
- Wave 6 owns the end-to-end evidence: a seeded instance where a department
  member reaches a team board they were never named on, and a personal
  workspace stays unreachable.
