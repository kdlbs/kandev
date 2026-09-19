---
id: "04-e2e"
title: "E2E: disable profile hides it from task creation"
status: done
wave: 4
depends_on: ["03-settings-toggle-ui"]
plan: "plan.md"
spec: "../../specs/agents/requirements/profile-disable.md"
---

# Task 04: E2E disable-profile flow

- **Acceptance:** Playwright spec covers: open a profile's settings page, toggle Enabled off and save, reload and confirm the toggle stays off; open the task-create dialog and confirm the profile is absent from the agent profile selector; open `/settings/agents` and confirm the profile still appears under "Manage existing profiles by agent" with the toggle off; toggle it back on and confirm it returns to the selector.
- **Verification:** `cd apps && pnpm --filter @kandev/web e2e tests/settings/agent-profile-disable.spec.ts` — 2 passed.
- **Results (2026-08-02):** Spec implemented; both tests pass. Two issues found and fixed during the run:
  1. The list-row `Switch` was nested inside the row's `Link`; `stopPropagation` alone does not cancel the anchor's native default navigation, so clicking the toggle navigated to the profile page. Restructured `ProfileListItem` so the switch is a sibling of the link (also fixes interactive-inside-interactive).
  2. With the seeded profile the only profile, disabling it makes `useExecutorProfileCompat` yield an empty compat list → the dialog renders the `agent-profile-empty-state` ("No compatible agent profiles") instead of the selector. The spec asserts the empty state + zero selectors for that case, and selector-absence + other-options for the multi-profile case.
  Adjacent specs run during verification: `create-task.spec.ts`, `new-session-deleted-profile.spec.ts`, `workflow-agent-profile.spec.ts`, `agent-profile-acp.spec.ts` — all pass except two pre-existing `create-task.spec.ts` plan-mode failures (`plan-panel` visibility; reproduce at HEAD with changes stashed).
- **Files likely touched:**
  - `apps/web/e2e/tests/settings/agent-profile-disable.spec.ts`
  - optionally a page object in `apps/web/e2e/pages/` if the existing settings page objects don't cover the profile page and task-create dialog (check `agent-profile-acp.spec.ts` first and reuse its helpers).
- **Dependencies:** Tasks 01–03.
- **Parallelism:** sequential.
- **Inputs:** spec scenarios for header toggle, list toggle, and selector hiding; plan "E2E Tests" section; the `agent-profile-acp.spec.ts` pattern.
- **Output contract:** Report the spec run result, changed files, risks, and task/plan status update.
