---
id: "08-what-it-did-ui"
title: "What it did"
status: pending
wave: 3
depends_on:
  - "03-activity-log-backend"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-ACTIVITY-LOG-002
  - REQ-COORDINATOR-ACTIVITY-LOG-003
acceptance_criteria:
  - AC-COORDINATOR-ACTIVITY-LOG-002.1
  - AC-COORDINATOR-ACTIVITY-LOG-002.2
  - AC-COORDINATOR-ACTIVITY-LOG-002.3
  - AC-COORDINATOR-ACTIVITY-LOG-002.4
  - AC-COORDINATOR-ACTIVITY-LOG-002.5
  - AC-COORDINATOR-ACTIVITY-LOG-002.6
  - AC-COORDINATOR-ACTIVITY-LOG-003.1
    - AC-COORDINATOR-ACTIVITY-LOG-003.6
  - AC-COORDINATOR-ACTIVITY-LOG-003.7

system_design:
  - ../../specs/coordinator/system-design/activity-log.md
---

# Task 08: What It Did (WP-6)

## Summary

Add the What it did section below the Queue groups: newest-first rows with
paging, a class filter kept in the address, authorisation text, Undo for
managers, and the undone state.

## In scope

- Section and route wiring (`002.1`, `002.2` client half with cursor
  paging and Load more).
- Class filter with `?class=` (`002.3`); the May do link from task 06
  lands here.
- Row: relative time, action, class, how it was authorised (policy,
  approver, "with edits", refused count) (`002.4`).
- Empty and filtered-empty texts (`002.5`); reader view without Undo
  (`002.6`).
- "No undo" on every `message` or `resume` class row whatever its outcome,
  nothing on other non-undoable rows (`003.1`).
- Undo button on undoable rows; the "Undo this?" confirmation dialog of
  the design's Undo column (effect text per class, Undo and Cancel, Cancel
  focused, Cancel or Escape sends nothing); and the refusal handling of
  the design's Undo column: `already_undone` refetches with no error text,
  `not_undoable` shows "This can no longer be undone", `undo_conflict` shows the text of its `reason` (`moved`, `archived` or an unknown or absent reason "It has moved since"; `agent_running`, `step_deleted`, `step_done`, `step_full` each their own text, `003.7`), other errors show "Undo failed. Try again."
  and keep the button; "Undone by <name>, <time>" on the original
  reversed row from `undone_by_name` and `undone_at`, and nothing in the
  Undo cell of the `undone` outcome row (`003.1`, `003.6`).
- Refresh on `coordinator.updated`. Six locales.

## Out of scope

- Undo semantics (task 03).

## ASCII UI preview

From [UI-01](plan.md#ui-01-what-it-did-entry-queue-below-the-groups):

```text
What it did                                        Action class [All      v]
| When       | Action                         | Action class | How it was authorised | Undo               |
| 2 min ago  | Created KAN-431 Retry webhooks | Create task  | Approved by Ana, with edits | [Undo]       |
| 1 h ago    | Moved KAN-411 Build -> Review  | Move task    | Approved by Ana       | It has moved since |
| 5 h ago    | Refused: not allowed by May do | Message task | Denied  x 3           | No undo            |
                                  [Load more]
```

Phone: each row is a card; Undo is a full-width button.

## Mockup screenshots and scenarios

- [`assets/p2-01-queue-what-it-did.png`](assets/p2-01-queue-what-it-did.png)
- No mockup scenario; `tests/coordinator/what-it-did.spec.ts` is new.

## Acceptance

- Rows render every row kind the log can hold, in time order, with paging.
- Undo is offered only where the server says it is undoable, and a conflict
  shows its text without removing the row.
- A component test mocks each undo refusal: `already_undone` shows no error
  and refetches to "Undone by", `not_undoable` shows its text and `undo_conflict` shows one text per reason (all six reasons plus an absent one), and a 500 shows "Undo failed. Try again." with the button
  still there.
- A component test renders a `proposed` message row and a `refused`
  message row and asserts "No undo" on both, and nothing in the Undo cell
  of a `rejected` create row.
- A component test renders an undone create row (approver Ana,
  `undone_by_name` Bo) and its `undone` outcome row, and asserts "Undone by
  Bo" on the original row, no Undo on it, and an empty Undo cell on the
  `undone` row.
- A component test clicks Undo on a create row and asserts the dialog
  "Undo this?" with "The task <identifier> will be archived." and focus on
  Cancel; Cancel sends no request; confirming sends one undo request.

## Verification

```bash
cd apps/web && pnpm test -- app/coordinator/queue/what-it-did
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/what-it-did.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/coordinator/what-it-did.spec.ts
```

E2E: the mock agent proposes a task, the manager approves, the row appears;
Undo archives the task and the row reads "Undone by"; filter to Move shows
the filtered-empty text; a reader sees rows and no Undo.

## Likely files

- `apps/web/app/coordinator/queue/what-it-did.tsx`,
  `what-it-did-row.tsx`
- `apps/web/app/coordinator/` Queue view composition
- `apps/web/hooks/domains/coordinator/use-activity.ts`

## Dependencies

- Task 03 (routes).

## Risks

- A long log must not slow the Queue: the section loads its first page after
  the Queue groups render.
