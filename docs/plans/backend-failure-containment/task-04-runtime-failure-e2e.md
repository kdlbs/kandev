---
id: "04-runtime-failure-e2e"
title: "Cover runtime failure across viewports"
status: done
wave: 3
depends_on: ["02-publish-agent-runtime-availability", "03-surface-runtime-failure"]
plan: "plan.md"
spec: "../../specs/platform/requirements/agent-runtime-availability.md"
---

# Task 04: Cover runtime failure across viewports

## Intent

Prove that desktop and phone users see the outage without losing last-known
route data or breaking viewport geometry, while keeping the managed E2E runtime
alive for the rest of the suite.

## Acceptance

- Desktop shows exactly one runtime alert over a populated task route when the
  backend-shaped unavailable snapshot is injected.
- Last-known workspace/task content remains visible and the optional App status
  bar setting does not hide the alert.
- The supported restart control calls the existing capability/restart endpoints
  and enters the restart progress flow.
- Phone shows readable stacked content, a 44 px action, no horizontal overflow,
  and no overlap with route bottom navigation or safe-area content.
- The tests do not terminate the shared E2E `agentctl` or rely on timing a real
  process crash.

## TDD sequence

1. Add desktop and `mobile-` Playwright scenarios that inject unavailable state
   through `window.__KANDEV_E2E_STORE__`; observe RED until Task 03 is present.
2. Route restart capability/request responses for the desktop recovery action
   and assert the progress phase without allowing the backend to exit.
3. Add phone bounding-box, hit-target, retained-content, and horizontal-overflow
   assertions using existing mobile resilience helpers.
4. Run each focused project independently and remove only test flakiness or
   locator ambiguity revealed by the real browser.

## Files likely touched

- `apps/web/e2e/tests/layout/agent-runtime-unavailable.spec.ts`
- `apps/web/e2e/tests/layout/mobile-agent-runtime-unavailable.spec.ts`
- Existing E2E helper/page-object files only if a shared store injection or
  overflow assertion already has a canonical home.

## Dependencies

Tasks 02 and 03.

## Parallelism

`sequential` — browser assertions depend on the final backend-shaped contract
and alert composition.

## Mobile parity

The mobile test is mandatory and is named with the `mobile-` prefix so the
`mobile-chrome` project discovers it. It checks touch sizing, stacked layout,
route content retention, bottom-navigation separation, and document overflow;
desktop success alone is not sufficient.

## Verification

- `cd apps/web && pnpm e2e:run --host tests/layout/agent-runtime-unavailable.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-agent-runtime-unavailable.spec.ts`

## Inputs

- Task 03 alert and recovery surface.
- `ws-connectivity-warning.spec.ts` store-injection pattern.
- `mobile-spa-resilience.spec.ts` alert, hit-target, and overflow assertions.

## Output contract

Record both focused Playwright results and any artifacts. Confirm explicitly
that no real `agentctl` process was stopped by the scenarios.

## Results

Added a shared E2E store-injection/restart-stub helper plus independent desktop
and `mobile-` Playwright scenarios. The desktop scenario retains a populated
board, verifies the alert remains visible with the App status bar disabled,
asserts the supported restart request and `restarting` progress phase, and
clears only after an available snapshot. The phone scenario opens a real
seeded task session, verifies the stacked alert stays above the real bottom
navigation, checks the 44 px action target and document overflow, and verifies
recovery without losing the task route.

Focused validation passed:

- `pnpm e2e --project=chromium e2e/tests/layout/agent-runtime-unavailable.spec.ts` — 1 passed.
- `pnpm e2e --project=mobile-chrome e2e/tests/layout/mobile-agent-runtime-unavailable.spec.ts` — 1 passed.

Both scenarios inject store state and stub only the restart HTTP responses; no
managed E2E `agentctl` process was stopped or crashed.
