---
id: multi-pr-review-04
title: Prove multi-PR Review across viewports
status: done
wave: 3
depends_on: [multi-pr-review-02, multi-pr-review-03]
plan: ./plan.md
---

## Acceptance

- Desktop E2E switches two same-repository PRs and proves no stale or wrong-path diff.
- Mobile E2E proves bottom-menu selection, 44px targets, viewport containment, and no document overflow.
- Tablet smoke opens Review; full format, typecheck, test, and lint pass.

## Files

- `apps/web/e2e/tests/review/review-multi-pr.spec.ts`
- `apps/web/e2e/tests/review/mobile-review-multi-pr.spec.ts`
- E2E page/helper files only when reuse warrants it
- Mock controller only if fixture realism requires it

## Inputs

- Tasks 02 and 03
- Existing review-file-status and pr-multi-popover specs

## Verification

- `cd apps/web && pnpm e2e:run tests/review/review-multi-pr.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/review/mobile-review-multi-pr.spec.ts`
- `make fmt`
- `make typecheck test lint`

## Output contract

Report E2E/visual evidence, full verification, remaining risks, and all files; mark status done only after all required checks pass or record exact external blocker.
