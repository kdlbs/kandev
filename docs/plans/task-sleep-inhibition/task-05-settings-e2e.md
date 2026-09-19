---
id: "05-settings-e2e"
title: "Sleep setting E2E coverage"
status: done
wave: 5
depends_on: ["03-system-api-wiring", "04-task-actions-card"]
plan: "plan.md"
spec: "../../specs/platform/requirements/task-sleep-inhibition.md"
---

# Task 05: Sleep setting E2E coverage

## Acceptance

- Desktop E2E proves default/configured value, manual-save timing, persistence after reload, visible deployment caveat, and cleanup restoration through the real UI and API.
- Mobile E2E proves the same saved outcome plus control/card containment, floating-save clearance, a 44px control row, and no document horizontal overflow.
- Focused managed-runner commands discover and pass the intended desktop and `mobile-chrome` tests against fresh production builds.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
```

```bash
cd apps/web && pnpm e2e:run tests/settings/settings-manual-save.spec.ts -- --grep "persists host sleep inhibition only when Save changes is pressed"
```

```bash
cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-general-settings.spec.ts -- --grep "host sleep inhibition"
```

## Files likely touched

- `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`
- `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`

## Dependencies

Tasks 03 and 04.

## Parallelism

Sequential after backend and frontend behavior exist.

## Inputs

- Spec scenarios: default off, save/reload, unavailable container host, phone viewport.
- Plan section: E2E Tests.
- E2E guidance: real UI assertions, baseline restore in `afterEach`/`finally`, production-build managed runner, and `mobile-*.spec.ts` routing. This existing mobile file already matches the `mobile-chrome` project.

## Risks

- E2E must not attempt to suspend or inhibit the CI host; native acquisition is covered through backend seams.
- Always restore the install-wide setting because the worker-scoped backend persists it across tests.

## Output contract

Report discovered test counts, exact commands/outcomes, screenshots or other artifacts if produced, cleanup evidence, files changed, blockers/risks, and synchronized task/plan status.

## Results

- Desktop managed runner passed:
  `cd apps/web && pnpm e2e:run tests/settings/settings-manual-save.spec.ts -- --grep
  "persists host sleep inhibition only when Save changes is pressed"` (1 passed).
- Mobile managed runner passed:
  `cd apps/web && pnpm e2e:run --project mobile-chrome
  tests/settings/mobile-general-settings.spec.ts -- --grep "host sleep inhibition"`
  (1 passed).
- The tests cover default/configured state, save timing, reload persistence, the host
  caveat, 44px control-row geometry, save clearance, horizontal containment, and
  `finally` restoration of the install-wide setting. Capture-enabled reruns also passed
  and produced the two ignored PR assets recorded in the plan verification results.
