---
id: "02-prove-restart-activity-recovery"
title: "Prove restart activity recovery"
status: done
wave: 2
depends_on: ["01-reconcile-authoritative-session-activity"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BACKGROUND-WORK-LIVENESS-001
acceptance_criteria:
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.3
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.9
system_design:
  - ../../specs/platform/system-design/background-work-liveness.md
---

# Task 02: Prove Restart Activity Recovery

## Summary

Prove that a resumed settled agent does not show pre-restart background work in
the real browser flow.

## In scope

- Extend the existing ACP restart/resume desktop Playwright coverage.
- Establish the retained pre-restart projection deterministically.
- Assert the durable state, composer readiness, and visible status outcome.

## Out of scope

- A separate mobile E2E scenario.
- Changes to production session status components.
- Additional backend restart fixtures or provider behavior.

## Acceptance

- After a retained background projection and backend restart, the authoritative
  API reports the resumed session as `WAITING_FOR_INPUT` without activity.
- Opening or refreshing the task shows a ready composer and no background-work
  status.
- Existing resume transcript, message continuity, and follow-up prompt
  assertions remain green.

## Verification

```bash
cd apps && pnpm --filter @kandev/web e2e:run -- tests/session/session-resume.spec.ts -- --grep "clears stale background activity after backend restart"
```

## Files likely touched

- `apps/web/e2e/tests/session/session-resume.spec.ts`
- `apps/web/e2e/helpers/session.ts` only if a reusable activity assertion is
  needed.

## Dependencies

Task 01 provides the authoritative activity reconciliation.

## Parallelism

`sequential`

The browser case consumes Task 01 and shares the restart-heavy session fixture.

## Inputs

- `AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.9`.
- The existing ACP backend-restart and session-resume Playwright scenario.
- The existing E2E-only store exposure and foreground-activity helpers.

## Risks

- The stale state must be seeded without bypassing the real authoritative list
  refresh that the test is intended to prove.
- Restart-heavy tests need the existing retry and bounded state waits to avoid
  timing-only failures.

## Results

- Added a focused ACP restart scenario that settles and resumes a real mock
  session, deterministically seeds the missed-event stale projection, and
  triggers the real session-list refresh.
- The test demonstrates the expected stale-activity timeout with the old merge
  behavior and passes with authoritative reconciliation enabled.
- The test confirms `WAITING_FOR_INPUT`, removes the background-work status,
  and restores the ready composer in desktop Chromium.
- No separate mobile scenario was added because the fix changes shared state
  reconciliation only and does not alter layout, navigation, touch, or
  viewport-specific behavior.
