---
id: "11-orders-goal-sections"
title: "Sections row, standing orders, goal and goal note"
status: pending
wave: 4
depends_on:
  - "05-standing-orders-backend"
  - "12-goals-backend"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COORDINATORS-009
  - REQ-COORDINATOR-STANDING-ORDERS-001
  - REQ-COORDINATOR-GOALS-001
  - REQ-COORDINATOR-GOALS-002
  - REQ-COORDINATOR-GOALS-003
acceptance_criteria:
  - AC-COORDINATOR-COORDINATORS-009.1
  - AC-COORDINATOR-STANDING-ORDERS-001.4
  - AC-COORDINATOR-STANDING-ORDERS-001.5
  - AC-COORDINATOR-STANDING-ORDERS-001.6
  - AC-COORDINATOR-GOALS-001.9
  - AC-COORDINATOR-GOALS-001.10
  - AC-COORDINATOR-GOALS-002.1
  - AC-COORDINATOR-GOALS-002.2
  - AC-COORDINATOR-GOALS-002.3
  - AC-COORDINATOR-GOALS-003.4
system_design:
  - ../../specs/coordinator/system-design/coordinators.md
  - ../../specs/coordinator/system-design/standing-orders.md
  - ../../specs/coordinator/system-design/goals.md
---

# Task 11: Sections Row, Standing Orders, Goal and Goal Note (WP-7, WP-10)

## Summary

Turn the phase-1 coordinator page into sections with the section in the
address, add the Standing orders and Goal sections, and show the goal note
above Needs you. Standing orders and the goal save through their own routes.

## In scope

- Sections row (Identity, Watches, May do, Standing orders, Goal) with help
  text and `?section=` in the address (`COORDINATORS-009.1`); Identity holds
  the phase-1 fields unchanged. Watches and May do are task 06's; until it
  lands their entries are not shown.
- Standing orders: list with number, text, Added, Last applied, Retire, Add
  dialog, empty text, retire toast with Undo for 10 seconds, reader view
  (`STANDING-ORDERS-001.4` to `001.6`).
- Goal: name, due, criteria with checkboxes, Add criterion, Mark milestone
  met, Set goal on an empty form, measures with baseline, direction and "No
  baseline" (`GOALS-001.9`).
- Goal note: no goal with Set a goal, active goal with due date, overdue in
  the viewer's time zone and "N of M criteria met", met with Set the next
  goal; readers see no buttons (`GOALS-002.1` to `002.3`).
- Readers see every section without controls. Six locales.

- Web client and hooks: the standing-order and goal typed functions and types
  in `lib/api/domains/coordinator-api.ts` (list with `include=retired`, add,
  retire, restore, get goal, put goal, toggle criterion, mark met), and the
  `use-standing-orders.ts` and `use-goal.ts` hooks under
  `hooks/domains/coordinator/`. Task 01 lists these but they are absent on the
  branch; task 09 consumes `use-standing-orders.ts`.
- `AddStandingOrderDialog` (`components/coordinators/add-standing-order-dialog.tsx`)
  is the single add component (a dialog, not an inline form). Whichever of
  tasks 09 and 11 lands second imports the file the first created.
- Error, empty, loading, concurrency and `?section=` behaviour are those in the
  three system designs' UI sections, which bind this task.

## Out of scope

- May do, Watches and Watches filtering (task 06); guided setup (task 07).

## ASCII UI preview

From [UI-02](plan.md#ui-02-coordinator-page-sections-and-may-do-entry-settings-coordinators-configure),
[UI-04](plan.md#ui-04-standing-orders-section-and-the-reject-offer)
and [UI-05](plan.md#ui-05-goal-section-and-the-goal-note):

```text
< All coordinators                                    Planner
[Identity] [Watches] [May do] [Standing orders] [Goal]
Standing order 1   Prefer small cards.
                   Added 12 Sep . Last applied 2 h ago        [Retire this order]
[Add standing order]

Goal note, top of Needs you
  Active:   "Ship the billing beta . Due 31 Oct . 1 of 2 criteria met"
```

Phone: the Sections row scrolls horizontally; the note wraps above the list
and its button is full width.

## Mockup screenshots and scenarios

- [`assets/p2-02-settings-coordinator-sections.png`](assets/p2-02-settings-coordinator-sections.png)
- Scenario `18-v21-copilot-anywhere` (settings sections) maps to
  `tests/coordinator/configure-sections.spec.ts`, which does not exist yet:
  this task creates it with the Sections row, Standing orders and Goal cases
  (task 06 adds its own cases to it). The goal note has no
  scenario; `tests/coordinator/needs-you-goal.spec.ts` is new.

## Acceptance

- Each section renders from the stored values and saves through its route.
- The note shows the state that matches the stored goal.
- A reader sees values and no write control.
- With `phase2` off, the page and Needs you are the phase-1 ones.

## Verification

```bash
cd apps/web && pnpm test -- app/settings/workspace app/coordinator/components/goal-note
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/configure-sections.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/coordinator/configure-sections.spec.ts
cd apps/web && pnpm e2e:run tests/coordinator/needs-you-goal.spec.ts
```

Unit: the overdue check across a time-zone boundary. E2E: open a section by
address; add, retire and Undo an order; set a goal, check a criterion, mark
it met, and assert the section and each note after a reload; a reader
account (auth project) sees no controls; flag-off shows the phase-1 page.

## Likely files

- `apps/web/components/coordinators/coordinator-editor-page.tsx` (the page's
  body; `app/settings/workspace/[id]/coordinators/[coordinatorId]/page.tsx`
  only wraps it)
- `apps/web/components/coordinators/sections/` `sections-row.tsx`,
  `standing-orders-section.tsx`, `goal-section.tsx`, and
  `components/coordinators/add-standing-order-dialog.tsx`
- `apps/web/lib/api/domains/coordinator-api.ts`,
  `apps/web/hooks/domains/coordinator/use-standing-orders.ts`, `use-goal.ts`
- `apps/web/app/coordinator/components/goal-note.tsx`
- `apps/web/e2e/tests/coordinator/configure-sections.spec.ts` (new),
  `needs-you-goal.spec.ts` (new)
- `apps/web/src/locales/*/coordinator.json`, `eslint.i18n.options.mjs`

## Dependencies

- Task 05 (order routes); task 12 (goal routes and measures).

## Risks

- The Sections row is shared with task 06: it takes a list of sections so
  task 06 adds entries without editing this task's components.
