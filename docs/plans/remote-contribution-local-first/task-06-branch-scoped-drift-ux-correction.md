---
id: "06-branch-scoped-drift-ux-correction"
title: "Branch-scoped drift UX correction"
status: completed
wave: 5
depends_on: ["05-desktop-mobile-drift-e2e"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 06: Branch-Scoped Drift UX Correction

Prevent historical pull requests from driving the live Changes relation and remove the duplicated
drift warning from the panel body.

## Confirmed Root Cause

`useRemoteContributionRelation` reads `useReviewPRSelection`, whose default is the first associated
pull request. A merged PR can therefore supply provider commits for a different branch while the Git
status comes from the current checkout, producing a false `diverged` relation. The Changes PR-file
path also reads every task PR, so historical files can leak into the live panel.

## Acceptance

1. Changes selects pull requests only when repository identity and normalized head branch match a
   live Git status. An open current-branch PR wins over a terminal sibling on the same branch.
2. Review selection remains independent and can continue to show historical pull requests.
3. Divergence appears as the toolbar warning icon and menu only. The body has no warning banner.
4. The provider disclosure is labeled **PR #<number> version** on desktop and mobile.
5. Desktop info icons open immediate tooltips without intercepting adjacent menu actions.

## TDD And Verification

1. Add a failing hook regression for a merged first PR and open second PR on the live branch.
2. Add failing desktop and mobile assertions for the corrected copy and warning composition.
3. Implement the branch-scoped selector and share it with provider commits and PR files.
4. Run focused Vitest, typecheck, i18n checks, and the two drift Playwright scenarios with retries
   disabled.

## Files Likely Touched

- `apps/web/hooks/domains/session/use-remote-contribution-relation.ts`
- `apps/web/hooks/domains/session/use-remote-contribution-relation.test.tsx`
- `apps/web/hooks/domains/github/use-active-task-pr-files.ts`
- `apps/web/components/task/changes-panel-data.tsx`
- `apps/web/components/task/changes-panel-body.tsx`
- `apps/web/components/task/remote-contribution-action-items.tsx`
- `apps/web/components/task/remote-contribution-header-actions.tsx`
- `apps/web/components/vcs-*.tsx`
- `apps/web/components/task/mobile/*.tsx`
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/git/git-changes-panel.spec.ts`
- `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts`

## Risks

- Do not let a Review preference control live checkout state.
- Preserve the legacy empty repository key for single-repository sessions.
- Keep tooltips non-interactive so their portal cannot block dropdown selection.

## Results

Completed 2026-08-12.

- Added a branch-scoped selector that matches pull requests to live Git status by repository identity
  and normalized branch. An open pull request wins over a merged or closed pull request on the same
  branch, and Review selection cannot override a different checked-out branch.
- Scoped provider commits, files, links, and drift state to the selected live pull request. Added the
  two-PR regression that keeps the merged pull request in Review while Changes follows the open pull
  request for the checkout.
- Removed the repeated yellow body banner. Kept the toolbar warning menu, changed the provider label to
  **PR #<number> version**, and replaced native titles with immediate shared tooltips.
- Focused verification passed:
  `pnpm --filter @kandev/web test -- hooks/domains/session/branch-scoped-task-pr.test.ts hooks/domains/session/use-remote-contribution-relation.test.tsx hooks/domains/session/remote-contribution-relation.test.ts components/task/remote-contribution-header-actions.test.tsx components/task/use-remote-contribution-resolution.test.tsx components/task/remote-contribution-resolution-dialog.test.tsx components/task/mobile/session-mobile-top-bar-git-controls.test.tsx components/vcs-split-button.test.ts components/vcs-multi-repo-menu.test.ts components/task/changes-panel-pr-files.test.tsx components/task/changes-panel-remote.test.ts components/task/changes-panel-header.test.tsx`
  (12 files, 66 tests).
- `pnpm --filter @kandev/web lint`, `pnpm run typecheck`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed.
- Desktop verification passed:
  `pnpm e2e:run --no-build --project chromium tests/git/git-changes-panel.spec.ts -- --retries=0`
  (21 tests). The focused two-PR regression and rewritten-history scenario also passed independently.
- Pixel 5 verification passed:
  `pnpm e2e:run --no-build --project mobile-chrome tests/git/mobile-pr-checkout-drift.spec.ts -- --retries=0`
  (1 test).
