---
id: "05-clarification-regression-green"
title: "Clarification regression green"
status: completed
wave: 5
depends_on:
  [
    "01-clarification-regression-red",
    "02-current-turn-backend-authority",
    "03-summary-pending-convergence",
    "04-pending-owner-navigation",
  ]
plan: "plan.md"
spec: "../../specs/tasks/requirements/clarification-active-lifecycle.md"
---

# Task 05: Clarification regression green

## Acceptance

- The exact desktop and phone black-box regressions captured in Task 01 pass against a fresh managed
  production build without retries masking failure.
- The existing detached current-turn deferred-answer E2E still passes, proving newer-turn
  supersession did not collapse the intended timeout recovery window.
- Reload, drawer dismissal, active session, task URL, question visibility, and no-horizontal-overflow
  assertions all pass; no main-instance data is mutated.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run tests/task/sidebar-pending-question.spec.ts)
(cd apps/web && pnpm e2e:run --no-build tests/chat/clarification.spec.ts -- --grep "timeout detaches clarification")
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-sidebar-task-actions.spec.ts)
git diff --check
```

Confirm Playwright discovers nonzero tests in both `chromium` and `mobile-chrome` before treating the
runs as evidence.

## Files likely touched

- `apps/web/e2e/tests/task/sidebar-pending-question.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`
- `apps/web/e2e/helpers/clarification.ts` only if introduced in Task 01
- `apps/web/e2e/pages/session-page.ts` only if introduced in Task 01

## Dependencies

Tasks 01-04.

## Parallelism

Sequential. This is the cross-layer acceptance gate for all prior tasks.

## Inputs

- Task 01's exact failing assertions and artifacts.
- Every user-visible scenario in the repair spec.
- E2E managed-runner guidance, production-build requirement, session-state polling, reload proof, and
  mobile touch rules.

## Risks

- Do not convert a product failure into a test pass with `force`, retries, fixed sleeps, or broader
  timeouts.
- Inspect fresh `error-context.md` and screenshots if a failure changes signature.
- The second and third commands intentionally reuse unchanged production artifacts from the first
  managed build; rerun without `--no-build` after any source edit.

## Output contract

Report discovered test counts, exact command outcomes, relevant artifact paths, and teardown/cleanup
evidence. Reconcile actual files, replace `## Results`, mark all plan checks/results complete only when
green, and leave no temporary capture specs or processes.

## Results

- Fresh managed production artifacts built successfully; no retry or widened timeout was used.
- Chromium discovered 3 tests in `sidebar-pending-question.spec.ts`; all 3 passed, including stale
  reload suppression and secondary-session ownership.
- Chromium discovered 1 matching detached-timeout test; it passed and accepted the deferred answer.
- `mobile-chrome` discovered 12 tests; all 12 passed. The new `.tap()` owner-selection case verified
  drawer dismissal, active secondary session, task URL, question visibility, and horizontal fit.
- `git diff --check` passed. Managed runners exited cleanly and used isolated `/tmp/kandev-e2e-*`
  databases/repositories; the read-only :9998 instance was not mutated.
