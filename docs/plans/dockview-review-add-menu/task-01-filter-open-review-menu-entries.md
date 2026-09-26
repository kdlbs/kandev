---
id: "01-filter-open-review-menu-entries"
title: "Filter open review menu entries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-layout-profiles.md"
---

# Task 01: Filter Open Review Menu Entries

## Acceptance

- Every task Dockview `+` menu and empty-group watermark omits a GitHub,
  GitLab, or registered-provider review that already has an exact canonical or
  keyed panel anywhere in the live layout.
- Other linked reviews remain available. GitHub chooses no row, one inline row,
  or a submenu from the filtered count, not the task's total linked PR count.
- Closing an exact review panel makes its row available on the next menu open;
  a mixed-review canonical selector does not hide individual reviews.
- Existing review actions still focus matching tabs in place outside the add
  menu and retain current placement for newly opened keyed tabs.
- Desktop E2E proves both layout-editor placement in another split and the
  multi-PR filtered-menu flow.
- Public session/review guidance explains the missing-only `+` menu and the
  explicit path for moving an existing review tab.

## TDD Sequence

1. Add the focused unit cases for canonical/keyed identities, filtered PR
   counts, provider parity, and mixed-review behavior.
2. Run the unit file against unchanged production code and record the expected
   failures.
3. Add the minimal live-API predicate and filter the three review menu paths.
4. Rerun the unit suite and existing review panel-action regression suite.
5. Update the two existing Chromium E2E scenarios, run them against a fresh
   production build, and record exact results.

## Verification

Bootstrap only if this worktree's dependencies are absent:

```bash
cd apps && pnpm install --frozen-lockfile
```

From the repository root, run:

```bash
cd apps && pnpm --filter @kandev/web exec vitest run components/task/dockview-add-panel-items.test.tsx lib/state/dockview-panel-actions-extra.test.ts
cd apps/web && pnpm exec eslint components/task/dockview-add-panel-items.tsx components/task/dockview-add-panel-items.test.tsx && pnpm run typecheck
cd apps/web && pnpm e2e:run tests/settings/layout-profiles.spec.ts tests/pr/pr-multi-popover.spec.ts -- --grep 'moves PR Details|selecting a different PR'
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Confirm Playwright discovers both intended Chromium tests before treating the
focused E2E command as evidence.

## Files Likely Touched

- `apps/web/components/task/dockview-add-panel-items.tsx`
- `apps/web/components/task/dockview-add-panel-items.test.tsx`
- `apps/web/e2e/tests/settings/layout-profiles.spec.ts`
- `apps/web/e2e/tests/pr/pr-multi-popover.spec.ts`
- `docs/public/sessions-and-review.md`
- `docs/specs/ui/requirements/task-layout-profiles.md`
- `docs/specs/ui/requirements/add-panel-pr-submenu.md`
- `docs/plans/dockview-review-add-menu/plan.md`
- `docs/plans/dockview-review-add-menu/task-01-filter-open-review-menu-entries.md`

## Dependencies

None.

## Parallelism

`sequential`. Unit behavior, production filtering, and E2E expectations share
one RED-GREEN cycle and overlapping files.

## Inputs

- `docs/specs/ui/requirements/task-layout-profiles.md`, especially review placement and the
  missing-only add-menu scenario.
- `docs/specs/ui/requirements/add-panel-pr-submenu.md`, especially filtered PR count and
  inline/submenu scenarios.
- `apps/web/components/task/dockview-add-panel-items.tsx` for all three review
  menu paths.
- `apps/web/lib/state/dockview-panel-actions.ts` and
  `apps/web/lib/state/dockview-review-panel-id.ts` for canonical/keyed identity
  and focus-in-place behavior.
- `apps/web/components/task/dockview-review-panel-sync.ts` for canonical
  provider-neutral parameters and mixed-review state.
- Existing E2E setup in `layout-profiles.spec.ts` and
  `pr-multi-popover.spec.ts`.

## Output Contract

Report the RED failures, GREEN command results and test counts, files changed,
any identity compatibility risk, and cleanup evidence. Mark this task `done`,
replace `## Results` below, check the task in `plan.md`, and synchronize the
plan's `## Verification Results` in the same primary conversation.

## Results

RED results:

- `dockview-add-panel-items.test.tsx` ran 18 tests: the five new canonical,
  keyed, filtered-count, GitLab, and registered-provider assertions failed as
  expected; 13 existing and mixed-selector guard assertions passed.

GREEN results:

- `pnpm --filter @kandev/web exec vitest run
  components/task/dockview-add-panel-items.test.tsx
  lib/state/dockview-panel-actions-extra.test.ts` passed 35 tests across 2
  files.
- Targeted ESLint completed without diagnostics and `pnpm run typecheck`
  passed.
- `pnpm exec playwright test --list ... --grep ...` discovered exactly the 2
  intended Chromium tests. The managed production-build run passed the
  multi-PR scenario; its layout scenario timed out on a new Agent-text test
  locator before reaching a product assertion. Replacing that locator with
  the repository's stable session-tab group selector and rerunning both tests
  in a fresh disposable backend against the unchanged production build passed
  2 tests in 22.0 seconds.
- `pnpm run i18n:check` passed with advisory pre-existing locale parity output;
  `pnpm run i18n:ratchet` reported 0 added plus 1 modified production file
  clean.
- Public-doc validation passed 61 tests and validated all 41 published pages.
- `git diff --check` passed.

The live-menu predicate recognizes exact GitHub and GitLab legacy/current keys,
registered-provider canonical identities, and keyed provider panels. A
provider-neutral canonical selector remains non-matching, and remounting the
menu after a panel closes restores the row. Existing topbar review actions and
panel placement code were not changed.

Public docs updated: `docs/public/sessions-and-review.md` remains a how-to guide
and now explains that `+` lists only missing review panels and that an open tab
must be dragged to move it between splits. No mobile E2E was added because the
phone/tablet task composition does not render Dockview's add-panel menu; its
dedicated Review navigation is unchanged.
