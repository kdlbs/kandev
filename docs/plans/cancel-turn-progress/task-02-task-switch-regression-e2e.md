---
id: "02-task-switch-regression-e2e"
title: "Task-switch cancellation regression"
status: pending
wave: 2
depends_on: ["01-session-scoped-cancel-state"]
plan: "plan.md"
spec: "../../specs/ui/cancel-turn-progress.md"
---

# Task 02: Task-switch cancellation regression

## Acceptance

- A WebSocket test controller can hold exactly one `agent.cancel` request, forward every unrelated
  frame, expose the held-request count, and release the request before teardown.
- The browser regression starts cancellation on task A, switches to task B, returns to task A, and
  observes the same disabled animated cancel state until release.
- Releasing the held request completes normal cancellation and leaves no held WebSocket request or
  stale progress entry at test end.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run --project chromium tests/chat/cancel-progress-task-switch.spec.ts
```

Follow TDD: add and run the browser regression against the pre-fix control to demonstrate that the
spinner disappears after the return navigation, then run it against Task 01's implementation.

## Files likely touched

- `apps/web/e2e/helpers/ws-drop.ts`
- `apps/web/e2e/tests/chat/cancel-progress-task-switch.spec.ts`

## Dependencies

Task 01.

## Parallelism

Sequential. This task validates Task 01's state ownership through the complete task-navigation flow.

## Inputs

- Spec: the pending-cancellation navigation scenario and persistence boundary.
- Plan: `E2E Tests` and `Mobile design contract`.
- Existing patterns: `routeMainWebSocketWithMessageAddResponseDrop` in `ws-drop.ts`,
  `SessionPage.clickTaskInSidebar`, and the task-switch flow in
  `apps/web/e2e/tests/session/multi-session-ux.spec.ts`.

## Output contract

Report the initial failing assertion, final Playwright result and discovered test count, held-frame
cleanup evidence, files changed, blockers or risks, and update this task plus `plan.md`
status/results in the same primary conversation.

## Results

Pending.
