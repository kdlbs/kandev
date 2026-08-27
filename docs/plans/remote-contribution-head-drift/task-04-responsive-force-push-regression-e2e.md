---
id: "04-responsive-force-push-regression-e2e"
title: "Prove rewritten PR behavior end to end"
status: done
wave: 4
depends_on: ["03-diverged-changes-ui-and-mobile-parity"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 04: Prove Rewritten PR Behavior End to End

Recreate the reported stale-checkout/current-PR mismatch in browser tests and prove that the behavior is
correct on desktop and mobile.

## Desktop scenario

Extend `apps/web/e2e/tests/git/git-changes-panel.spec.ts` with a deterministic local Git graph and
mocked current GitHub PR commits:

1. Create a task checkout with several commits representing the original contributor history and record
   its local/upstream head.
2. Associate a mocked open PR whose current commit list uses different SHAs, simulating a force-pushed
   rewrite. At least one rewritten commit may intentionally reuse a message and file totals to prove
   metadata is not used for deduplication.
3. Open Changes and assert:
   - the drift warning is visible;
   - “Current PR commits” and “Local checkout commits” are separate;
   - each list contains only its own SHAs/messages;
   - the old local rows have the neutral checkout marker, not the green unpushed arrow;
   - Push and Pull are unavailable with the reconciliation reason;
   - no action mutated local HEAD or the worktree.
4. Add a safe local-ahead control case: provider/upstream head match and one local commit exists. Assert
   one Push commit even when base `ahead` is larger.

Reuse `mockGitHubAddPRs`, `mockGitHubAddPRCommits`, and `mockGitHubAssociateTaskPR`. Extend the E2E helper
only if the test cannot establish the required upstream ref with its real temporary repository.

## Mobile scenario

Add `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts` under the `mobile-chrome` project:

- Open the same kind of linked task on Pixel 5 dimensions.
- Enter the shared Changes panel through the normal mobile navigation.
- Assert the warning and both histories are readable without horizontal overflow.
- Open the 44px Git actions trigger and assert Pull/Push are disabled while Commit remains available.
- Assert the panel keeps one vertical scroll owner and the warning/headings can be reached by ordinary
  touch scrolling.

Use stable roles, translated accessible names, and `data-testid` only where no semantic locator exists.
Do not use fixed sleeps.

## Acceptance

- The original visual failure is reproduced by test data and fails before Tasks 01–03.
- Desktop and mobile tests pass after the fix.
- Tests prove both containment of a rewritten history and preservation of an ordinary local-ahead flow.
- The test does not depend on a live GitHub account or network timing.

## Files likely touched

- `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts` (new)
- `apps/web/e2e/helpers/api-client.ts` only if a narrow mock/status helper is required
- relevant Changes/Session page object only if semantic locators cannot express the new state

## Verification

Run the focused specs first, then the related browser project if time permits:

```bash
cd apps/web && pnpm e2e:raw --project=chromium tests/git/git-changes-panel.spec.ts --grep 'rewritten PR|upstream push count'
cd apps/web && pnpm e2e:raw --project=mobile-chrome tests/git/mobile-pr-checkout-drift.spec.ts
```

## Dependencies and parallelism

Depends on Task 03. Run after the UI contract is stable; this task may expose wiring defects that must
be fixed in the owning files rather than hidden in test helpers.

## Output contract

Record the red failure signature, green desktop/mobile command output, screenshots or traces for any
failure investigated, viewport/scroll assertions, and blockers. Update this task and the plan checkbox
when complete.

## Completion evidence

- The browser fixture initially exposed a missing local `origin/main` tracking ref. The regression
  setup now restores the temporary remote URL, fetches the ref, and establishes the graph without
  relying on fixture history.
- Desktop rewrite and local-ahead scenarios passed with Chromium; the rewritten-history scenario passed
  on Pixel 5. Assertions cover separate histories, accurate provenance, disabled remote actions,
  unchanged local HEAD/worktree, one scroll owner, and no horizontal overflow.
- Screenshot capture hooks are present for the desktop and mobile regression states; fresh PR assets
  will be captured during final delivery verification.
