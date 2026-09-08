---
id: "05-runtime-update-e2e"
title: "Verify the runtime update user flows"
status: done
wave: 4
depends_on: ["03-backend-update-pipeline", "04-settings-update-ui"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
role: test-engineer
model_tier: default
---

# Task 05: Verify the runtime update user flows

## Acceptance

- A production-build Playwright flow proves update submission, visible
  current-to-target progress/output, terminal success/failure, retained-job
  rehydration, and refreshed catalogue/model rendering through the UI.
- Mobile Chrome coverage proves the action is touch-reachable, at least 44 px
  high, content/log/retry states remain vertically reachable, and the page
  never gains horizontal overflow.
- Tests use deterministic fixtures and stable test IDs; they do not require
  registry access or add product-only test hooks.

## Verification

- `cd apps/web && pnpm e2e:run tests/settings/agent-runtime-update.spec.ts`
- `cd apps/web && pnpm e2e:run tests/settings/mobile-agent-runtime-update.spec.ts -- --project=mobile-chrome`

## Files likely touched

- `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`
- Existing Settings page objects/helpers only if shared interaction warrants it
- Runtime-update components only for stable `data-testid` attributes or a
  directly exposed regression found by the RED test

## Dependencies

Tasks 03 and 04 must be integrated and their targeted verification green.

## Inputs

- Every user-visible scenario in the spec
- Plan `E2E Tests`
- `apps/web/e2e/tests/settings/agent-install-streaming.spec.ts`
- E2E managed-runner, production-build, selector, and mobile interaction rules

## Output contract

Report intent/acceptance, base/head SHA, changed specs/helpers, named spec
scenarios, risk tags (`e2e`, `production-build`, `mobile-chrome`), exact RED
and GREEN commands/results, artifacts inspected, and uncertainties. Update only
this task file to `done`; do not edit `plan.md`.
