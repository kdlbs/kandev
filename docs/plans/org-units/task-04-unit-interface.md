---
id: "04-unit-interface"
title: "Unit tree and placement interface"
status: done
wave: 3
depends_on: ["03-unit-management-api"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/org-units.md"
---

# Task 04: Unit Tree and Placement Interface

## Outcome

An organization administrator can build the tree, manage a unit's members, and
move a workspace between units, on desktop and on a phone.

## In scope

- A unit tree in settings under Access Control, with create, rename, move and
  delete.
- A unit members view reusing the existing member picker and role control.
- Workspace placement shown on the workspace, with a move control gated on the
  caller's scopes.
- Copy in all five locales.

## Out of scope

- Removing the visibility control, order 05.
- Screenshots and public documentation, order 06.

## Requirements

`REQ-WORKSPACES-ORG-UNITS-001`, `REQ-WORKSPACES-ORG-UNITS-002`,
`REQ-WORKSPACES-ORG-UNITS-005`.

Acceptance criteria: `AC-WORKSPACES-ORG-UNITS-001.8`,
`AC-WORKSPACES-ORG-UNITS-002.1`, `AC-WORKSPACES-ORG-UNITS-005.2`.

## System design

[Organization units](../../specs/workspaces/system-design/org-units.md), section
Components and responsibilities.

## Implementation acceptance

1. The tree renders an organization's units in hierarchy order and hides
   controls the caller's scopes do not permit.
2. Moving a workspace updates its displayed placement without a reload.
3. The interface works at a phone viewport, per `/mobile-parity`.

## Verification

```bash
cd apps/web
pnpm run typecheck && pnpm run lint
pnpm test -- components/settings/units lib/api/domains/org-units-api.test.ts
pnpm run i18n:check && pnpm run i18n:ratchet
```

## Likely files

- `apps/web/components/settings/units/` (new)
- `apps/web/lib/api/domains/org-units-api.ts` (new)
- `apps/web/components/app-sidebar/sections/settings/settings-menu-sections.ts`
- `apps/web/src/locales/*/settings.json`

## Results

Done. `Settings > Access Control > Organization units` renders the tree with a
row per unit, indented by depth derived from the materialized path, and offers
create, delete and member management. A personal unit and the root show no
control that would only ever be refused: a personal unit takes no members and
cannot be deleted, and deleting the root would leave every workspace beneath it
homeless.

Copy ships in all five locales.

A gap found later while auditing screenshots: this order claimed workspace
placement was shown on the workspace, and it was not. The API and the e2e
coverage existed, but nothing in the interface could move a workspace, which is
the only way to narrow access in this design. `WorkspacePlacementCard` closes
it, gated on `workspace.manage` and restoring the previous unit when a move is
refused.

RED/GREEN:

- RED: treating a personal unit as an ordinary one fails
  "offers no member or delete control on a personal unit".
- GREEN: `pnpm test -- components/settings/units lib/api/domains/org-units-api.test.ts`
- GREEN: typecheck, eslint, i18n:check, i18n:ratchet.
