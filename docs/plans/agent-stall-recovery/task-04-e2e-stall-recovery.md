---
id: "04-e2e-stall-recovery"
title: "Prove desktop and mobile stall recovery"
status: done
wave: 4
depends_on:
  - "02-persist-stall-warning"
  - "03-render-running-warning"
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-stall-recovery.md"
---

# Task 04: Prove desktop and mobile stall recovery

## Acceptance

- Desktop Playwright proves a running stalled session shows **Cancel turn** and
  becomes input-ready after the action without navigation or restart. The
  notice is a neutral single inline row rather than an alert card.
- Mobile Playwright performs the same recovery with `.tap()`, proves a minimum
  44px content-width action, verifies the seeded notice remains a compact row,
  and finds no document horizontal overflow.
- Tests use the E2E-only session/message seed helpers; they do not wait five
  wall-clock minutes or add production test hooks.

## Verification

- `cd apps && pnpm --filter @kandev/web e2e:run tests/session/pause-resume-recovery.spec.ts`
- `cd apps && pnpm --filter @kandev/web e2e:run tests/session/mobile-pause-resume-recovery.spec.ts -- --project=mobile-chrome`

Each E2E scenario must first fail because the running-only notice is hidden,
then pass against freshly built backend and Vite artifacts.

The browser scenarios seed the persisted notice for deterministic coverage;
the real five-minute watchdog boundary is covered by the lifecycle synctest.

## Files likely touched

- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/session/pause-resume-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-pause-resume-recovery.spec.ts`

## Dependencies

Tasks 02 and 03.

## Parallelism

Sequential. It validates the integrated backend metadata and frontend
presentation.

## Inputs

- Spec cancellation and phone scenarios
- Plan E2E section and mobile design contract
- Existing `seedTaskSession`, `seedSessionMessage`, and pause/recovery E2E
  patterns

## Output contract

Report both RED failures, desktop and mobile user outcomes, exact managed-runner
results, rendered mobile inspection, files changed, blockers, and risks. Mark
this task `done` and update its plan checkbox in the same conversation.
