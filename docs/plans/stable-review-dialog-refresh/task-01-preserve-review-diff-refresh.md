---
id: "01-preserve-review-diff-refresh"
title: "Preserve Review diff during refresh"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/multi-branch.md"
---

# Task 01: Preserve Review diff during refresh

## Acceptance

- Advancing `last_synced_at` for the same workspace and PR triggers one replacement request without removing previously resolved files, reporting blocking loading, or remounting the Review diff while pending.
- The current response atomically replaces retained files; a failed background refresh retains them, while an initial failure still exposes the existing error/retry state.
- Workspace or PR changes mask incompatible files immediately, and late superseded responses cannot overwrite the current selection.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run hooks/domains/github/use-pr-diff.test.ts && pnpm --filter @kandev/web lint && pnpm --filter @kandev/web typecheck
```

```bash
cd apps/web && pnpm e2e:run tests/review/review-multi-pr.spec.ts
```

```bash
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/review/mobile-review-multi-pr.spec.ts
```

## Files likely touched

- `apps/web/hooks/domains/github/use-pr-diff.ts`
- `apps/web/hooks/domains/github/use-pr-diff.test.ts`
- `docs/specs/tasks/requirements/multi-branch.md`
- `docs/plans/stable-review-dialog-refresh/plan.md`
- this task file

## Dependencies

None.

## Parallelism

Sequential. Hook behavior and regression coverage share the same state contract.

## Inputs

- Multi-branch spec Review refresh scenarios.
- Root cause and identity split in `plan.md`.
- Existing stable request/display identity pattern in `use-active-task-pr-files.ts`.
- Existing different-PR late-response regression in `use-pr-diff.test.ts`.

## TDD sequence

1. Re-run the existing same-PR refresh regression and confirm the expected missing-file assertion failure.
2. Add controlled atomic-replacement and initial/background-failure cases.
3. Implement the smallest logical-identity/request-key split in `use-pr-diff.ts`.
4. Run the focused unit suite, lint, typecheck, then existing desktop and mobile Review E2E specs.

## Output contract

Report RED/GREEN evidence, logical and timestamped identity behavior, failure retention, exact commands/results, E2E cleanup, files changed, residual risks, and synchronize this task plus `plan.md` in the same primary conversation.

## Results

- RED: the original same-PR timestamp regression failed because the pending refresh returned no files; expanded success and failure coverage produced 3 failures among 8 focused tests.
- QA RED: retaining files while returning `loading: true` still failed the mounted-surface contract because `ReviewPRDiffBoundary` uses that value to replace a PR-only diff with its blocking loader. The focused suite failed 1 of 8 tests until background refresh became non-blocking.
- GREEN: `cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/github/use-pr-diff.test.ts --reporter=dot` passed 8 tests covering pending refresh, atomic replacement, background and initial failure, superseded timestamp responses, and different-PR isolation.
- Adjacent scope: `cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/github/use-pr-workspace-scope.test.ts --reporter=dot` passed 6 tests.
- Static checks: `cd apps && pnpm --filter @kandev/web lint` and `cd apps && pnpm --filter @kandev/web typecheck` passed.
- Fresh production E2E: desktop Review passed 1 test; phone and coarse-pointer tablet Review passed 2 tests with the task-file commands above.
- `usePRDiff` now separates logical display identity from timestamped request freshness, retains only a resolved diff for the same workspace/PR, treats that refresh as non-blocking, atomically replaces current responses, retains content on background failure, and ignores superseded responses.
- No layout, touch, navigation, copy, backend, security/trust-boundary, or external-state changes were made. Managed E2E cleaned up its isolated processes.
