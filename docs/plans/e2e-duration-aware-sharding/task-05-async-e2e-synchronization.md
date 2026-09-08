---
id: "05-async-e2e-synchronization"
title: "Repair asynchronous E2E synchronization"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/e2e-duration-aware-sharding.md"
---

# Task 05: Repair asynchronous E2E synchronization

## Acceptance

- Config-management coverage waits for the actual MCP update completion state
  and preserves useful error evidence when the state does not converge; it
  does not rely on a marker that can be missed by a 30-second race.
- Mobile autopilot coverage waits for the API/WS/UI hydration boundary before
  checking the sidebar state icon and distinguishes a missing event from a
  missing render.
- Both regressions pass repeatedly with existing timeouts and expose a
  bounded, state-aware wait instead of a larger arbitrary timeout.

## Verification

```sh
cd apps/web
pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium e2e/tests/settings/config-management.spec.ts
pnpm exec playwright test --config e2e/playwright.config.ts --project=mobile-chrome e2e/tests/task/mobile-autopilot-mode.spec.ts
```

Repeat each targeted command enough to exercise the original retry path and
retain traces or logs when convergence fails.

## Files likely touched

- `apps/web/e2e/tests/settings/config-management.spec.ts`
- `apps/web/e2e/tests/task/mobile-autopilot-mode.spec.ts`
- the smallest existing E2E API, WS, or page helper needed to expose a stable
  readiness assertion

## Dependencies

None. Preserve the current worker-scoped backend and WebSocket fixture
contracts.

## Parallelism

Parallel candidate with Tasks 01, 04, and 06. It must not modify the shard
planner or global retry policy.

## Inputs

- `runAndWait` and its exact-marker behavior in the config-management spec.
- The mobile autopilot API poll and sidebar assertion ordering.
- Existing Playwright trace, network, and WebSocket diagnostics.

## Output contract

Report the observed event/order race, the readiness condition chosen, targeted
repeat results, and evidence that the fix does not merely increase timeout or
retry values.

## Implementation result

Config-chat now waits for the backend session to complete before reading the
transcript, and the model round-trip uses an advertised mock model so later
tests cannot inherit an unsupported profile value. The full config-management
suite passed 21/21. Mobile autopilot waits for both hydrated state icons and
passed its focused test within the existing timeout.
