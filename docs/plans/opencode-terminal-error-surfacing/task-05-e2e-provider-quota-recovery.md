---
id: "05-e2e-provider-quota-recovery"
title: "Prove desktop and mobile quota recovery"
status: done
wave: 5
depends_on:
  - "03-classify-provider-failure"
  - "04-render-provider-quota-recovery"
plan: "plan.md"
spec: "../../specs/agent-stall-recovery/spec.md"
---

# Task 05: Prove Desktop and Mobile Recovery

## Acceptance

- Desktop Playwright proves a settled OpenCode quota failure replaces the
  running status with model/reset guidance, keeps technical details collapsed,
  and reveals no workspace URL or identifier when expanded.
- Mobile Playwright proves the same information and recovery actions are
  reachable by touch, actions are at least 44px, expanded details remain
  viewport-contained, and the document has no horizontal overflow.
- Tests use E2E-only session/message seeding and never exhaust real provider
  quota, read a developer's OpenCode log, or add a production test hook.

## Verification

- `cd apps && pnpm install --frozen-lockfile`
- `cd apps/web && pnpm e2e:run --no-build tests/session/provider-quota-recovery.spec.ts`
- `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-provider-quota-recovery.spec.ts`

Run against freshly built backend and Vite artifacts through the managed E2E
runner. Inspect the rendered phone viewport in addition to assertions.

## Files likely touched

- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/session/provider-quota-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-provider-quota-recovery.spec.ts`

## Dependencies

Tasks 03 and 04.

## Parallelism

Sequential. This is the integrated user-visible proof for the completed
backend and frontend contracts.

## Inputs

- Spec desktop/mobile quota scenarios and privacy guarantees
- Plan E2E section and mobile design contract
- Existing `seedTaskSession`, `seedSessionMessage`, missing-branch technical
  details, transient-retry, and mobile pause/recovery patterns

## Risks

- Seed only the safe normalized metadata contract; do not copy the real
  OpenCode workspace URL or account identifier into fixtures.
- A passing seeded UI test does not replace Task 02's real prompt-boundary
  integration regression.

## Output contract

Report RED failures, exact desktop/mobile results, rendered mobile inspection,
privacy assertions, files changed, blockers, risks, and teardown evidence.
Mark this task `done`, update its plan checkbox, and synchronize the plan's
verification results in the same conversation.

## Results

Added E2E-only seeded desktop and mobile specs. Both managed production-build
runs passed: Chromium verifies the localized card, collapsed/expanded privacy
details, and recovery actions; `mobile-chrome` verifies touch disclosure,
44px actions, viewport containment, and no horizontal overflow. The first run
found and fixed a fixture requirement for `completed_at`; no provider quota or
developer log was used.
