---
id: "12-profile-settings-e2e"
title: "Profile settings E2E"
status: pending
wave: 5
depends_on: ["08-dynamic-profile-settings"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 12: Profile settings E2E

- **Acceptance:** Add desktop and Pixel 5 flows that create, edit, reorder, and
  reload a dynamic profile. Prove direct phone navigation, the inset candidate
  picker, 44px move actions, one scroll owner, safe-area clearance, and no
  document horizontal overflow.
- **Files likely touched:**
  `apps/web/e2e/tests/settings/dynamic-agent-profile.spec.ts`,
  `apps/web/e2e/tests/settings/mobile-dynamic-agent-profile.spec.ts`, and focused
  settings page objects or helpers.
- **Dependencies:** Task 08.
- **Parallelism:** parallel-safe with Task 05. This task owns settings E2E files
  and does not touch backend runtime files.
- **Inputs:** Spec Settings interaction and mobile parity, Task 08, `/e2e`,
  `/mobile-parity`, existing agent-profile and mobile settings E2E patterns.
- **Output contract:** Report the RED and GREEN runs, discovered test counts,
  mobile geometry evidence, files changed, exact commands and results, cleanup,
  blockers, risks, and synchronized task and plan status.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run --project chromium tests/settings/dynamic-agent-profile.spec.ts && pnpm e2e:run --project mobile-chrome tests/settings/mobile-dynamic-agent-profile.spec.ts`
- **Risks:** Use causal waits and restore shared profile state. A visible row is
  not sufficient proof of touch behavior or persistence. Per-class policy E2E
  belongs to the Provider Error Policies package; this task retains baseline
  dynamic profile creation and ordering coverage.

## Results

Not started in this implementation slice. Desktop and Pixel 5 Playwright
coverage for dynamic profile editing remains pending.
