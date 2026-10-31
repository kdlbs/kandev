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
requirements:
  - REQ-TASKS-REMOTE-CONTRIBUTION-TASKS-001
acceptance_criteria:
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.4
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.5
  - AC-TASKS-REMOTE-CONTRIBUTION-TASKS-001.6
system_design:
  - ../../specs/tasks/system-design/remote-contribution-tasks.md
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

## Mutation receipt correction (2026-10-27)

SDK PR CI on `e0415183a79` read the prior local HEAD after the mobile
confirmation disappeared. The failed attempt has no trace; its later screenshot
shows a success toast and recovery branch. It does not establish the operation's
completion time relative to the failed HEAD read.

A controlled real file-watch update establishes the invalid test boundary:
while the second `worktree.use_contribution` request is held, a real pending
Git-detail update removes the confirmation and HEAD remains local. The former
immediate HEAD expectation fails. Removing the probe file, releasing enrichment
and forwarding the original request produces a real successful response, then
all original HEAD, cleanliness, and recovery-branch checks pass.

The normal fixture now arms the existing request-ID-correlated WebSocket
watcher before confirmation, matching the prepared session, expected provider
head, and explicit workspace-root repository scope. It asserts the operation's
success before dialog absence and Git postconditions. Dirty-worktree refusal,
lease, recovery-branch, viewport, and focus checks remain intact. This is a test
completion correction, not a claim that the original CI ordering was traced.
The temporary probe was removed. Full paired mobile validation follows.


Validation: the controlled former HEAD assertion failed while the actual request
was held and the real pending-detail event had removed the dialog. The control
passed after waiting for the matching successful response and preserving all
Git checks. Both normal mobile cases passed three repetitions each (6/6) with
retries disabled. Independent review found no actionable issue in the fixture
correction. The original CI request timing remains untraced.
