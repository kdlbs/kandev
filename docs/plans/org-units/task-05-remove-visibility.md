---
id: "05-remove-visibility"
title: "Remove workspace visibility"
status: done
wave: 4
depends_on: ["04-unit-interface"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/org-units.md"
---

# Task 05: Remove Workspace Visibility

## Outcome

The mechanism the tree replaced is gone. There is one reach model in the schema,
the API, the interface, and the documentation.

## In scope

- Dropping `workspaces.visibility` and the organization default-visibility
  setting.
- Removing `POST /api/v1/workspaces/visibility/bulk` and the `visibility` field
  from workspace DTOs and payloads.
- Removing the visibility control, its store state, and its copy from the
  interface, including locale entries that become orphans.
- Removing visibility from `docs/public/team-access.md` and from the coverage
  manifest.

## Out of scope

- New public documentation for units, order 06.

## Requirements

`REQ-WORKSPACES-ORG-UNITS-004`.

Acceptance criterion: `AC-WORKSPACES-ORG-UNITS-004.1`, in that the resolver has
one input path after this order.

## System design

[Organization units](../../specs/workspaces/system-design/org-units.md), section
Purpose and boundaries.

## Implementation acceptance

1. No `visibility` symbol remains in backend schema, services, handlers, or
   frontend state for workspaces.
2. Every suite passes with the column absent.
3. `pnpm run i18n:check` reports no orphaned visibility copy.

## Verification

```bash
cd apps/backend && go test ./... && make lint
cd ../web && pnpm run typecheck && pnpm run lint && pnpm test && pnpm run i18n:check
rg -n 'workspace.*visibility|default_workspace_visibility' apps/ docs/public || true
```

## Likely files

- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/task/dto/dto.go`
- `apps/web/components/settings/workspaces/`
- `docs/public/team-access.md`, `docs/public/coverage.json`

## Results

Done. `workspaces.visibility`, the organization default-visibility setting, its
bulk endpoint, the service surface, the DTO field, the store state and the
settings control are gone. There is one reach model in the schema, the API, the
interface and the documentation.

Two defects surfaced while removing it, both found by driving the running
instance rather than by a test:

- The workspace `UPDATE` statement is hand-written and never carried `unit_id`,
  so a move returned the new placement and persisted nothing. Pinned by
  `TestUpdateWorkspacePersistsUnitPlacement`.
- The human-assignee reach check still asked only about a direct membership
  row, so it refused a colleague who plainly reached the board through the
  tree. Its refusal was also mapped to 500 on the task route rather than 400.

RED/GREEN:

- RED: dropping `unit_id` from the update statement fails
  `TestUpdateWorkspacePersistsUnitPlacement`.
- GREEN: `go test ./internal/...` 239 packages, 0 failures; `make lint` 0 issues.
- GREEN: typecheck, eslint, i18n:check across the web app.
