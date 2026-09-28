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
system_design:
  - ../../specs/coordinator/system-design/copilot.md
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
  `hideSuccessfulStartupRows` in `components/quick-chat/startup-rows.ts`.
- The status line above the composer: a fixed tool-name-to-verb table with a
  generic fallback, and the elapsed seconds.
- The collapsed tool chip per finished turn, with `propose_task_kandev` calls
  kept out of it.
- Copy in every shipped locale.
- Everything opt-in on the view, set only by the coordinator panel
  ([copilot design](../../specs/coordinator/system-design/copilot.md#activity-display)).

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
- Settings configuration chat and Quick Chat render as before.

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
