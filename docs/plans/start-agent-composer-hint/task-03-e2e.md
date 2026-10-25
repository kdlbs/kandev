---
id: "03-e2e"
title: "E2E: hint replaces resume button, desktop and mobile"
status: done
wave: 1
parallelism: sequential
depends_on: ["02-composer-hint-ui"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/prevent-agent-autostart-on-open.md"
---

# Task 03: E2E — hint replaces resume button, desktop and mobile

## Acceptance

- `apps/web/e2e/tests/settings/prevent-auto-start-on-open.spec.ts`,
  "post-restart session does not auto-resume when the preference is on":
  - after `backend.restart()` + `testPage.reload()`, asserts
    `composer-agent-start-hint` is visible and
    `session-resume-start-button` has count 0 (and "Resumed agent" never
    appears on its own);
  - sends `/e2e:simple-message` via the composer (`session.sendMessage`) and
    asserts the mock response ("simple mock response") becomes visible and
    "Resumed agent" appears — sending a message is the explicit start.
  - `test.setTimeout(180_000)` retained; settings/task teardown unchanged.
- The two final-step tests in the same file still assert
  `task-description-start-button` visibility/absence exactly as today
  (CREATED button unchanged, setting-off control unchanged).
- `apps/web/e2e/tests/settings/mobile-prevent-auto-start-on-open.spec.ts`
  (CREATED button on mobile) stays green unchanged.
- New `apps/web/e2e/tests/settings/mobile-agent-start-hint.spec.ts`
  (`mobile-chrome` project, phone viewport from the project device, no
  per-test `setViewportSize`): same restart pattern — task with
  `/e2e:simple-message` description, first turn completes, `backend.restart()`
  + `testPage.reload()`, then asserts `composer-agent-start-hint` visible,
  `session-resume-start-button` count 0, sends a message, and asserts the
  mock response appears. Timeouts mirror the desktop restart test
  (`test.setTimeout(180_000)`).

## Verification

```bash
(cd apps/web && KANDEV_E2E_MOCK=true pnpm e2e:raw --project=chromium settings/prevent-auto-start-on-open.spec.ts)
(cd apps/web && KANDEV_E2E_MOCK=true pnpm e2e:raw --project=mobile-chrome settings/mobile-agent-start-hint.spec.ts settings/mobile-prevent-auto-start-on-open.spec.ts)
```

## Files Likely Touched

- `apps/web/e2e/tests/settings/prevent-auto-start-on-open.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-start-hint.spec.ts` (new)

## Dependencies

Task 02 (the hint must be rendered before the specs can assert it).

## Inputs

- Current restart test body at
  `prevent-auto-start-on-open.spec.ts:117-166` (uses `backend.restart()` +
  `testPage.reload()`, `SessionPage.waitForLoad`,
  `session.sendMessage("/e2e:simple-message")`,
  `session.expectChatResponseVisible("simple mock response", 0, …)`).
- Mobile restart flow mirrors the desktop one; the mobile-chrome project
  auto-selects `mobile-*.spec.ts` files and supplies the device.

## Output Contract

Desktop and mobile E2E prove: a recovered-idle session shows the composer
hint and no Start agent button; sending a message starts the agent and the
mock response appears; the CREATED final-step button flows (desktop and
mobile) are regression-green.
