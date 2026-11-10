---
id: "05-unread-dot-e2e"
title: "Unread dot end-to-end coverage"
status: done
wave: 3
depends_on: ["02-live-unread-projection", "03-unread-list-repair", "04-task-row-unread-dot"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-UNREAD-DOT-001
acceptance_criteria:
  - AC-TASKS-TASK-UNREAD-DOT-001.1
  - AC-TASKS-TASK-UNREAD-DOT-001.2
  - AC-TASKS-TASK-UNREAD-DOT-001.3
  - AC-TASKS-TASK-UNREAD-DOT-001.5
  - AC-TASKS-TASK-UNREAD-DOT-001.6
  - AC-TASKS-TASK-UNREAD-DOT-001.7
  - AC-TASKS-TASK-UNREAD-DOT-001.8
  - AC-TASKS-TASK-UNREAD-DOT-001.13
  - AC-TASKS-TASK-UNREAD-DOT-001.14
  - AC-TASKS-TASK-UNREAD-DOT-001.15
  - AC-TASKS-TASK-UNREAD-DOT-001.16
system_design:
  - ../../specs/tasks/system-design/task-unread-dot.md
---

# Task 05: Unread dot end-to-end coverage

## Summary

Playwright specs prove the dot lifecycle through the real backend: appear on
background messages, clear on read, hide for the open task and when the setting
is off, keep the row height, and survive reload. A mobile spec covers the task
switcher.

## In scope

- Desktop spec in the `chromium` project and mobile spec in `mobile-chrome`,
  with `@covers` tags for the listed criteria.
- Each spec enables the independent dot through `apiClient.saveUserSettings({ unread_divider: false, unread_task_dot: true })`. The desktop scenario uses a 3-second hide delay and asserts the opened dot lingers, then clears; the phone scenario uses 0 to exercise immediate hiding.
- Seeding through the e2e read-cursor route, which task 02 makes publish the
  cursor event with `reset: true`, plus mock-agent messages.
- A bounding-box assertion in both specs that the row height is equal with and without the dot.

## Out of scope

- New product code.

## Acceptance

- Each listed AC has a tagged assertion that fails when the dot logic breaks.
- Both specs pass in the single-worker local runner.

## Verification

```bash
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/task/task-unread-dot.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/task/mobile-task-unread-dot.spec.ts)
```

## Files likely touched

- `apps/web/e2e/tests/task/task-unread-dot.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-unread-dot.spec.ts`
- `apps/web/e2e/helpers/unread-divider.ts` (shared seeding)

## Dependencies

`02-live-unread-projection`, `03-unread-list-repair`, `04-task-row-unread-dot`.

## Risks

- Timing of summary delivery; assert with Playwright auto-retry, never sleeps.
- The `chromium` project ignores `mobile-*.spec.ts`; the mobile command must
  name `mobile-chrome`.

## Parallelism

`sequential`

## Inputs

- `apps/web/e2e/tests/chat/unread-divider.spec.ts`, `e2e/README.md`.

## Results

Done. `e2e/tests/task/task-unread-dot.spec.ts` (`chromium`) and
`e2e/tests/task/mobile-task-unread-dot.spec.ts` (`mobile-chrome`) seed
agent-less tasks through `seedUnreadDotTasks` in `helpers/unread-divider.ts`.
Both passed in the single-worker host runner (desktop 23.0s, mobile 10.5s).
