---
id: "07-guided-setup"
title: "Guided setup"
status: pending
wave: 6
depends_on:
  - "06-may-do-watches"
  - "11-orders-goal-sections"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COORDINATORS-008
acceptance_criteria:
  - AC-COORDINATOR-COORDINATORS-008.1
  - AC-COORDINATOR-COORDINATORS-008.2
  - AC-COORDINATOR-COORDINATORS-008.3
  - AC-COORDINATOR-COORDINATORS-008.4
  - AC-COORDINATOR-COORDINATORS-008.5
  - AC-COORDINATOR-COORDINATORS-008.6
system_design:
  - ../../specs/coordinator/system-design/coordinators.md
---

# Task 07: Guided Setup (WP-7)

## Summary

Replace the phase-1 new-coordinator form with a six-step setup that reuses
the section forms of tasks 06 and 11, ends on a "What it wrote" review, and creates the
coordinator, settings, Watches and goal in one transaction through
`POST .../coordinators/setup`.

## In scope

- `POST .../coordinators/setup`: validates the whole payload with the same
  validators as each route and inserts all rows in one transaction or none
  ([design](../../specs/coordinator/system-design/coordinators.md#guided-setup)).
- Steps: Who runs it, What it watches (`all` preselected), What it is for
  (goal form, Skip this step), What it knows (context, Skip this step), What
  it may do (four Requires approval, `start_agent` and `stop` Denied),
  Review (`008.1`, `008.2`).
- Review table with Setting, Value, Owned from now on by and Change back to
  the step with values kept (`008.3`).
- Finish enabled only when valid; success opens the new Configure page
  (`008.4`). Leaving creates nothing; readers never reach it (`008.5`).
- State held in the page only; no draft is stored.
- First task inside this work order: make the Watches, Goal and May do
  sections controlled components (values and handlers passed in; the stored
  reads, activity counts, Review link and goal actions stay in the Configure
  wrappers) with regression tests on the Configure page before the setup
  steps use them.
- Finish banners: nothing created when the server answered, could not
  confirm when no answer arrived (`008.6`).

## Out of scope

- Editing after creation (the sections of tasks 06 and 11 own it).

## ASCII UI preview

From [UI-06](plan.md#ui-06-guided-setup-entry-add-coordinator):

```text
Add coordinator
 (1) Who runs it  (2) What it watches  (3) What it is for  (4) What it knows
 (5) What it may do  (6) Review
 | Setting | Value                       | Owned from now on by |
 | Name    | Planner                     | Identity   [Change]  |
 | Watches | Product, Release            | Watches    [Change]  |
                                                   [Back]  [Finish]
```

Phone: the step list collapses to "Step 3 of 6" with the step name.

## Mockup screenshots and scenarios

- No screenshot; UI-06 is the reference.
- Scenario `14-first-run-setup` (coordinated part) maps to
  `tests/coordinator/guided-setup.spec.ts`.

## Acceptance

- Finish creates everything or nothing; a fault in the goal insert leaves no
  coordinator.
- Change returns to the step with every value kept.

## Verification

```bash
make -C apps/backend test PKG=./internal/coordinator/...
cd apps/web && pnpm test -- app/settings/workspace/\[id\]/coordinators/new
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/guided-setup.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/coordinator/guided-setup.spec.ts
```

Backend: a setup with an invalid goal is 400 and inserts nothing; a
fault-injected Watches insert rolls back the coordinator. E2E: complete the
setup with two boards and a goal, use Change from Review, Finish, and assert
the Configure page shows the values; navigate away mid-setup and assert the
list is unchanged.

## Likely files

- `apps/backend/internal/coordinator/setup_routes.go`
- `apps/web/app/settings/workspace/[id]/coordinators/new/page.tsx`
- `apps/web/app/settings/workspace/[id]/coordinators/components/setup-*.tsx`

## Dependencies

- Tasks 06 and 11 (section forms reused as steps).

## Risks

- The flag-off path must still render the phase-1 form; the setup component
  is chosen by `phase2` at the page.
