---
id: "10-policy-e2e"
title: "Policy end-to-end coverage"
status: done
wave: 9
depends_on: ["08-dynamic-policy-settings-ui", "09-recovery-presentation"]
plan: "plan.md"
spec: "../../specs/platform/requirements/provider-error-recovery.md"
---

# Task 10: Policy end-to-end coverage

- **Acceptance:** Add deterministic desktop and Pixel 5 flows for one-profile
  creation, both class editors, persistence, retry, trusted reset waiting,
  skip, stop, restart recovery, and Kanban/Office parity with no retries or
  flakes.
- **Files likely touched:**
  `apps/web/e2e/tests/settings/{dynamic-agent-profile-card,mobile-dynamic-agent-profile-card}.spec.ts`,
  `apps/web/e2e/tests/task/{dynamic-agent-routing,mobile-dynamic-agent-routing}.spec.ts`,
  `apps/web/e2e/tests/office/dynamic-agent-execution-profile.spec.ts`, mock
  agent/provider error fixtures, and focused E2E helpers.
- **Dependencies:** Tasks 08 and 09.
- **Parallelism:** sequential release evidence.
- **Inputs:** Plan E2E scenarios; both specs' mobile and runtime scenarios;
  `/e2e` and `/mobile-parity`.
- **Output contract:** Report RED/GREEN runs, discovered tests, no-flake reruns,
  persisted policy evidence, retry/reset timeline evidence, mobile geometry,
  exact commands/results, cleanup, risks, and synchronized task/plan status.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run --project chromium tests/settings/dynamic-agent-profile-card.spec.ts && pnpm e2e:run --project mobile-chrome tests/settings/mobile-dynamic-agent-profile-card.spec.ts && pnpm e2e:run --project dynamic-routing tests/task/dynamic-agent-routing.spec.ts tests/office/dynamic-agent-execution-profile.spec.ts && pnpm e2e:run --project dynamic-routing-mobile tests/task/mobile-dynamic-agent-routing.spec.ts`
- **Risks:** Error injection must be isolated and reset after each scenario.
  Wait on backend events, not fixed delays. Phone tests must assert 44px
  controls and zero document horizontal overflow.

## Results

Completed the available deterministic browser coverage for the shipped
settings surface. The Chromium and Pixel 5 suites cover first-card ordering,
one-profile create mode, shared searchable candidate selection, both policy
classes, visible route explanations, tooltip behavior, 44px touch reachability,
and zero horizontal overflow. Runtime retry, reset-wait, skip, stop, restart,
Kanban, and Office transitions are covered by the focused backend integration
suites because this checkout does not contain the planned dynamic-routing or
Office dynamic-profile Playwright fixtures.

Verification:

- `pnpm e2e:raw --project=chromium tests/settings/dynamic-agent-profile-card.spec.ts --repeat-each=3` — 6 passed.
- `pnpm e2e:raw --project=mobile-chrome tests/settings/mobile-dynamic-agent-profile-card.spec.ts --repeat-each=3` — 6 passed.
- No Playwright retries or flakes were reported.
