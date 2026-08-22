---
id: "01-separate-review-sticky-headers"
title: "Separate Review sticky headers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/submodule-review.md"
---

# Task 01: Separate Review sticky headers

## Intent

Keep repository-scope and current-file headers simultaneously visible and interactive in Review without changing repository grouping or responsive composition.

## Acceptance

- With workspace-root and submodule changes, the sticky **Other changes** header ends at or above the current file header on desktop and phone.
- The current file disclosure control remains the topmost element at its center and can still be activated after the headers stick.
- A Review diff with no rendered repository header keeps the existing zero sticky inset and gains no empty header lane.
- Programmatic file navigation aligns grouped file sections below the repository header rather than hiding the selected header or first diff content.

## Files likely touched

- `apps/web/components/review/review-diff-list-groups.tsx`
- `apps/web/components/review/review-diff-list.tsx`
- `apps/web/components/review/review-diff-header.tsx`
- `apps/web/e2e/tests/review/submodule-review-helpers.ts`
- `apps/web/e2e/tests/review/submodule-review.spec.ts`
- `apps/web/e2e/tests/review/mobile-submodule-review.spec.ts`
- `docs/plans/review-sticky-header-clearance/plan.md`
- `docs/plans/review-sticky-header-clearance/task-01-separate-review-sticky-headers.md`

## Dependencies

None.

## Parallelism

`sequential`. Regression assertions and source changes share one sticky-layout contract and overlapping files.

## Inputs

- `docs/specs/ui/submodule-review.md`, especially the mixed root/submodule sticky-header requirement and scenario.
- `docs/plans/review-sticky-header-clearance/plan.md`, including **Root cause**, **Sticky repository and file lanes**, and **Mobile design contract**.
- `apps/web/components/review/review-diff-list.tsx`: `showRepoHeaders`, `FileDiffSection`, and programmatic `scrollIntoView` behavior.
- `/tdd`, `/e2e`, and `/mobile-parity` guidance. Use strict Red-Green-Refactor; no production change before both browser regressions fail for the expected overlap.

## TDD sequence

1. **RED:** add a shared nested-submodule Review assertion that scrolls the root file header to the top, requires the **Other changes** header to end above the file header, and requires `document.elementFromPoint()` at the disclosure center to resolve to the control. Invoke it from desktop and mobile specs. Run both with `--retries=0`; record failures showing intersecting boxes and the covering repository header owning the hit point.
2. **GREEN:** give the repository header a stable height, pass the grouped-header state to each file section, offset grouped file headers by that height, and give grouped sections matching scroll margin. Do not alter ungrouped headers.
3. **REFACTOR:** keep one shared geometry assertion and one shared spacing value per rendered relationship. Remove duplication without introducing a general sticky-header abstraction.
4. Rerun both E2E scenarios after the last source edit against the rebuilt production bundle.

## Verification

Bootstrap once if this worktree lacks dependencies:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run desktop and phone regressions in order. The first command rebuilds current production assets; the second reuses those unchanged assets:

```bash
cd apps/web && pnpm e2e:run tests/review/submodule-review.spec.ts -- --grep "shows nested scopes and commits child gitlinks through the UI" --workers=1 --retries=0
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/review/mobile-submodule-review.spec.ts -- --grep "keeps nested scope and diff context touch-reachable" --workers=1 --retries=0
```

Run focused frontend checks:

```bash
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web exec eslint components/review/review-diff-list-groups.tsx components/review/review-diff-list.tsx components/review/review-diff-header.tsx e2e/tests/review/submodule-review-helpers.ts e2e/tests/review/submodule-review.spec.ts e2e/tests/review/mobile-submodule-review.spec.ts
cd apps && pnpm exec prettier --check web/components/review/review-diff-list-groups.tsx web/components/review/review-diff-list.tsx web/components/review/review-diff-header.tsx web/e2e/tests/review/submodule-review-helpers.ts web/e2e/tests/review/submodule-review.spec.ts web/e2e/tests/review/mobile-submodule-review.spec.ts ../docs/specs/ui/submodule-review.md ../docs/plans/review-sticky-header-clearance/plan.md ../docs/plans/review-sticky-header-clearance/task-01-separate-review-sticky-headers.md
git diff --check
```

## Output contract

Report the RED failure messages for desktop and phone, final command outcomes and test counts, exact changed files, rendered geometry deltas, generated artifacts and cleanup evidence, residual risks, and synchronized task/plan status. State that no backend, API, persistence, localization, or external side effect changed.

## Results

- TDD RED reproduced the overlap in both focused browser scenarios. Desktop and
  `mobile-chrome` each reported `headersSeparated: false` and
  `disclosureHit: false` with retries disabled.
- GREEN gives `RepoGroupHeader` a fixed `h-8` lane. `ReviewDiffList` opts grouped
  sections into matching `scroll-mt-8`, and `ReviewDiffHeader` uses `top-8` only
  when that repository lane exists; direct and ungrouped callers default to
  `top-0`.
- The shared E2E helper now scrolls the file section as production navigation
  does, compares repository/file bounding boxes, verifies the disclosure owns
  its center hit point, and activates it with mouse or touch. Final desktop and
  phone runs each passed one focused scenario with `--retries=0`.
- Geometry delta: repository header 30px to 32px; grouped file sticky inset 0px
  to 32px; nominal overlap 30px to 0px. Browser separation and hit-target states
  changed false to true on desktop and phone.
- Changed code/tests:
  `review-diff-list-groups.tsx`, `review-diff-list.tsx`,
  `review-diff-header.tsx`, `submodule-review-helpers.ts`,
  `submodule-review.spec.ts`, and `mobile-submodule-review.spec.ts`. Durable
  records changed in the owning spec, plan, and this task file.
- Verification: header unit suite 8/8; web typecheck, scoped ESLint, scoped
  Prettier, and `git diff --check` all passed. A single mobile setup attempt hit
  a transient disposable-repository `index.lock`; a clean zero-retry run passed.
- Managed E2E cleanup removed disposable repositories; no generated artifact is
  tracked. Residual risk is limited to keeping the three matching Tailwind
  `8` spacing classes synchronized if this geometry changes later.
- No backend, API, persistence, localization, external-service, or other
  external side effect changed.
