---
id: "02-pinned-prompt-overflow"
title: "Measure pinned prompt overflow"
status: done
wave: 2
depends_on: ["01-transcript-threshold-and-controls"]
plan: "plan.md"
spec: "../../specs/ui/requirements/last-prompt-pinning-regressions.md"
---

# Task 02: Measure pinned prompt overflow

- **Acceptance:** The collapsed bar exposes two rendered lines; expand appears only when the prompt's measured content is clipped. The expanded prompt stays internally scrollable and max-height bounded. Hidden bar controls remain inert.
- **Verification:** First make the fitting/clipped prompt test fail, then run `cd apps/web && pnpm exec vitest run components/task/chat/anchored-last-prompt-bar.test.tsx`.
- **Files likely touched:** `apps/web/components/task/chat/anchored-last-prompt-bar.tsx`, `anchored-last-prompt-bar.test.tsx`.
- **Dependencies:** Task 01.
- **Parallelism:** sequential — it shares the transcript bar surface with task 01.
- **Output contract:** Summary, exact files changed, targeted test result, remaining risks, and plan/task status update.
