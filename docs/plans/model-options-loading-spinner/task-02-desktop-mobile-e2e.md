---
id: "02-desktop-mobile-e2e"
title: "Prove selector loading state"
status: done
wave: 2
depends_on:
  - "01-selector-loading-state"
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-provider-options.md"
---

# Task 02: Prove Selector Loading State

## Acceptance conditions

1. Desktop Playwright coverage shows the loading row inside the open selector
   and the selected-row spinner while the model-option response is pending.
2. Mobile Playwright coverage proves the same touch flow without horizontal
   overflow.
3. Both tests prove that resolved option controls replace the status spinner
   and the selected-row spinner is replaced by the check icon after the
   response.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run --project chromium tests/settings/agent-profile-acp.spec.ts && pnpm e2e:run --project mobile-chrome tests/settings/mobile-agent-profile-config-selector.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/settings/agent-profile-acp.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-config-selector.spec.ts`

## Dependencies

- Task 01 provides the loading row and its stable selector.

## Parallelism

Sequential. These tests depend on Task 01 and modify existing profile flows.

## Inputs

- The `E2E Tests` and `Mobile design note` sections in `plan.md`.
- `apps/web/e2e/helpers/causal-waits.ts` for `injectLatency` and HTTP response
  synchronization.
- The existing desktop and mobile profile model-option tests.

## Output contract

Report the delayed route, assertions, discovered test counts, changed files,
commands, risks, and blockers. Update this task and `plan.md` with the exact
results.

## Risks

- A cached response can skip the route delay. The test must select a model that
  the page has not resolved.
- The delay is test stimulus. Do not add raw sleeps or larger assertion
  timeouts.

## Results

Added resolver latency to the existing desktop and mobile profile flows. Both
flows assert that the selector remains open, both loading indicators are
visible, stale option controls are absent, the indicators are removed after
resolution, the selected-row check returns, and resolved controls return. The
mobile flow also asserts that the document has no horizontal overflow and uses
touch actions.

Verification passed:

- Desktop profile E2E: 4 tests passed.
- Mobile profile E2E: 2 tests passed.
- Disposable PR-asset capture: 1 desktop test and 1 mobile test passed. The
  resulting screenshots were inspected, compressed, and verified against the
  combined asset manifest.
