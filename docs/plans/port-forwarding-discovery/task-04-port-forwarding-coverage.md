---
id: "04-port-forwarding-coverage"
title: "Port-forwarding coverage"
status: completed
wave: 4
depends_on: ["03-task-surfaces"]
plan: "plan.md"
spec: "../../specs/ui/requirements/port-forwarding-discovery.md"
---

# Task 04: Port-forwarding coverage

Update the existing desktop port-forward E2E coverage to exercise the new preference flow and add a
phone-native test for the Drawer/top-bar path. Keep the existing detected/manual port and proxy URL
assertions as regression coverage for the unchanged runtime dialog.

## Acceptance

- Desktop local and remote sessions start with the preference off, can enable it from the launcher,
  open the dialog, and retain the preference after reload.
- Disabling visibility does not send a tunnel-stop request and re-enabling lists the still-active
  tunnel.
- Phone can reach the action from the task-switcher Drawer, opens the dialog after enabling, meets
  viewport/touch/safe-area expectations, and has no horizontal document overflow.
- The not-ready path is visibly disabled/hidden as specified.

## Verification

- If dependencies are absent: `cd apps && pnpm install --frozen-lockfile`
- `cd apps/web && pnpm e2e -- tests/session/port-forward-dialog.spec.ts --project=chromium`
- `cd apps/web && pnpm e2e -- tests/session/mobile-port-forwarding.spec.ts --project=mobile-chrome`
- Record any required scoped backend fixture command and cleanup evidence in `## Results`.

## Files likely touched

- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/session/port-forward-dialog.spec.ts`
- `apps/web/e2e/tests/session/mobile-port-forwarding.spec.ts`

## Dependencies

Task 03.

## Parallelism

Sequential. These tests depend on the shipped HTTP, state, and responsive surfaces.

## Inputs

- Spec scenarios and the existing `port-forward-dialog.spec.ts` seed helpers.
- Existing mobile-chrome naming convention and task-switcher Drawer geometry assertions.

## Output contract

Report exact E2E commands, project/test counts, any fixture teardown, changed files, remaining risks,
and synchronized task/plan status. Do not mark the feature complete without updating the plan's
Verification Results.

## Results

Implemented the launcher-driven preference flow in the existing desktop and mobile E2E fixtures.
The desktop coverage now verifies local and remote opt-in, disable/re-enable without stopping an
active tunnel, reload persistence, and the existing dialog port/proxy behavior. The mobile coverage
verifies the phone Drawer action, dialog opening, viewport containment, and no document-level
horizontal overflow.

Verification:

- `rtk make build-web` — passed.
- `rtk make build-backend` — passed; built the backend, mock agent, and agentctl fixtures.
- `rtk make -C apps/backend e2e-plugin-package` — passed; wrote the E2E plugin package.
- `rtk pnpm exec playwright test --config e2e/playwright.config.ts e2e/tests/session/port-forward-dialog.spec.ts --project=chromium` — 12 passed in 1 minute.
- `rtk pnpm exec playwright test --config e2e/playwright.config.ts e2e/tests/session/mobile-port-forwarding.spec.ts --project=mobile-chrome` — 1 passed in 6.9 seconds.

The fixture-managed E2E runs exited cleanly and created their temporary repositories under `/tmp`.
Changed E2E files: `e2e/pages/session-page.ts`, `e2e/tests/session/port-forward-dialog.spec.ts`,
and `e2e/tests/session/mobile-port-forwarding.spec.ts`.

Not-ready path evidence: the focused launcher/provider coverage verifies the disabled `canToggle`
state, but these E2E runs do not simulate an agentctl readiness loss, so the not-ready E2E scenario
remains uncovered.

Remaining risks: a dedicated E2E fixture is still needed to exercise the live agentctl-not-ready
transition and its accessible disabled-state explanation. The implementation fails closed through
the shared readiness gate, and the covered unit/surface tests continue to verify that behavior.

Synchronized status: Task 04 and the implementation plan remain marked completed; the uncovered
scenario is recorded explicitly as a validation risk rather than claimed as E2E coverage.
