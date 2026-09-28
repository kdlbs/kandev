---
id: "06-configure-sections"
title: "Coordinator page sections"
status: pending
wave: 4
depends_on:
  - "02-policy-enforcement"
  - "03-activity-log-backend"
  - "05-orders-goals-backend"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COORDINATORS-009
  - REQ-COORDINATOR-PERMISSIONS-001
  - REQ-COORDINATOR-PERMISSIONS-003
  - REQ-COORDINATOR-PERMISSIONS-004
  - REQ-COORDINATOR-STANDING-ORDERS-001
  - REQ-COORDINATOR-GOALS-001
acceptance_criteria:
  - AC-COORDINATOR-COORDINATORS-009.1
  - AC-COORDINATOR-COORDINATORS-009.2
  - AC-COORDINATOR-PERMISSIONS-001.6
  - AC-COORDINATOR-PERMISSIONS-001.7
  - AC-COORDINATOR-PERMISSIONS-001.8
  - AC-COORDINATOR-PERMISSIONS-001.9
  - AC-COORDINATOR-PERMISSIONS-003.6
  - AC-COORDINATOR-PERMISSIONS-004.3
  - AC-COORDINATOR-STANDING-ORDERS-001.4
  - AC-COORDINATOR-STANDING-ORDERS-001.5
  - AC-COORDINATOR-STANDING-ORDERS-001.6
  - AC-COORDINATOR-GOALS-001.9
system_design:
  - ../../specs/coordinator/system-design/coordinators.md
  - ../../specs/coordinator/system-design/permissions.md
  - ../../specs/coordinator/system-design/standing-orders.md
  - ../../specs/coordinator/system-design/goals.md
---

# Task 06: Coordinator Page Sections (WP-7, WP-10)

## Summary

Turn the phase-1 coordinator page into five sections (Identity, Watches, May
do, Standing orders, Goal) with the section in the address, and give each
list card its summary line. May do and Watches save with the phase-1 save
bar; standing orders and the goal save through their own routes.

## In scope

- Sections row with help text and `?section=` in the address (`009.1`);
  Identity holds the phase-1 fields unchanged.
- List summary line from the list's `summary` field (`009.2`).
- May do: six rows in order, disabled Automatic with its note, Stop Denied
  only, two locked "Always human" rows, last-30-days counts from
  `activity/summary`, **Review the last 30 days** to What it did filtered by
  class, and the start-agent note (`001.6` to `001.9`).
- Watches: the `all` switch, the workflow list in workspace order with Put
  in and Take out of scope, the in-form refusal of the last board, and the
  "watches no board" notice with Choose boards (`003.6`, and the Configure
  half of `003.5`).
- Both save through the save bar with "Saving a change starts the next
  conversation fresh." (`004.3`).
- Standing orders: list with number, text, Added, Last applied, Retire, Add
  dialog, empty text, retire toast with Undo for 10 seconds, reader view
  (`STANDING-ORDERS-001.4` to `001.6`).
- Goal: name, due, criteria with checkboxes, Add criterion, Mark milestone
  met, Set goal on an empty form, measures with baseline, direction and "No
  baseline" (`GOALS-001.9`).
- Readers see every section without controls. Six locales.

## Out of scope

- Guided setup (task 07); the Needs you goal note (task 11).

## ASCII UI preview

From [UI-02](plan.md#ui-02-coordinator-page-sections-and-may-do-entry-settings-coordinators-configure),
[UI-03](plan.md#ui-03-watches-section), [UI-04](plan.md#ui-04-standing-orders-section-and-the-reject-offer)
and [UI-05](plan.md#ui-05-goal-section-and-the-goal-note):

```text
< All coordinators                                    Planner
[Identity] [Watches] [May do] [Standing orders] [Goal]
  Create a task    ( ) Denied (*) Requires approval ( ) Automatic   12 approved, 2 rejected
  Stop a task      (*) Denied   Stopping is not available yet.
  Merge a pull request   Always human
  Review the last 30 days
Saving a change starts the next conversation fresh.          [Discard] [Save]

Standing order 1   Prefer small cards.
                   Added 12 Sep . Last applied 2 h ago        [Retire this order]
[Add standing order]
```

Phone: the Sections row scrolls horizontally; each May do action is a
stacked group with a segmented control.

## Mockup screenshots and scenarios

- [`assets/p2-02-settings-coordinator-sections.png`](assets/p2-02-settings-coordinator-sections.png)
- Scenario `18-v21-copilot-anywhere` (settings sections) maps to
  `tests/coordinator/configure-sections.spec.ts`.

## Acceptance

- Each section renders from the stored values and saves through its route.
- A reader sees values and no write control in every section.
- With `phase2` off, the page and list are the phase-1 page and list.

## Verification

```bash
cd apps/web && pnpm test -- app/settings/workspace
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/configure-sections.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/coordinator/configure-sections.spec.ts
```

The E2E spec: set Message to Denied and save; take a board out of scope and
save; add, retire and Undo an order; set a goal and check a criterion;
reload and assert each value; a reader account (auth project) sees no
controls; flag-off shows the phase-1 page.

## Likely files

- `apps/web/app/settings/workspace/[id]/coordinators/[coordinatorId]/page.tsx`
- `apps/web/app/settings/workspace/[id]/coordinators/components/`
  `sections-row.tsx`, `may-do-section.tsx`, `watches-section.tsx`,
  `standing-orders-section.tsx`, `goal-section.tsx`
- `apps/web/app/settings/workspace/[id]/coordinators/page.tsx` (summary)
- `apps/web/src/locales/*/coordinator.json`, `eslint.i18n.options.mjs`

## Dependencies

- Task 02 (settings routes); task 03 (summary); task 05 (orders and goal
  routes).

## Risks

- Four independent forms on one page: only May do and Watches share the
  save bar; the others save on their own action, and leaving with unsaved
  May do changes uses the phase-1 guard.
