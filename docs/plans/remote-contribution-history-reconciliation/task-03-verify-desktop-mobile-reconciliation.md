---
id: "03-verify-desktop-mobile-reconciliation"
title: "Verify desktop and mobile reconciliation"
status: complete
wave: 3
depends_on: ["01-recover-provider-commit-evidence", "02-reconcile-and-style-commit-provenance"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 03: Verify Desktop and Mobile Reconciliation

Prove the final Changes behavior through the existing desktop and phone surfaces. Use real provider
fixture transitions and semantic assertions.

## Inputs

- Spec scenarios: Show a provider fast-forward safely, Keep Changes usable when provider history is
  unavailable, and Distinguish commits when the provider is ahead.
- Plan sections: Desktop and mobile contract and Tests.
- Existing desktop exemplar: `apps/web/e2e/tests/git/git-changes-panel.spec.ts`.
- Existing mobile exemplar: `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts`.

## Acceptance

1. A transient provider read failure does not show a provider-history warning in the Changes body.
2. A successful retry reconciles the view without a user action.
3. A provider-only commit appears before shared commits with current-PR provenance.
4. Shared commits keep neutral provenance.
5. A confirmed divergence shows violet current-PR commits and amber local-checkout commits.
6. Desktop and mobile expose the same accessible provenance labels.
7. Provider-ahead without an upstream offers neither an unsafe Push nor a misleading Pull.
8. The mobile document keeps zero horizontal overflow and existing touch geometry.

## Files Likely Touched

- `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts`
- Existing Git provider fixture helpers only when they cannot express a one-request failure followed by
  success.

## TDD Sequence

1. Add the desktop assertions and confirm that the warning, order, or provenance test fails.
2. Add the mobile assertions and confirm the provenance test fails.
3. Add a one-request provider commit failure before navigation and verify the automatic retry keeps
   checkout history visible, removes the warning, and adds the provider-only commit.
4. Complete any missing product behavior in Task 01 or Task 02 files.
5. Run each focused spec with `--retries=0`.

Do not use arbitrary sleeps. Poll the semantic Changes state. Confirm that Playwright discovers the
expected tests in each owning project.

## Verification

Desktop:

```bash
cd apps/web && pnpm e2e:run tests/git/git-changes-panel.spec.ts -- --retries=0
```

Mobile:

```bash
cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-pr-checkout-drift.spec.ts -- --retries=0
```

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential. Run after the shared behavior is complete.

## Risks

- Use one-shot failure fixtures so the intended retry is deterministic and later assertions do not
  inherit a queued failure.
- Assert semantic provenance attributes and labels. Do not depend only on generated CSS strings.
- Keep the mobile test in `mobile-chrome`. The default project excludes mobile specs.

## Output Contract

Report the fixture transition, desktop and mobile scenarios, discovered test counts, exact command
results, and any remaining browser-specific behavior. Update this task and `plan.md` in the same
conversation.

## Results

- The desktop divergence scenario asserts `Rewritten provider commit 15` is the first provider row,
  proving newest-first provider history.
- The mobile divergence scenario asserts `Mobile rewritten provider commit 15` is the first provider
  row and exercises the same ordering contract on `mobile-chrome`.
- A one-shot mock provider failure followed by the existing automatic retry now covers the original
  failure-and-retry workflow. The desktop PR-only scenario and two mobile scenarios all keep local
  history visible, show the provider-only commit after retry, and finish without the provider warning.
- The desktop command passed 21 tests with Chromium before the review remediation:
  `cd apps/web && pnpm e2e:run tests/git/git-changes-panel.spec.ts -- --retries=0`
- The review-remediation desktop retry scenario passed 1 test with Chromium:
  `cd apps/web && pnpm e2e:run tests/git/git-changes-panel.spec.ts -- --grep "PR-only commit uses GitHub details when local history is stale" --retries=0`
- Mobile command passed 1 test with `mobile-chrome`:
  `cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-pr-checkout-drift.spec.ts -- --retries=0`
- The review-remediation mobile retry scenarios passed 1 test each with `mobile-chrome`:
  `cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-pr-checkout-drift.spec.ts -- --retries=0`
  `cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-changes-panel.spec.ts -- --grep "PR-only commit opens the remote commit sheet when local history is stale" --retries=0`
- The fixtures cover preserved local history, provider-only history order and provenance, shared
  commits, confirmed divergence, provider-ahead actions, and mobile zero-overflow/touch behavior.
- Fresh desktop and Pixel 5 screenshots were captured, inspected, compressed, and mapped in
  `apps/web/.pr-assets/manifest.json`.
