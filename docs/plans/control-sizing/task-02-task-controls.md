---
id: "02-task-controls"
title: "Task control sweep"
status: complete
wave: 2
depends_on: ["01-shared-sizes"]
plan: "plan.md"
requirements:
  - REQ-UI-CONTROL-SIZING-001
acceptance_criteria:
  - AC-UI-CONTROL-SIZING-001.1
  - AC-UI-CONTROL-SIZING-001.2
  - AC-UI-CONTROL-SIZING-001.3
  - AC-UI-CONTROL-SIZING-001.4
  - AC-UI-CONTROL-SIZING-001.5
  - AC-UI-CONTROL-SIZING-001.6
  - AC-UI-CONTROL-SIZING-001.7
  - AC-UI-CONTROL-SIZING-001.8
  - AC-UI-CONTROL-SIZING-001.9
  - AC-UI-CONTROL-SIZING-001.10
system_design:
  - ../../specs/ui/system-design/control-sizing.md
---

# Task 02: Task control sweep

## Summary

Normalize ordinary controls throughout task creation and task details. Repair completed-session and recovery actions while preserving compact chat chrome.

## In scope

- Disposition all Task surfaces candidates in sweep-inventory.md, including newly discovered callers.
- Repair session-stopped-banner, dynamic-route-recovery, session dialogs, action-message controls, file/preview toolbars, and task-level selectors.
- Keep Start Task at the reference desktop size and preserve its split action and phone composition.
- Inspect workflow disclosure row spacing separately from its already compact Move here action.
- Add task/control-sizing.spec.ts and task/mobile-control-sizing.spec.ts using the shared geometry helper.

## Out of scope

- Feature state, callbacks, permission changes, and persistence.
- Unrelated theme, typography, or navigation redesign.

## Acceptance

- New Agent, Resume, and Start fresh session render at 28px on desktop and retain touch minimums.
- Task controls have a recorded disposition, including correct compact and mobile-only exceptions.
- Completed/recovery actions, keyboard activation, busy states, and long labels retain their behavior.

## Regression first

Seed a completed session through existing isolated fixtures. Confirm that New Agent measures 44px and fails the 28px desktop assertion before the patch.

Use TDD for new helper logic and browser regressions.
Run the new regression before production edits and record the expected failure.
Rerun the same check after the correction.

## Verification

Run from the repository root. The first package command requires installed workspace dependencies.
Run desktop and mobile checks sequentially.
The mobile command reuses the immediately preceding unchanged production build.

```bash
pnpm --dir apps/web exec vitest run components/task/chat/session-stopped-banner.test.tsx components/task-create-dialog-footer.test.ts
pnpm --dir apps/web e2e:run --project chromium tests/task/control-sizing.spec.ts tests/layout/task-topbar-workflow-stepper.spec.ts
pnpm --dir apps/web e2e:run --no-build --project mobile-chrome tests/task/mobile-control-sizing.spec.ts
pnpm --dir apps/web run typecheck
```

Before completion, run ESLint on the exact changed TypeScript files from `apps/web`.
Record the expanded file list and command in Results.
Do not substitute class-string checks for browser geometry.

## Files likely touched

- `apps/web/components/task/chat/session-stopped-banner.tsx`
- `apps/web/components/task/chat/dynamic-route-recovery.tsx`
- `apps/web/components/task-create-dialog-footer.tsx`
- `apps/web/components/task/workflow-step-disclosure.tsx`
- `apps/web/components/task/chat/session-stopped-banner.test.tsx`
- `apps/web/components/task-create-dialog-footer.test.ts`
- `apps/web/e2e/tests/task/control-sizing.spec.ts`
- `apps/web/e2e/tests/task/mobile-control-sizing.spec.ts`

The [inventory](sweep-inventory.md) section **Task surfaces** defines the remaining file scope.

## Dependencies

01-shared-sizes

## Risks

Chat menus and selection rows can use button tags without being ordinary actions. Session completion fixtures must expose the actual banner.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/control-sizing.md)
- [System design](../../specs/ui/system-design/control-sizing.md)
- [Sweep inventory](sweep-inventory.md)
- Existing Start Task, settings typography, and mobile geometry tests.

## Results

Completed 2026-09-10.

- Red: the completed-session regression measured the desktop New Agent action at
  44px instead of 28px before the migration.
- Green: focused task unit coverage passed; task control sizing passed with 2
  desktop and 1 mobile test, and the workflow-stepper suite passed with 2 desktop
  tests covering fine-pointer and contained tablet touch navigation.
- Session recovery, task creation, action-message, preview, selector, and filter
  controls now use the standard role or a recorded compact/content exception.
- The final typecheck and exact changed-file ESLint passed with 0 errors; the
  completed inventory records the task-surface dispositions.
