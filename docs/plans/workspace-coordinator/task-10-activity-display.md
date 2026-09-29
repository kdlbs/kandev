---
id: "10-activity-display"
title: "Copilot activity display"
status: pending
wave: 6
depends_on:
  - "11-panel-swap"
  - "12-review-follow-ups"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COPILOT-006
acceptance_criteria:
  - AC-COORDINATOR-COPILOT-006.1
  - AC-COORDINATOR-COPILOT-006.2
  - AC-COORDINATOR-COPILOT-006.3
  - AC-COORDINATOR-COPILOT-006.4
  - AC-COORDINATOR-COPILOT-006.5
  - AC-COORDINATOR-COPILOT-006.6
  - AC-COORDINATOR-COPILOT-006.7
  - AC-COORDINATOR-COPILOT-006.8
  - AC-COORDINATOR-COPILOT-006.9
system_design:
  - ../../specs/coordinator/system-design/copilot.md
  - ../../specs/coordinator/system-design/copilot-panel.md
---

# Task 10: Copilot Activity Display (WP-4d)

## Summary

The copilot shows the coordinator's work the way assistant chats for
non-developers do: one live status line while a turn runs, one collapsed chip
of tool calls afterwards, the proposal card always in full, and no session
start-up rows. This changes mockup `p1-05`, which shows each tool call as a
row.

## In scope

- `hideStartupRows` on `QuickChatSessionView`, backed by
  `hideSuccessfulStartupRows` in `components/quick-chat/startup-rows.ts`:
  already built; this task only verifies it and pins it with tests.
- The status line, via the `activityDisplay` prop, above the composer: a fixed tool-name-to-verb table with a
  generic fallback, and the elapsed seconds.
- The collapsed tool chip per finished turn, with `propose_task_kandev` calls
  kept out of it.
- Copy in every shipped locale.
- Everything opt-in on the view, set only by the coordinator panel
  ([copilot design](../../specs/coordinator/system-design/copilot-panel.md#activity-display)).

## Out of scope

- Settings configuration chat, Quick Chat and the task page.

## ASCII UI preview

```text
+---------------------------------+
| * Coordinator: Planner      [x] |
|---------------------------------|
| You: what needs me today?       |
| [v Checked 3 sources . 9s]      |  collapsed chip
| Two cards need you: ...         |
| +-----------------------------+ |
| | Pending approval  ...       | |  proposal card, never collapsed
| +-----------------------------+ |
|---------------------------------|
| Reading tasks... 4s             |  status line while running
| Ask the coordinator...   [Stop] |
+---------------------------------+
```

## Acceptance

- While a turn runs, one status line updates in place and no tool rows render.
- After the turn, one collapsed chip expands to the tool rows; a proposal card
  stays visible.
- Start-up rows are hidden after a successful start and kept while starting or
  after a failed start.
- Settings configuration chat, Quick Chat and the task page render as before.
- A tool call awaiting permission stays visible with Approve and Deny while
  the turn runs, including while the session waits for that decision; a
  stopped or failed turn still collapses into a chip.
- A proposal card is visible while its turn is still running; an unreturned
  or failed proposal call stays hidden until the turn ends
  (`AC-COORDINATOR-COPILOT-006.6`).
- After Approve or Deny of a permission the status line keeps its verb and
  count, and the turn's other calls stay hidden until it ends; a stale
  pending request of an older turn does not keep the line running (unit tests
  on the derived running-turn id).
- A message with a malformed `created_at` leaves the chip label without a
  duration instead of showing a wrong one.
- The chip label states count, duration (`Ns`, `Nm Ss`, `Nh Mm`) and a
  "N failed" text; unit tests pin the formats, the one-call and no-call cases
  and the proposal exclusion (`AC-COORDINATOR-COPILOT-006.7`).
- The status line shows "Working" when no tool runs or the tool is unknown,
  continues the count after a reload, and is absent while `STARTING`; unit
  tests on the verb table and the running-turn rules pin it
  (`AC-COORDINATOR-COPILOT-006.9`).

## Verification

```bash
cd apps/web && pnpm test -- components/quick-chat app/coordinator/copilot
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator
```

## Likely files

- `apps/web/components/quick-chat/startup-rows.ts` and test
- `apps/web/components/quick-chat/` (session view and content props)
- `apps/web/app/coordinator/copilot/` (status line, chip)
- `apps/web/src/locales/*/coordinator.json`

## Dependencies

- Task 11 has passed Review: the activity display is built on the right-side
  panel, not the popover. Task 11 itself follows tasks 08 and 09, so the
  branch stacks on task 11's branch.

## Risks

- The collapse must not hide a permission request or the proposal card; tests
  pin both.
