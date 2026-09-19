---
id: "06-cancel-reload-regression-e2e"
title: "Cancel reload regression"
status: completed
wave: 4
depends_on: ["05-backend-owned-cancel-control"]
plan: "plan.md"
spec: "../../specs/ui/requirements/cancel-turn-progress.md"
---

# Task 06: Cancel reload regression

## Acceptance

- The desktop regression sends cancellation to the backend during an existing slow mock turn,
  observes backend `cancellation_pending=true`, switches tasks, returns, reloads, and sees the same
  disabled animated control until cancellation settles.
- A `mobile-chrome` regression taps cancel, waits for backend-owned pending state, reloads, and
  observes the same shared-control behavior without relying on desktop-only navigation.
- The old outbound-request hold helper and assertions are removed; teardown leaves no held frames,
  stale pending state, or running seeded turn.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/chat/cancel-progress-task-switch.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/chat/mobile-cancel-progress-reload.spec.ts)
(cd apps/web && pnpm run typecheck)
```

Follow TDD: first forward the cancel request and prove the reloaded control loses progress without
backend hydration, then enable Tasks 03-05 and make both viewport flows pass. Wait on the exposed
backend-derived store field rather than a timeout before reloading.

## Files likely touched

- `apps/web/e2e/helpers/session-store.ts`
- `apps/web/e2e/helpers/ws-drop.ts`
- `apps/web/e2e/tests/chat/cancel-progress-task-switch.spec.ts`
- `apps/web/e2e/tests/chat/mobile-cancel-progress-reload.spec.ts`

## Dependencies

Task 05.

## Parallelism

Sequential. This task validates the complete backend-to-hydration-to-control path.

## Inputs

- Spec: task navigation, reload, session-isolation, failure, and compact-mobile scenarios.
- Plan: `E2E Tests` and `Mobile design contract`.
- Existing patterns: the `/slow` mock-agent command, `SessionPage.clickTaskInSidebar`,
  `page.reload()`, `waitForActiveSessionForegroundActivity`, and mobile-only spec naming.

## Output contract

Report the initial failing reload assertion, final desktop/mobile Playwright counts and durations,
seeded-turn settlement/cleanup evidence, exact files/commands, blockers/risks, and synchronize this
task plus `plan.md` in the same primary conversation.

## Results

Replaced request-held coverage with backend-accepted task navigation and reload coverage.

- `cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium e2e/tests/chat/cancel-progress-task-switch.spec.ts` — 1 passed (12.1s) after rebuilding the backend binary and Vite assets.
- `cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=mobile-chrome e2e/tests/chat/mobile-cancel-progress-reload.spec.ts` — 1 passed (10.4s).
- `cd apps/web && pnpm run typecheck` — passed.
- `git diff --check` — passed.

The first desktop attempt exposed that the E2E fixture was launching a stale backend binary; rebuilding
`apps/backend/bin/kandev` and `apps/web/dist` made the new live event available. The first mobile
attempt used the desktop keyboard helper; it was corrected to submit through the mobile button. Both
final tests observe backend `cancellation_pending=true`, verify the disabled spinner after
navigation/reload, and wait for the explicit `false` settle. The old held-frame helper was removed,
and each run used only the isolated mock backend/workspace.
