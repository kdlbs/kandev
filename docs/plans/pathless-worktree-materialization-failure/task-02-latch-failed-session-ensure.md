---
id: "02-latch-failed-session-ensure"
title: "Latch Failed Session Ensure Requests"
status: done
wave: 2
depends_on:
  - "01-persist-pathless-failed-environment"
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.7
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 02: Latch Failed Session Ensure Requests

## Summary

The session hook must keep a failed ensure request latched.
The user can start the next request with the existing Retry control.

## In scope

- Add a failing hook test that changes the session-loader identity after an ensure error.
- Keep the request count at one after state-driven rerenders.
- Preserve explicit retry and task-change behavior.

## Out of scope

- Changes to the error banner or preview empty-state markup.
- Changes to WebSocket request timeouts.
- Automatic retry or backoff logic.

## Acceptance

- Loader identity changes do not repeat a failed `session.ensure` request.
- The existing Retry control starts one new request.
- A task change clears the prior error and permits the new task request.

## Verification

Run this command from `apps/web`:

```bash
pnpm exec vitest run hooks/domains/session/use-ensure-task-session.test.ts
```

## Files likely touched

- `apps/web/hooks/domains/session/use-ensure-task-session.ts`
- `apps/web/hooks/domains/session/use-ensure-task-session.test.ts`

## Dependencies

- Task 01 must preserve the terminal environment state that this hook displays.

## Risks

- The hook can suppress a valid retry if task changes do not clear its latch.

## Parallelism

`sequential`

## Inputs

- `AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.2`
- `AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.7`
- `AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8`
- `docs/specs/tasks/system-design/task-launch-failure-recovery.md`
- `apps/web/components/task/ensure-session-error.tsx`

## Results

- RED: the focused test observed two ensure requests after the loader identity changed.
- GREEN: the full hook test file passed all 22 tests.
