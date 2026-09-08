---
id: "05-desktop-mobile-drift-e2e"
title: "Desktop and mobile drift E2E"
status: completed
wave: 4
depends_on:
  - "01-exact-lease-contribution-operations"
  - "03-desktop-local-first-contribution-ui"
  - "04-mobile-contribution-version-choices"
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 05: Desktop and Mobile Drift E2E

Prove the full local-first flow against real local Git refs and provider mocks. Cover desktop remote
replacement, desktop provider adoption, and mobile remote replacement.

## Inputs

- Spec rewritten-history and mobile scenarios.
- Plan section: E2E Tests.
- Existing drift coverage in `git-changes-panel.spec.ts` and
  `mobile-pr-checkout-drift.spec.ts`.
- Tasks 01, 03, and 04.

## Acceptance

1. Desktop replacement changes the real contribution ref only after confirmation and exact lease
   success. A stale lease changes no ref.
2. Desktop provider adoption preserves old HEAD on a recovery branch and moves the task branch to the
   provider head.
3. Mobile completes remote replacement from the Git menu and satisfies viewport, touch-target, scroll,
   and horizontal-overflow checks.

## Files Likely Touched

- `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts`
- `apps/web/e2e/tests/git/git-changes-panel-helpers.ts` (new, only if both specs share setup)
- `apps/web/e2e/helpers/api-client.ts` (only if provider mock refresh needs a missing helper)

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run --project chromium tests/git/git-changes-panel.spec.ts -- --grep "local-first contribution" && pnpm e2e:run --project mobile-chrome tests/git/mobile-pr-checkout-drift.spec.ts
```

## Dependencies

Tasks 01, 03, and 04.

## Parallelism

Sequential. This task integrates shared backend and responsive frontend behavior.

## Risks

- Synthetic provider SHAs cannot prove force-with-lease behavior. Use commits that exist in the test
  remote.
- Update provider mocks after remote mutation so aligned-state assertions observe current data.
- Confirm Playwright discovers the intended desktop and mobile test counts before accepting results.
- Use `.tap()` for touch-specific mobile controls and remove all temporary runtime artifacts.

## Output Contract

Report the discovered test counts, exact commands, Git ref assertions, screenshot paths, and cleanup
evidence. Update this task and `plan.md` in the same conversation.

## Results

Completed 2026-08-12.

- Desktop and mobile drift fixtures now create provider histories from real local Git commits. The UI
  tests verify local-first status, collapsed provider history, preserved local history, exact provider
  head disclosure, scoped action availability, mobile touch targets, one scroll owner, and zero
  horizontal overflow.
- Desktop verification passed:
  `pnpm e2e:run --no-build --project chromium tests/git/git-changes-panel.spec.ts -- --grep "local-first contribution keeps rewritten provider history separate" --retries=0`
  (1 passed).
- Mobile verification passed:
  `pnpm e2e:run --no-build --project mobile-chrome tests/git/mobile-pr-checkout-drift.spec.ts -- --grep "local-first contribution menu preserves local history after a rewrite" --retries=0`
  (1 passed).
- The real-Git process suite proves successful exact-lease replacement, stale-lease no-mutation,
  provider adoption, recovery-branch preservation, dirty-tree rejection, and stale-fetch rejection.
- The current REST E2E helper creates a TaskPR association but cannot author the server-owned
  `RemoteContribution` binding. Therefore the browser scenarios stop at the confirmation boundary;
  destructive ref mutation and provider adoption are covered by the real-Git process tests rather than
  by these REST-created browser tasks. A future bound-provider E2E can use the MCP task-creation path.
