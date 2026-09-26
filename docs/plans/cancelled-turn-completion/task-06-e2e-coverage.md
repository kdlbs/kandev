---
id: "06-e2e-coverage"
title: "Cancelled turn E2E coverage"
status: completed
wave: 4
depends_on: ["03-cancellation-routing", "04-settings-ui"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/workflow-cancelled-turn-completion.md"
---

# Task 06: Cancelled turn E2E coverage

## Acceptance

- Desktop Playwright proves persisted enabled and disabled settings produce complete-and-advance and pause-in-place behavior through the real cancel button, with cancellation visibility and exactly one destination transition.
- Mobile Playwright enables/saves the setting by touch, reloads it, cancels a delayed turn, observes the same workflow transition, and asserts the associated label's 44px touch target, the existing scroll owner, and no document horizontal overflow.
- Existing pause/resume and parked-queue scenarios affected by the new `simple` template default are updated only where their expected workflow position changes; their same-session and queue contracts remain intact.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run tests/workflow/workflow-cancel-completion.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/workflow/mobile-workflow-cancel-completion.spec.ts)
```

## Files Likely Touched

- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/pages/workflow-settings-page.ts`
- `apps/web/e2e/tests/workflow/workflow-cancel-completion.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-cancel-completion.spec.ts`
- `apps/web/e2e/tests/session/pause-resume-recovery.spec.ts` (only if the template-default expectation changes)
- `apps/web/e2e/tests/session/mobile-pause-resume-recovery.spec.ts` (only if the template-default expectation changes)

## Dependencies

Tasks 03 and 04.

## Parallelism

Sequential. E2E depends on the complete backend and frontend behavior and may need to reconcile existing tests that use newly instantiated `simple` workflows.

## Inputs

- Every user-visible spec scenario selected in `plan.md` E2E coverage.
- Existing workflow settings, workflow auto-advance, pause/resume recovery, mobile pause, and API helper patterns.
- Mobile-parity requirement to use a `mobile-*.spec.ts` file, `.tap()`, touch-sized controls, and document-overflow assertions.

## Risks

- Use a deterministic delayed mock-agent turn so cancellation happens while the session is genuinely running; do not add fixed sleeps or inflate timeouts.
- Confirm Playwright discovers the mobile spec under `mobile-chrome` and that the second run reuses the freshly built artifacts from the first command.
- Preserve worker-scoped seed state and restore any canonical workflow-step edits in `test.afterEach`, or create disposable workflows instead.

## Output Contract

Report desktop/mobile scenarios and discovered test counts, files changed, exact managed-runner results, artifact/cleanup evidence, blockers, and residual flake risks. Update this task, `plan.md`, and `## Verification Results` in the same conversation.

## Results

Added managed Playwright coverage:

- Desktop enabled/disabled scenarios seed a real turn with a delayed response, click the existing Cancel action, verify the cancellation settles, and assert advance-versus-stay plus a single surviving task session.
- Mobile coverage enables the setting through the workflow editor with `.tap()`, verifies persistence after reload, measures the actual `min-h-11` associated label, cancels a delayed turn by touch, verifies the destination, and asserts no document horizontal overflow.
- The E2E API helper accepts `cancel_triggers_turn_complete` updates and the workflow settings page exposes a touch-aware policy helper/save path.

Verification:

- `rtk pnpm e2e:run --no-build tests/workflow/workflow-cancel-completion.spec.ts` — 2 passed.
- `rtk pnpm e2e:run --no-build --project mobile-chrome tests/workflow/mobile-workflow-cancel-completion.spec.ts` — 1 passed.
- A follow-up managed mobile run passed 1/1 on the first attempt, including the label geometry assertion; the earlier transient settings hydration timeout did not reproduce.

The initial managed runs exposed only test-harness assumptions (nullable empty history and a mobile-only stepper/cancel-button sizing difference); assertions were adjusted to the stable API/current-step and existing touch contracts. No product behavior changes were needed.
