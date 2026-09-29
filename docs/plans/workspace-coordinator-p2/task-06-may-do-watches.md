---
id: "06-may-do-watches"
title: "May do and Watches"
status: pending
wave: 5
depends_on:
  - "02-policy-enforcement"
  - "03-activity-log-backend"
  - "11-orders-goal-sections"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COORDINATORS-009
  - REQ-COORDINATOR-PERMISSIONS-001
  - REQ-COORDINATOR-PERMISSIONS-003
  - REQ-COORDINATOR-PERMISSIONS-004
acceptance_criteria:
  - AC-COORDINATOR-COORDINATORS-009.2
  - AC-COORDINATOR-PERMISSIONS-001.6
  - AC-COORDINATOR-PERMISSIONS-001.7
  - AC-COORDINATOR-PERMISSIONS-001.8
  - AC-COORDINATOR-PERMISSIONS-001.9
  - AC-COORDINATOR-PERMISSIONS-001.10
  - AC-COORDINATOR-PERMISSIONS-003.4
  - AC-COORDINATOR-PERMISSIONS-003.5
  - AC-COORDINATOR-PERMISSIONS-003.6
  - AC-COORDINATOR-PERMISSIONS-003.7
  - AC-COORDINATOR-PERMISSIONS-004.3
system_design:
  - ../../specs/coordinator/system-design/coordinators.md
  - ../../specs/coordinator/system-design/permissions.md
---

# Task 06: May Do and Watches (WP-7)

## Summary

Add the May do and Watches sections to the coordinator page, give each list
card its summary line, and filter Needs you, Queue and the count strip to
watched tasks. May do and Watches save with the phase-1 save bar.

## In scope

- May do: six rows in order, disabled Automatic with its note, Stop Denied
  only, two locked "Always human" rows, last-30-days counts from
  `activity/summary`, **Review the last 30 days** to What it did filtered by
  class, and the start-agent note (`PERMISSIONS-001.6` to `001.9`).
- Watches: the `all` switch, the workflow list in workspace order with Put
  in and Take out of scope, the in-form refusal of the last board, and the
  "watches no board" notice with Choose boards (`003.6`, and the Configure
  half of `003.5`).
- Both save through the save bar with "Saving a change starts the next
  conversation fresh." (`004.3`).
- Both sections are entries (`watches`, `may-do`) in task 11's Sections row.
- List summary line from the list's `summary` field (`COORDINATORS-009.2`).
- The client watch filter: one pure module `apps/web/lib/coordinator/watch-filter.ts`
  filters tasks and stalls for `classify` and the toast count from the
  effective watch set of the settings GET; proposals always show, the
  proposal source-task lookup and What it did stay unfiltered, the sidebar
  badge (`open_proposals`) is unfiltered; the watch set input fails closed
  with its own banner line (`PERMISSIONS-003.4`, `003.7`). Needs you shows the
  "watches no board" notice with a Watches link for managers (the Needs you
  half of `003.5`).
- Backend: the list DTO's `summary` (`watch_scope`, `watched_count`,
  `approval_actions`, `active_orders`; no other task adds it), with tests for
  `all`, `selected` and an empty effective set.
- One shared draft hook `useControlDraft` owned by `CoordinatorSections`, one
  save contributor and one PUT carrying only changed members (`004.3`).
- Readers see both sections without controls. Six locales.

## Out of scope

- The Sections row, Standing orders and Goal sections and the goal note
  (task 11); guided setup (task 07).

## ASCII UI preview

From [UI-02](plan.md#ui-02-coordinator-page-sections-and-may-do-entry-settings-coordinators-configure)
and [UI-03](plan.md#ui-03-watches-section):

```text
< All coordinators                                    Planner
[Identity] [Watches] [May do] [Standing orders] [Goal]
  Create a task    ( ) Denied (*) Requires approval ( ) Automatic   12 approved, 2 rejected
  Stop a task      (*) Denied   Stopping is not available yet.
  Merge a pull request   Always human
  Review the last 30 days
Saving a change starts the next conversation fresh.          [Discard] [Save]
```

Phone: each May do action is a stacked group with a segmented control.

## Mockup screenshots and scenarios

- [`assets/p2-02-settings-coordinator-sections.png`](assets/p2-02-settings-coordinator-sections.png)
- Scenario `18-v21-copilot-anywhere` (settings sections) maps to
  `tests/coordinator/configure-sections.spec.ts`, whose May do and Watches
  cases this task adds.

## Acceptance

- Each section renders from the stored values and saves through its route.
- A reader sees values and no write control.
- A task in an unwatched workflow never appears or counts; a proposal about
  it still shows.
- With `phase2` off, the page, list and Needs you are the phase-1 ones.

## Verification

```bash
make -C apps/backend test PKG=./internal/coordinator/...
cd apps/web && pnpm test -- components/coordinators app/coordinator lib/coordinator
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/configure-sections.spec.ts
cd apps/web && pnpm e2e:run tests/coordinator/mobile-configure-sections.spec.ts
cd apps/web && pnpm e2e:run tests/coordinator/needs-you-watches.spec.ts
cd apps/web && pnpm e2e:run tests/auth/coordinator-settings-reader.spec.ts
```

Backend: list `summary` tests with `all`, `selected` and an empty
effective set; Vitest for the watch filter, the draft hook (one PUT, only
changed members, unvisited section) and the summary line plurals. E2E:
set Message to Requires approval and save (Message is Denied under the
phase-1 policy, so Denied would not dirty the form); take a board out of
scope and save; reload and assert each value; the Review link lands on
What it did filtered by class; seed two boards, watch one, assert Needs you counts;
a reader account sees no controls (`pnpm e2e:run tests/auth/coordinator-settings-reader.spec.ts`, the auth project matches only `auth/`; the mobile project matches only `mobile-*` files); flag-off shows the
phase-1 page.

## Likely files

- `apps/web/components/coordinators/sections/` `may-do-section.tsx`,
  `watches-section.tsx`, `control-draft.ts(x)`, `coordinator-sections.tsx`
- `apps/web/components/coordinators/coordinators-list-page.tsx` (summary line)
- `apps/web/lib/coordinator/watch-filter.ts`,
  `apps/web/app/coordinator/use-coordinator-attention.ts`,
  `use-coordinator-watch-set.ts`
- `apps/backend/internal/coordinator/` list query and DTO (`summary`)
- `apps/web/app/coordinator/components/watches-none-notice.tsx`
- `apps/web/src/locales/*/coordinator.json`, `eslint.i18n.options.mjs`

## Dependencies

- Task 02 (settings routes); task 03 (summary); task 11 (Sections row).

## Risks

- The count strip and the list must use one filter function so they never
  disagree.
- Leaving with unsaved May do changes uses the phase-1 guard; Standing
  orders and Goal save on their own actions.
