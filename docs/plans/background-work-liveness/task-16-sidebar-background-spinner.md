---
id: "16-sidebar-background-spinner"
title: "Align the sidebar background spinner"
status: done
wave: 16
depends_on:
  ["14-attest-background-lifecycles", "15-publish-active-subagent-counts"]
plan: "plan.md"
spec: "../../specs/platform/requirements/background-work-liveness.md"
---

# Task 16: Align the sidebar background spinner

- **Acceptance:** The shared sidebar/task-switcher row renders foreground
  generation with the existing yellow dashed spinner and background work with
  the same dashed spinner in violet. The background element retains a distinct
  test ID and exposes “Background work is running” through tooltip/accessibility
  text. Other task-state icon surfaces remain unchanged.
- **Verification:** `cd apps && pnpm --filter @kandev/web test -- components/task/task-item.test.tsx`; focused rendered inspection at desktop and mobile width.
- **Files likely touched:** `apps/web/components/task/task-item.tsx` and
  `apps/web/components/task/task-item.test.tsx`.
- **Dependencies:** Tasks 14-15 establish the trustworthy state feeding the
  shared row.
- **Inputs:** Spec sidebar indicator behavior; existing
  `IconCircleDashed` generating branch; nearest mobile exemplar in the shared
  task switcher/sidebar row.
- **Worker:** Primary planner, direct localized UI implementation; standard
  model tier.
- **Mobile parity:** Shared content/style only; no layout, navigation, touch, or
  responsive contract changes. Verify the same component at mobile width.
- **Output contract:** Report exact DOM/accessibility contract, RED/GREEN unit
  evidence, rendered desktop/mobile check, exact commands/results, and update
  only this task status.
