---
id: "05-cross-fork-e2e"
title: "Prove cross-fork comparison end to end"
status: done
wave: 5
depends_on: ["04-comparison-target-ui-and-docs"]
plan: "plan.md"
spec: "../../specs/platform/requirements/workspace-git-status.md"
---

# Task 05: Cross-fork desktop/mobile regression

Add deterministic browser coverage for the reported stale-fork failure and its manual recovery path. Use real
temporary Git repositories plus the mock provider; do not assert only mocked numeric payloads.

## Red tests first

Create an upstream repository, an outdated fork with the same `main` branch name, and a feature branch whose
single commit changes three files. Associate a mock provider PR whose head repository is the fork and target
repository is upstream. Before production changes, the fixture must reproduce inflated commits/diff from the
fork base.

## Desktop scenario

Add `apps/web/e2e/tests/git/fork-pr-comparison-target.spec.ts`:

- create/start the fork-attached task and wait for its initial Git observation;
- associate through the real task-PR association route with complete provider head/base identity;
- assert target label `upstream/widget:main`;
- assert exactly one local contribution commit and the expected three-file `+/-` scope in Changes/Review;
- assert the task-card totals refresh without backend/session restart;
- reload and prove the same target and totals survive;
- choose fork `main` in **Compare against**, assert the provider target clears, and confirm this action does not
  change the PR base, `origin`, checkout branch, or push remote; and
- cover an unreadable/colliding comparison target with the explicit unavailable state and no inflated totals.

## Mobile scenario

Add `apps/web/e2e/tests/git/mobile-fork-pr-comparison-target.spec.ts` using the mobile project:

- open Changes through the normal mobile control;
- open the existing branch details touch drawer and assert the full target identity is accessible;
- assert the one-commit/three-file scope and unavailable-state recovery copy;
- perform the same-name manual override through touch input;
- verify one Changes scroll owner, touch-sized controls, no clipped drawer content, and no horizontal document
  overflow.

## Fixture and cleanup rules

- Extend shared Git/provider helpers only where both specs reuse them.
- Use the managed E2E backend and API client; do not start a second backend.
- Keep repositories under the test-owned temporary directory and restore/reset shared seed state in cleanup.
- Use causal API/WS waits rather than arbitrary sleeps. Keep screenshots optional and deterministic.

## Acceptance

- The exact public-repository fork topology that produced the bug is covered on desktop and mobile.
- The test fails against branch-only origin resolution and passes only when association, persistence,
  materialization, cache refresh, and UI are all connected.
- Existing PR drift/history and multi-PR E2E suites remain green.

## Likely files

- `apps/web/e2e/tests/git/fork-pr-comparison-target.spec.ts` (new)
- `apps/web/e2e/tests/git/mobile-fork-pr-comparison-target.spec.ts` (new)
- `apps/web/e2e/helpers/git-helper.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/fixtures/*` only if the existing fixture cannot express two remotes
- `apps/backend/internal/backendapp/e2e_reset.go` only for provider identity fixture support not completed in Task 03

## Verification

```bash
cd apps/web && pnpm e2e:run --host --no-build --project chromium -- e2e/tests/git/fork-pr-comparison-target.spec.ts
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome -- e2e/tests/git/mobile-fork-pr-comparison-target.spec.ts
cd apps/web && pnpm e2e:run --host --no-build --project chromium -- e2e/tests/git/git-changes-panel.spec.ts e2e/tests/pr/pr-detection.spec.ts
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome -- e2e/tests/git/mobile-pr-checkout-drift.spec.ts
```

Run `cd apps && pnpm install --frozen-lockfile` first in a fresh worktree.

## Parallelism

`sequential`. Final integration gate after Tasks 01-04.

## Output contract

Record the reproduced before-values, corrected after-values, desktop/mobile command results, cleanup behavior,
and any remaining executor limitation. Mark Task 05 and the plan complete only after both new projects pass.

## Completion record

- The fixture keeps the local fork's `origin/main` at the old commit, advances the disposable upstream,
  and bases `feature/fork` on that upstream commit before adding one three-file contribution commit.
- Corrected results are one local contribution commit, three PR files, and `upstream/widget:main` in both
  desktop and mobile Changes surfaces. The branch-only baseline would include the upstream commit and is
  therefore distinct from the asserted result.
- Green commands:
  - `pnpm e2e:run --host --no-build --project chromium -- e2e/tests/git/fork-pr-comparison-target.spec.ts`
  - `pnpm e2e:run --host --no-build --project mobile-chrome -- e2e/tests/git/mobile-fork-pr-comparison-target.spec.ts`
- The host runner cleaned each disposable repository/backend fixture after the run; no executor limitation
  remains for this regression.
