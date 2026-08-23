---
id: "11-responsive-launch-error-surface"
title: "Responsive launch-error surface"
status: done
wave: 6
depends_on: ["06-recovery-actions-ws", "08-frontend-failure-surface-and-recovery"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-launch-failure-recovery.md"
---

# Task 11: Responsive launch-error surface

Render one persistent launch-error card on desktop and phone.
Connect it to the task-scoped recovery action.

- **Acceptance:**
  1. `TaskChat` shows `TaskLaunchErrorEntry` before its empty state.
  2. `RunErrorEntry` reuses the card without changing existing session recovery behavior.
  3. One source error renders one card after summary and session data converge.
  4. Actions send exact payloads with task, session, row, action, branch, and stamp fields.
  5. Desktop uses the branch popover pattern. Phone uses `MobilePickerSheet`.
  6. Actions wrap without page overflow and provide at least 44px touch targets.
  7. The picker owns internal scrolling, safe-area clearance, focus return, and dismissal.
  8. The pointer toast contains no raw Git output. All new copy uses `t()`.

- **Verification:**
  `cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- components/task/simple/components/task-launch-error-entry.test.tsx components/task/simple/components/run-error-entry.test.tsx components/task/simple/task-chat.test.tsx && cd web && pnpm run typecheck`

- **Files likely touched:**
  `apps/web/components/task/simple/chat-activity-tabs.tsx`,
  `apps/web/components/task/simple/task-chat.tsx`,
  `apps/web/components/task/simple/components/task-launch-error-entry.tsx`,
  `apps/web/components/task/simple/components/run-error-entry.tsx`,
  `apps/web/components/task/base-branch-picker.tsx`,
  a shared responsive branch-picker helper beside those components,
  the task-launch toast call site and focused tests.

- **Dependencies:** Tasks 06 and 08.
- **Parallelism:** sequential.
- **Inputs:** spec "Desktop and mobile behavior" and mobile exemplar `MobilePickerSheet`.

## Results
Implemented the shared recovery card, desktop branch popover, and mobile branch sheet.
Recovery controls wrap on narrow screens, use 44px touch targets, and close the picker before the request completes.

Verification:

- `cd apps/web && pnpm exec vitest run components/task/simple/components/task-launch-error-entry.test.tsx components/task/simple/components/run-error-entry.test.tsx components/task/simple/task-chat.test.tsx`: 2 files and 24 tests passed.
- Desktop and mobile recovery E2E tests passed.
- Desktop and mobile screenshots show the recovery card without page overflow or a stale recovery toast.
