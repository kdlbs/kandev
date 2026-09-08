---
id: "03-e2e-docs-verification"
title: "PR Details E2E, docs, and verification"
status: done
wave: 3
depends_on: ["02-review-sync-and-opening"]
plan: "plan.md"
spec: "../../specs/ui/requirements/task-layout-profiles.md"
---

# Task 03: PR Details E2E, docs, and verification

Prove layout-owned PR Details behavior through production-build desktop/mobile flows, update public guidance, and remove obsolete global-setting test support.

## Acceptance

- Desktop Layout settings show PR Details in the built-in Default Agent group and can save it in another group; fresh/reset and Plan Mode exit workbenches honor that placement with Agent selected.
- Canonical PR Details remains present with no linked review, follows the active task's provider/key, and is not auto-created after the user removes it.
- Explicit secondary PR/MR tabs join the canonical group, fall back to center when canonical is absent, and focus without relocation when already open.
- Mobile Layout settings can add/move PR Details using existing touch controls and produce no document-level horizontal overflow.
- Mobile/tablet task Review UI remains unchanged.
- Appearance settings and E2E helpers contain no PR placement preference flow.
- Public Sessions and Review docs describe Layouts-based placement and fresh/reset versus existing-layout behavior.

## E2E sequence

1. Update tests for the intended behavior and run the focused files against the current production build to capture expected failures.
2. Finish only testability changes needed by stable `data-testid` selectors or page-object helpers.
3. Run desktop PR/Layout specs with a fresh managed build.
4. Run mobile Layout settings under `mobile-chrome` and assert touch operation plus overflow bounds.
5. Run lint, typecheck, backend tests, public-doc validation, and the focused unit suite before delivery.

## Files likely touched

- `apps/web/e2e/tests/settings/layout-profiles.spec.ts`
- `apps/web/e2e/tests/settings/mobile-layout-profiles.spec.ts`
- add `apps/web/e2e/tests/pr/pr-detail-layout.spec.ts`
- remove `apps/web/e2e/tests/pr/pr-detail-auto-show.spec.ts`
- `apps/web/e2e/tests/pr/pr-detail-manual-open.spec.ts`
- `apps/web/e2e/tests/pr/pr-detail-dedup.spec.ts`
- `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`
- `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`
- `apps/web/e2e/fixtures/test-base.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/pages/session-page.ts`
- `docs/public/sessions-and-review.md`

## Verification

- `cd apps/web && pnpm e2e:run tests/settings/layout-profiles.spec.ts tests/pr/pr-detail-layout.spec.ts tests/pr/pr-detail-manual-open.spec.ts tests/pr/pr-detail-dedup.spec.ts`
- `cd apps/web && pnpm e2e:run tests/settings/mobile-layout-profiles.spec.ts -- --project=mobile-chrome`
- `make -C apps/backend test`
- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web lint`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `rg -n "pr_panel_placement|prPanelPlacement|Pull Request Tabs" apps docs/public`

## Dependencies

Task 02.

## Output contract

Report changed files, exact E2E/unit/lint/typecheck/docs results, failure artifacts or blockers, public-doc impact, and residual risks; set this task to `done` and tick it in `plan.md`.
