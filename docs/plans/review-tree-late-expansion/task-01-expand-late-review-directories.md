---
id: "01-expand-late-review-directories"
title: "Expand late Review directories"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/submodule-review.md"
---

# Task 01: Expand late Review directories

## Acceptance

- A Review tree that receives a nested submodule file after its parent scope automatically exposes the newly introduced intermediate directory and nested boundary.
- A directory already observed and manually collapsed remains collapsed when later files arrive; the fix does not reset the reviewer's expansion choices.
- The existing desktop nested-submodule E2E passes without relying on a retry-only success.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/review/review-file-tree.test.tsx
cd apps/web && pnpm e2e:run tests/review/submodule-review.spec.ts -- --grep "shows nested scopes and commits child gitlinks through the UI" --workers=1 --retries=0
cd apps/web && pnpm e2e:run --project mobile-chrome tests/review/mobile-submodule-review.spec.ts
```

The managed E2E runner rebuilds the production backend and Vite assets before the desktop check. The focused unit test is the RED test and must fail before the source change, then pass after it.

## Files likely touched

- `apps/web/components/review/review-file-tree.tsx`
- `apps/web/components/review/review-file-tree.test.tsx`

## Dependencies

None.

## Parallelism

`sequential` — the regression test and the component change share the same files and must follow Red-Green-Refactor.

## Inputs

- `docs/specs/ui/requirements/submodule-review.md`, especially the nested-boundary and late-source scenarios.
- `docs/plans/review-tree-late-expansion/plan.md`.
- Existing first-seen directory reconciliation in `apps/web/components/task/changes-panel-tree.tsx`.
- `/tdd`, `/e2e`, and `/mobile-parity` guidance.

## Output contract

Report the RED and GREEN unit-test results, exact E2E commands and counts, files changed, cleanup evidence, residual risks, and synchronized task/plan status.

## Results

Completed 2026-08-15.

- RED: the new rerender regression failed before the implementation (`17 passed, 1 failed`).
- GREEN: the focused ReviewFileTree suite passed (`18 passed`).
- `cd apps/web && pnpm e2e:run tests/review/submodule-review.spec.ts -- --grep "shows nested scopes and commits child gitlinks through the UI" --workers=1 --retries=0` passed (`1 passed`).
- `cd apps/web && pnpm e2e:run --no-build tests/review/submodule-review.spec.ts -- --grep "shows nested scopes and commits child gitlinks through the UI" --workers=1 --retries=0 --repeat-each=4` passed (`4 passed`).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/review/mobile-submodule-review.spec.ts` passed (`1 passed`).
- `cd apps && pnpm --filter @kandev/web typecheck` reported no errors.
- `cd apps && pnpm --filter @kandev/web lint` passed with zero warnings.
- Changed files: `apps/web/components/review/review-file-tree.tsx`, `apps/web/components/review/review-file-tree.test.tsx`, `docs/specs/ui/requirements/submodule-review.md`, `docs/plans/review-tree-late-expansion/plan.md`, and this task file.
- Residual risk: the Review tree still rebuilds and walks its small directory model when `files` changes; no shared `useTree` behavior or mobile tree surface changed.
- The disposable screenshot capture spec was removed after capture. The synthetic desktop screenshot manifest was non-empty, every manifest entry mapped to an existing file, and the PNG was compressed for PR publication.
- Synchronization: this task is `done`, the parent plan is `implemented` with Task 01 checked, and the linked submodule Review spec records the late-source expansion contract.
