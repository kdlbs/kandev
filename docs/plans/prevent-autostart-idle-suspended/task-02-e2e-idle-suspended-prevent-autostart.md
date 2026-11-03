---
id: "02-e2e-idle-suspended-prevent-autostart"
title: "E2E coverage for idle-suspended open with the preference"
status: completed
wave: 2
depends_on:
  - "01-gate-idle-suspended-focus-resume"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-001
acceptance_criteria:
  - AC-EXECUTORS-IDLE-PARKING-001.4
  - AC-EXECUTORS-IDLE-PARKING-001.5
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
---

# Task 02: E2E Idle-Suspended Open With Prevent Auto-Start

## Summary

Prove end to end, on desktop and phone, that an idle-suspended task with the
preference enabled opens stopped and shows the composer hint, and that a
message resumes it exactly once.

## ASCII UI preview

`UI-01` from [plan.md](plan.md#ascii-ui-preview): with the preference on, the
composer shows `composer-agent-start-hint` above the input and no agent
starts. The phone composition is the same, above the touch composer.

## In scope

- Desktop: add a test to `apps/web/e2e/tests/session/session-idle-parking.spec.ts`
  (or a sibling spec). Reuse its setup: workspace policy with a 1-minute
  timeout, `readIdleSuspensionSnapshot`, and the WS frame listener that counts
  `session_focus` launches. Before opening the task, call
  `apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true })`.
  After opening the task, assert:
  - `composer-agent-start-hint` is visible.
  - Zero `session_focus` launch requests were sent. This is a negative
    assertion, so use a sanctioned `dwell(page, ms, "negative-assertion",
    reason)` or wait for the hint as the causal anchor.
  - `readIdleSuspensionState` is still `"suspended"`.
  Then send a message and assert that the mock reply arrives once and the
  suspension state clears.
- Phone: add the same open-stopped assertion to
  `apps/web/e2e/tests/session/mobile-session-idle-parking.spec.ts` before its
  existing message-wake step, with the preference enabled.
- Restore the preference and workspace policy in `finally`.

## Out of scope

- Changing the existing preference-off focus-resume test, which must stay
  green unchanged.

## Acceptance

- Desktop and phone specs fail on `main`, because the session resumes on
  open, and pass with Task 01.
- The existing idle-parking specs remain green.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/session/session-idle-parking.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-idle-parking.spec.ts
cd apps/web && pnpm run e2e:sleep-ratchet
```

## Likely files

- `apps/web/e2e/tests/session/session-idle-parking.spec.ts`
- `apps/web/e2e/tests/session/mobile-session-idle-parking.spec.ts`
- `apps/web/e2e/helpers/idle-parking.ts` (only if a small shared helper is
  needed)

## Results

Completed 2026-11-03.

- Desktop: new case in `e2e/tests/session/session-idle-parking.spec.ts` enables
  the preference before opening the suspended task, asserts the composer hint,
  zero `session_focus` launches after a sanctioned dwell, the suspended DB
  state, and no extra agent turn. Sending one message clears the suspension and
  adds exactly one agent reply.
- Phone: `e2e/tests/session/mobile-session-idle-parking.spec.ts` now opens the
  suspended task with the preference on, asserts the hint, zero
  `session_focus` launches, and the suspended state, then keeps its existing
  message-wake flow.
- `e2e/helpers/idle-parking.ts` owns `watchSessionFocusLaunches` so both
  negative assertions correlate request IDs with responses; the existing
  preference-off desktop case uses the same helper instead of a local listener.
- Verification: desktop spec 2 passed (4.7m), mobile spec 1 passed (1.7m);
  the existing preference-off focus-resume case remains green.
