---
id: "02-prove-desktop-mobile-recency"
title: "Prove desktop and mobile recency"
status: done
wave: 2
depends_on:
  - "01-resolve-recent-profile"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-RECENT-USE-001
  - REQ-AGENTS-PROFILE-RECENT-USE-002
  - REQ-AGENTS-PROFILE-RECENT-USE-003
acceptance_criteria:
  - AC-AGENTS-PROFILE-RECENT-USE-001.2
  - AC-AGENTS-PROFILE-RECENT-USE-001.4
  - AC-AGENTS-PROFILE-RECENT-USE-001.6
  - AC-AGENTS-PROFILE-RECENT-USE-001.8
  - AC-AGENTS-PROFILE-RECENT-USE-002.1
  - AC-AGENTS-PROFILE-RECENT-USE-003.1
  - AC-AGENTS-PROFILE-RECENT-USE-003.5
system_design:
  - ../../specs/agents/system-design/profile-recent-use.md
---

# Task 02: Prove Desktop and Mobile Recency

## Summary

Add focused Playwright regressions for the shared New Agent dialog. Prove that
desktop and mobile select, show, and launch the recent `task_session` profile
when the current task uses another profile.

## In scope

- Write the desktop Playwright scenario and observe the expected RED result.
- Assert the selected trigger, first option, and effective backend session
  profile.
- Add the same outcome through the existing mobile New Agent entry point.
- Remove created profiles in `finally` cleanup.

## Out of scope

- New layout, geometry, or screenshot assertions.
- Repeating persistence caps and synchronization tests.
- Quick Chat and configuration-chat recency coverage.

## Acceptance

- Desktop and mobile select profile B when `task_session` history ranks B and
  the current task session uses profile A.
- Starting the dialog creates a session whose effective profile is B.
- Handoff and cancellation behavior in existing scenarios remains unchanged.

## Verification

```bash
cd apps/web && pnpm e2e:run --project chromium tests/session/new-session-dialog.spec.ts -- --grep "uses task-session recency"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-new-session-dialog.spec.ts -- --grep "uses task-session recency"
```

## Files likely touched

- `apps/web/e2e/tests/session/new-session-dialog.spec.ts`
- `apps/web/e2e/tests/session/mobile-new-session-dialog.spec.ts`
- `apps/web/e2e/pages/new-session-dialog-page.ts`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/helpers/api-client.ts`

## Dependencies

- Task 01 provides the corrected selection and launch behavior.

## Risks

- A one-task setup can mask the defect after profile B becomes the active
  session.
- The test must poll backend session state before it asserts the effective
  profile.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-PROFILE-RECENT-USE-001`
- Existing desktop and mobile new-session Playwright files.
- E2E fixture-state and UI-state cleanup guidance.

## Results

- Added shared E2E setup that creates two eligible profiles, records profile B
  through a successful source-task launch, and creates a profile-A target task
  so the recency default is observable.
- Added desktop coverage for the selected trigger, first selector option, and
  effective backend session profile.
- Added equivalent mobile coverage through the sessions picker and touch
  interactions.
- Extracted the recency-specific profile controls into
  `apps/web/e2e/pages/new-session-dialog-page.ts`; desktop and mobile tests
  assert a single matching profile option and its first-position ordering.
- `cd apps/web && pnpm e2e:run --project chromium tests/session/new-session-dialog.spec.ts -- --grep "uses task-session recency"` passed (1 test).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-new-session-dialog.spec.ts -- --grep "uses task-session recency"` passed (1 test).
