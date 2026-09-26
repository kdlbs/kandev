---
id: "05-prove-submodule-review"
title: "Prove responsive submodule review"
status: done
wave: 3
depends_on: ["03-present-submodule-review-hierarchy", "04-order-nested-git-mutations"]
plan: "plan.md"
spec: "../../specs/ui/requirements/submodule-review.md"
---

# Task 05: Prove responsive submodule review

## Acceptance

- A real disposable parent/direct/nested submodule graph produces parent and child textual diffs in desktop Review with marked nested boundaries, isolated duplicate paths, and no duplicate gitlink row.
- The user-facing commit-all path commits children before parents and the resulting parent tree records each new child commit.
- Mobile Review exposes submodule scope and file diff through its existing touch path, stays viewport-contained, and has no document-level horizontal overflow.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/review/submodule-review.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/review/mobile-submodule-review.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/review/submodule-review-helpers.ts` (new)
- `apps/web/e2e/tests/review/submodule-review.spec.ts` (new)
- `apps/web/e2e/tests/review/mobile-submodule-review.spec.ts` (new)
- Existing Review page object only when a reusable user action is missing

## Dependencies

Tasks 03 and 04.

## Parallelism

`sequential` — this is the integrated proof over the backend and both frontend tasks.

## Inputs

- Every spec scenario involving rendered Review, duplicate paths, commit ordering, and phone parity.
- `/e2e` managed runner, production-build, disposable-fixture, and UI-assertion guidance.
- `/mobile-parity` mobile-chrome naming, geometry, touch-target, and overflow requirements.

## Output contract

Report desktop/mobile test counts, discovered Playwright projects, commit/tree assertions, screenshots or other rendered evidence, disposable-repository cleanup, changed files, blockers/risks, and synchronized task/plan status.

## Results

Added disposable parent/outer/inner repository fixtures using the direct local executor, recursive initialization, UI assertions for duplicate `README.md` identities and gitlink suppression, deepest-first commit/tree checks, and mobile geometry/overflow assertions. Cleanup removes the per-test source tree in `finally`.

Verification:

- `pnpm run build:vite` — passed.
- `pnpm run e2e:run --host --no-build --project chromium tests/review/submodule-review.spec.ts` — 1 passed.
- `pnpm run e2e:run --host --no-build --project mobile-chrome tests/review/mobile-submodule-review.spec.ts` — 1 passed.
