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
  - AC-COORDINATOR-PERMISSIONS-003.4
  - AC-COORDINATOR-PERMISSIONS-003.6
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
- Both sections are entries in task 11's Sections row.
- List summary line from the list's `summary` field (`COORDINATORS-009.2`).
- Server projections filter tasks and stalls by the `WatchSet`; proposals
  always show; the count strip counts the same set (`PERMISSIONS-003.4`).
  Needs you "watches no board" notice with a Watches link for managers (the
  Needs you half of `003.5`).
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
cd apps/web && pnpm test -- app/settings/workspace app/coordinator
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/configure-sections.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/coordinator/configure-sections.spec.ts
cd apps/web && pnpm e2e:run tests/coordinator/needs-you-watches.spec.ts
```

Backend: projection tests with `all`, `selected` and an empty list. E2E:
set Message to Denied and save; take a board out of scope and save; reload
and assert each value; seed two boards, watch one, assert Needs you counts;
a reader account (auth project) sees no controls; flag-off shows the
phase-1 page.

## Likely files

- `apps/web/app/settings/workspace/[id]/coordinators/components/`
  `may-do-section.tsx`, `watches-section.tsx`
- `apps/web/app/settings/workspace/[id]/coordinators/page.tsx` (summary)
- `apps/backend/internal/coordinator/projections.go`, `counts.go`
- `apps/web/app/coordinator/components/watches-none-notice.tsx`
- `apps/web/src/locales/*/coordinator.json`, `eslint.i18n.options.mjs`

## Dependencies

- Task 02 (settings routes); task 03 (summary); task 11 (Sections row).

## Risks

- The count strip and the list must use one filter function so they never
  disagree.
- Leaving with unsaved May do changes uses the phase-1 guard; Standing
  orders and Goal save on their own actions.
