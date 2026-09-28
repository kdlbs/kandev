---
id: "11-needs-you-goal-watches"
title: "Goal note and Watches filtering in Needs you"
status: pending
wave: 4
depends_on:
  - "01-shared-interface"
  - "02-policy-enforcement"
  - "05-orders-goals-backend"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-GOALS-002
  - REQ-COORDINATOR-PERMISSIONS-003
acceptance_criteria:
  - AC-COORDINATOR-GOALS-002.1
  - AC-COORDINATOR-GOALS-002.2
  - AC-COORDINATOR-GOALS-002.3
  - AC-COORDINATOR-PERMISSIONS-003.4
system_design:
  - ../../specs/coordinator/system-design/goals.md
  - ../../specs/coordinator/system-design/permissions.md
---

# Task 11: Goal Note and Watches Filtering in Needs You (WP-10, WP-7)

## Summary

Filter Needs you, Queue and the count strip to watched tasks, keep the
coordinator's own proposals, show the goal note above Needs you, and show the
"watches no board" notice.

## In scope

- Server projections filter tasks and stalls by the `WatchSet`; proposals
  always show; the count strip counts the same set (`PERMISSIONS-003.4`).
- Needs you "watches no board" notice with a Watches link for managers (the
  Needs you half of `003.5`).
- Goal note: no goal with Set a goal, active goal with due date, overdue in
  the viewer's time zone and "N of M criteria met", met with Set the next
  goal; readers see no buttons (`GOALS-002.1` to `002.3`).

## Out of scope

- The Goal section (task 06).

## ASCII UI preview

From [UI-05](plan.md#ui-05-goal-section-and-the-goal-note):

```text
Goal note, top of Needs you
  No goal:  "No goal is set, so this list is ordered by urgency alone." [Set a goal]
  Active:   "Ship the billing beta . Due 31 Oct . 1 of 2 criteria met"
  Met:      "Ship the billing beta was met on 28 Oct." [Set the next goal]
```

Phone: the note wraps above the list; the button is full width.

## Mockup screenshots and scenarios

- No screenshot; UI-05 is the reference. No mockup scenario;
  `tests/coordinator/needs-you-goal.spec.ts` is new.

## Acceptance

- A task in an unwatched workflow never appears or counts; a proposal about
  it still shows.
- The note shows the state that matches the stored goal.

## Verification

```bash
make -C apps/backend test PKG=./internal/coordinator/...
cd apps/web && pnpm test -- app/coordinator/components/goal-note
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/needs-you-goal.spec.ts
```

Backend: projection tests with `all`, `selected` and an empty list. Unit:
the overdue check across a time-zone boundary. E2E: seed two boards, watch
one, assert counts; set a goal, check a criterion, mark it met, assert each
note.

## Likely files

- `apps/backend/internal/coordinator/projections.go`, `counts.go`
- `apps/web/app/coordinator/components/goal-note.tsx`,
  `watches-none-notice.tsx`

## Dependencies

- Task 01 (Watches store); task 02 (settings route, used by the E2E spec
  to choose Watches); task 05 (goal routes).

## Risks

- The count strip and the list must use one filter function so they never
  disagree.
