---
id: "06-e2e-and-docs"
title: "End-to-end coverage and public documentation"
status: done
wave: 5
depends_on: ["05-remove-visibility"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/org-units.md"
---

# Task 06: End-to-End Coverage and Public Documentation

## Outcome

The user-facing outcome has browser evidence, and the public documentation
describes units rather than visibility.

## In scope

- An end-to-end spec covering the inherited-reach outcome: a department member
  opens a team board they were never named on, and cannot open a workspace in
  another user's personal unit.
- An end-to-end spec covering placement: moving a workspace changes who reaches
  it.
- Rewriting `docs/public/team-access.md` around the tree, with screenshots
  captured from a seeded instance.
- Extending the demo seed script to build a small department and team tree.

## Out of scope

- Inherited configuration such as executors or secrets, which the requirement
  excludes.

## Requirements

`REQ-WORKSPACES-ORG-UNITS-002`, `REQ-WORKSPACES-ORG-UNITS-003`,
`REQ-WORKSPACES-ORG-UNITS-005`.

Acceptance criteria: `AC-WORKSPACES-ORG-UNITS-002.2`,
`AC-WORKSPACES-ORG-UNITS-003.3`, `AC-WORKSPACES-ORG-UNITS-005.3`.

## System design

[Organization units](../../specs/workspaces/system-design/org-units.md).

## Implementation acceptance

1. The end-to-end specs pass against a seeded tree.
2. The public page contains no visibility instructions and its screenshots show
   the tree.
3. `/docs-maintainer` reports no stale public references.

## Verification

```bash
cd apps/web
pnpm run build:e2e
pnpm e2e:run tests/access/org-units.spec.ts
cd .. && python3 scripts/lint-spec-files.py --all
```

## Likely files

- `apps/web/e2e/tests/access/org-units.spec.ts` (new)
- `docs/public/team-access.md`, `docs/screenshots/`
- `scripts/demo_team_access_seed.py`

## Results

Done. `e2e/tests/auth/org-units.spec.ts` runs in the `auth` project and covers
the outcome the tree exists for: a member of a department reaches a board in a
team beneath it with no invitation, moving that board into a personal unit
withdraws the reach, a member cannot rearrange the organization but can read
it, and the tree renders in settings.

`docs/public/team-access.md` is rewritten around units, with screenshots
captured from a seeded instance through `scripts/team-access-docs-shots.mjs`.
The demo seed builds a Platform department with a Runtime team rather than
flipping a visibility switch.

GREEN: `pnpm e2e:raw --project=auth -g "organization units"` reports 5 passed.
