---
id: "05-prove-rollback-desktop-mobile"
title: "Prove rollback on desktop and mobile"
status: done
wave: 5
depends_on: ["04-add-responsive-version-recovery-ui"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 05: Prove rollback on desktop and mobile

## Acceptance

- Desktop E2E covers an unknown-version repair state and selects an older stable
  version for rollback, reviews the exact command, and submits one rollback.
- `mobile-chrome` completes the same behavior through the bottom drawer with no
  nested overlay, clipped action, browser-chrome collision, or page overflow.
- Failed candidate validation keeps the previous active marker and lets the
  operator select another version.
- Existing update, same-version, install-conflict, output, and page-reload
  scenarios continue to pass with explicit targets.

## TDD sequence

1. Update mock routes and helpers to require target query/body values.
2. Add the desktop rollback scenario and confirm RED before UI wiring.
3. Add the mobile parity and containment scenario.
4. Add failed-candidate preservation coverage where it is not already proven
   by component tests.
5. Run focused projects, inspect artifacts on failure, and clean generated
   screenshots/traces/videos not intentionally retained.

## Files likely touched

- `apps/web/e2e/tests/settings/agent-runtime-update-helpers.ts`
- `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`

## Verification

```bash
cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/settings/agent-runtime-update.spec.ts
cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/settings/mobile-agent-runtime-update.spec.ts
```

## Risks

- Assert request payload and rendered outcome, not only button visibility.
- Keep deterministic mocked registry data; do not contact public npm in E2E.
- Verify one scroll owner and touch reachability at the narrow shipped viewport.

## Output contract

Record test counts, viewport, artifacts, cleanup evidence, checks, and risks in
Results. Update this task and `plan.md` status.

## Results

GREEN verification:

- `rtk pnpm e2e:raw --project=chromium e2e/tests/settings/agent-runtime-update.spec.ts` — 6 passed.
- `rtk pnpm e2e:raw --project=mobile-chrome e2e/tests/settings/mobile-agent-runtime-update.spec.ts` — 4 passed.
- Request assertions verified target query/body values for update and rollback,
  operation labels, up-to-date disabling, failed-job retry, mobile drawer
  containment, and long-output scrolling.
- Fresh disposable capture runs passed once per viewport. The compressed
  synthetic assets are `desktop-runtime-version-selector` and
  `mobile-runtime-version-selector`; both manifest entries map to non-empty
  PNGs under ignored `apps/web/.pr-assets`.

Temporary capture specs were removed after publication assets were validated.
The only environment cleanup was disposal of the recoverable Go build cache
after an `ENOSPC` during the first capture attempt.
