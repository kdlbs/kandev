---
id: "02-settings-default-inheritance"
title: "Render inherited action profiles"
status: done
wave: 2
depends_on: ["01-backend-default-inheritance"]
plan: "plan.md"
spec: "../../specs/agents/requirements/utility-agent-profiles.md"
---

# Task 02: Render inherited action profiles

Update the Utility Agents page after the backend contract is in place.

- **Acceptance:** A built-in action with an `inherit` binding and no concrete profile ID renders the
  selected default in its profile picker. Empty and concrete `unconfigured` bindings remain
  unavailable for repair.
- **Acceptance:** A built-in action with a concrete stale profile ID remains unavailable and keeps
  its repair state. Explicit profile selection and shared Settings save/discard behavior remain
  unchanged.
- **Acceptance:** Desktop and phone layouts keep the selector and edit action reachable without
  introducing horizontal overflow.
- **Verification:** Bootstrap a fresh worktree if needed, then run:

  ```bash
  cd apps
  pnpm install --frozen-lockfile
  pnpm --filter @kandev/web test -- --run components/settings/utility-sections.test.tsx components/settings/utility-agents-section.test.ts
  cd web
  pnpm run typecheck
  pnpm run i18n:check
  pnpm run i18n:ratchet
  pnpm e2e:run --project chromium tests/settings/utility-agents.spec.ts
  pnpm e2e:run --project mobile-chrome tests/settings/mobile-utility-agents.spec.ts
  ```

- **Files likely touched:** `apps/web/components/settings/utility-sections.tsx`,
  `apps/web/components/settings/utility-sections.test.tsx`,
  `apps/web/e2e/tests/settings/utility-agents.spec.ts`, and
  `apps/web/e2e/tests/settings/mobile-utility-agents.spec.ts`.
- **Dependencies:** Task 01.
- **Parallelism:** sequential.
- **Inputs:** The Utility Agents settings scenarios and mobile scenario in
  `docs/specs/agents/requirements/utility-agent-profiles.md`, the existing stacked action-row mobile test, and
  the shared `UtilityAgentProfilePicker`.
- **Output contract:** Report rendered inherited and stale states, desktop/mobile checks, exact
  commands and results, and synchronized task/plan status. Do not add new copy unless existing
  localized strings are insufficient.

## Results

Implemented the settings picker selection helper so inherited empty built-in bindings use the
default sentinel, while empty and concrete unconfigured profile IDs remain unavailable for repair.
The desktop and mobile regression scenarios exercise an inherited empty built-in response with the
saved default profile; the mobile test selects the concrete profile option by ID so duplicate display
labels do not make the interaction ambiguous.

Verification:

- `cd apps && pnpm --filter @kandev/web test -- --run components/settings/utility-sections.test.tsx components/settings/utility-agents-section.test.ts` (pass: 2 files, 6 tests)
- `cd apps/web && pnpm run typecheck` (pass)
- `cd apps/web && pnpm run i18n:check` (pass; existing real-locale parity warnings are advisory)
- `cd apps/web && pnpm run i18n:ratchet` (pass)
- `cd apps/web && pnpm e2e:run --project chromium tests/settings/utility-agents.spec.ts` (pass: 9 tests)
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-utility-agents.spec.ts` (pass: 1 test)
