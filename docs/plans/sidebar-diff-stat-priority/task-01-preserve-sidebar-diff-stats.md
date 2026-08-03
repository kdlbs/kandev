---
id: "01-preserve-sidebar-diff-stats"
title: "Preserve sidebar diff stats"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/sidebar-diff-stat-priority.md"
---

# Task 01: Preserve sidebar diff stats

- **Acceptance:** A selected desktop task row with non-zero diff stats displays its totals while
  idle, even if the row owns focus after selection.
- **Acceptance:** Fine-pointer hover swaps the totals for the existing Task actions trigger;
  opening the context menu retains the visible trigger, and keyboard focus on that trigger remains
  discoverable.
- **Acceptance:** The existing mobile task-switcher continues to show both the totals and a
  non-overlapping, 44px Task actions target.
- **Files likely touched:** `apps/web/components/task/task-item.tsx`,
  `apps/web/components/task/task-item.test.tsx`,
  `apps/web/e2e/tests/task/sidebar-diff-stats.spec.ts`.
- **Dependencies:** None.
- **Parallelism:** Sequential; the shared row and its desktop E2E coverage are coupled.
- **Inputs:** `docs/specs/ui/sidebar-diff-stat-priority.md`, `plan.md`,
  `apps/web/components/task/task-switcher-context-menu.tsx`, and the existing mobile assertion in
  `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`.
- **Verification:**
  ```bash
  cd apps && pnpm install --frozen-lockfile
  pnpm --filter @kandev/web test -- components/task/task-item.test.tsx
  pnpm --filter @kandev/web e2e:run -- tests/task/sidebar-diff-stats.spec.ts
  pnpm --filter @kandev/web e2e:run -- --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts
  pnpm --filter @kandev/web run typecheck
  ```
- **Output contract:** Report the changed files, exact tests and results, blockers and risks, then
  update this task and `plan.md` status in the same conversation.

## Results

- `rtk pnpm --filter @kandev/web test -- components/task/task-item.test.tsx` — final run passed,
  28/28 tests. The preceding RED run failed only the new selected-row visibility assertion
  (27/28 passing), proving the old focus rule was exercised.
- `rtk pnpm e2e:run tests/task/sidebar-diff-stats.spec.ts` — initial run reached the new assertion
  but failed because the pointer remained over the clicked row; the corrected
  `rtk pnpm e2e:run --no-build tests/task/sidebar-diff-stats.spec.ts` passed, 1/1 test, including
  the final context-menu-open assertion.
- `rtk pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts`
  — passed, 8/8 tests, including the diff/action non-overlap check.
- `rtk pnpm run typecheck` — passed.
- `rtk git diff --check` — passed.
- Fixup commit `8c0f56c0b` added the named action-focus scope, restored no-stats row-focus
  disclosure, and added the context-menu-open unit assertion; the targeted unit suite passed
  30/30, desktop E2E passed 1/1 after a production rebuild, mobile E2E passed 8/8, and web
  typecheck passed.
- `rtk pnpm e2e:run --no-build tests/task/sidebar-diff-stats-capture.spec.ts` — passed, 1 desktop
  screenshot capture test.
- `rtk pnpm e2e:run --no-build --project mobile-chrome
  tests/task/mobile-sidebar-diff-stats-capture.spec.ts` — final capture run passed, 1 mobile
  screenshot capture test.

Fresh-worktree installation was skipped because `apps/node_modules` was already present. No
temporary capture specs remain; ignored PR assets were inspected, compressed, and recorded in the
non-empty `apps/web/.pr-assets/manifest.json`.
