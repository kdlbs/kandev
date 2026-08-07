---
id: "02-e2e-and-public-docs"
title: "E2E and public documentation"
status: completed
wave: 2
depends_on: ["01-conditional-panel-behavior"]
plan: "plan.md"
spec: "../../specs/ui/task-layout-profiles.md"
---

# Task 02: E2E and public documentation

Prove the conditional tab lifecycle in production-build desktop flows, preserve touch access to layout customization, and align the public explanation.

## Acceptance

- Desktop E2E proves a fresh review-less Default task has no PR Details tab, linking a PR adds the tab without focus theft, and closing it suppresses automatic re-creation for the session.
- Desktop and mobile Layout settings E2E add PR Details to the built-in Default through existing controls and prove configured placement persists; desktop additionally proves the saved tab is hidden before association and appears in that group after linking, while mobile retains touch reachability and no horizontal overflow.
- Public Sessions and Review documentation describes association-gated visibility, explicit layout placement, and same-session dismissal accurately.

## TDD and E2E sequence

1. Update focused desktop E2E expectations and run them against the current production build to capture the review-less default failure.
2. Update Layout settings flows so they add PR Details before moving/saving it; retain stable selectors and current page objects.
3. After Task 01 is green, run desktop E2E through the managed runner with a fresh build.
4. Run the updated mobile Layout settings spec under `mobile-chrome`.
5. Update and validate public docs, then record all results and cleanup evidence.

## Files likely touched

- `apps/web/e2e/tests/task/task-default-layout.spec.ts`
- `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`
- `apps/web/e2e/tests/settings/layout-profiles.spec.ts`
- `apps/web/e2e/tests/settings/mobile-layout-profiles.spec.ts`
- `apps/web/e2e/pages/layout-settings-page.ts` only if an existing add-panel action lacks a stable helper
- `docs/public/sessions-and-review.md`

## Verification

- `cd apps && pnpm install --frozen-lockfile`
- `cd apps/web && pnpm e2e:run tests/task/task-default-layout.spec.ts tests/pr/pr-detail-layout.spec.ts tests/settings/layout-profiles.spec.ts`
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-layout-profiles.spec.ts`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `git diff --check`

## Dependencies

Task 01.

## Parallelism

Sequential. These tests and docs validate the conditional behavior implemented by Task 01.

## Inputs

- `docs/specs/ui/task-layout-profiles.md` Scenarios and Out of scope
- `docs/plans/conditional-pr-details-tab/plan.md` E2E Tests, Mobile design contract, and Public documentation
- Existing production-build patterns in `pr-detail-layout.spec.ts`, `layout-profiles.spec.ts`, and `mobile-layout-profiles.spec.ts`

## Output contract

Report exact E2E discovery/count/results, failure artifacts, docs validation, mobile evidence, cleanup, blockers, residual risks, and task/plan status updates in the primary conversation.

## Results

Completed.

- Chromium E2E passed for task default layout (`2` tests), PR Details lifecycle (`3` tests), and Layouts settings (`4` tests). The corrected saved-right-group flow independently passed its unlinked-then-linked transition (`1` test).
- Mobile Chromium E2E passed for touch Layouts editing (`2` tests).
- `node --test scripts/validate-public-docs.test.mjs` passed (`58` tests).
- `node scripts/validate-public-docs.mjs` passed (`41` published docs pages).
- `git diff --check` passed.
