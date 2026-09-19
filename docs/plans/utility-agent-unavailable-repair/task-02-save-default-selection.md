---
id: "02-save-default-selection"
title: "Save inherited utility selection"
status: done
wave: 2
depends_on: ["01-normalize-empty-bindings"]
plan: "plan.md"
spec: "../../specs/agents/requirements/utility-agent-profiles.md"
---

# Task 02: Save inherited utility selection

Fix the shared Utility Agents draft transition and prove the real persistence path on desktop and
phone viewports.

- **Acceptance:** The picker's empty fallback callback creates `agent_profile_id: ""`,
  `profile_binding_state: "inherit"`, and `enabled: true`. A concrete profile remains explicit.
- **Acceptance:** Selecting Default from an unavailable built-in action saves through the real
  backend and remains selected after reload.
- **Acceptance:** The existing stacked mobile action row remains reachable and the touch flow sends
  the same inherited binding without horizontal overflow.
- **Verification:** Bootstrap dependencies if needed, add the failing unit and E2E regressions first,
  then run:

  ```bash
  cd apps
  pnpm install --frozen-lockfile
  pnpm --filter @kandev/web test -- --run components/settings/utility-agents-section.test.ts
  cd web
  pnpm run typecheck
  pnpm run i18n:check
  pnpm run i18n:ratchet
  pnpm e2e:run --project chromium tests/settings/utility-agents.spec.ts -- --grep "repairs an unavailable action with the default"
  pnpm e2e:run --project mobile-chrome tests/settings/mobile-utility-agents.spec.ts
  ```

- **Files likely touched:** `apps/web/components/settings/utility-agents-section.tsx`,
  `apps/web/components/settings/utility-agents-section.test.ts`,
  `apps/web/e2e/tests/settings/utility-agents.spec.ts`, and
  `apps/web/e2e/tests/settings/mobile-utility-agents.spec.ts`.
- **Dependencies:** Task 01.
- **Parallelism:** sequential.
- **Inputs:** The Utility Agents save and persistence scenarios in
  `docs/specs/agents/requirements/utility-agent-profiles.md`, the current shared picker empty-value contract, the
  existing desktop Utility Agents spec, and the existing stacked mobile action-row spec.
- **Output contract:** Report files changed, RED and GREEN test commands and results, saved PATCH
  payload and reload evidence, desktop/mobile outcomes, cleanup, and synchronized task/plan status.

## Results

Implemented a shared draft transition that treats both the Default sentinel and the picker's empty
callback as inherited state. Concrete profile IDs remain explicit. The desktop regression serves a
synthetic unavailable GET response, lets the PATCH reach the isolated Go backend, reloads from that
backend, and restores the original fixture row. The mobile regression selects Default by touch,
captures the inherited PATCH body, and verifies that the actions card does not overflow.

- RED: `cd apps/web && pnpm exec vitest run
  components/settings/utility-agents-section.test.ts` failed as expected because the empty callback
  produced `explicit` instead of `inherit`.
- GREEN: the same command passed 5 tests after the draft transition repair.
- Component regression: `cd apps/web && pnpm exec vitest run
  components/settings/utility-agents-section.test.ts
  components/settings/utility-sections.test.ts` passed 8 tests.
- Desktop E2E: `cd apps/web && pnpm exec playwright test --config=e2e/playwright.config.ts
  e2e/tests/settings/utility-agents.spec.ts --project=chromium --grep "repairs an unavailable
  action with the default"` passed 1 test.
- Mobile E2E: `cd apps/web && pnpm exec playwright test --config=e2e/playwright.config.ts
  e2e/tests/settings/mobile-utility-agents.spec.ts --project=mobile-chrome` passed 2 tests.
- Saved mobile PATCH: `agent_profile_id: ""`, `profile_binding_state: "inherit"`, and
  `enabled: true`.
- Fixup review: both desktop and mobile option selection now scope through the page-level listbox
  role, while the picker test ID remains available for the trigger and dropdown semantics.
- Generated artifacts: None.
- Cleanup: The desktop E2E restores the original built-in row in a `finally` block.
- Security or external side effects: None. Both browser projects use an isolated E2E backend.
