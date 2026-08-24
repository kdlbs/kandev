---
id: "01-install-loading-state"
title: "Show install loading state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/requirements/plugins.md"
---

# Task 01: Show install loading state

## Acceptance

- While a URL or upload install is pending, the manual install dialog’s primary
  action is disabled, marked busy, shows an animated spinner, and uses the
  existing installing translation.
- When the install settles successfully, the existing dialog-close, plugin-store,
  and bundle-load behavior is unchanged; when it fails, the spinner stops and the
  existing inline error/retry state remains available.
- The same primary-action behavior remains reachable on the configured mobile
  plugin settings flow; no mobile layout or touch target regression is introduced.

## Verification

- `cd apps && pnpm install --frozen-lockfile` if workspace dependencies are absent.
- `cd apps && pnpm --filter @kandev/web test -- --run app/settings/plugins/page.test.tsx`
- `cd apps/web && pnpm e2e:run --project chromium e2e/tests/plugins/plugins.spec.ts -- --grep "install.*spinner|spinner.*install"`
- `cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/plugins/mobile-plugin-nav.spec.ts -- --grep "install.*spinner|spinner.*install"`
- `cd apps/web && pnpm run typecheck`

## Files likely touched

- `apps/web/components/settings/plugins/install-plugin-dialog.tsx`
- `apps/web/app/settings/plugins/page.test.tsx`
- `apps/web/e2e/tests/plugins/plugins.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-plugin-nav.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. The UI and tests share the same transient install-state contract.

## Inputs

- `docs/specs/plugins/requirements/plugins.md` install pipeline and install-dialog scenario.
- `docs/plans/plugin-install-loading-state/plan.md` root-cause evidence and
  mobile parity contract.
- Existing `installBusy` wiring in
  `apps/web/components/settings/plugins/use-plugin-actions.ts`.
- Existing spinner pattern in `apps/web/components/settings/install-agent-card.tsx`.

## Output contract

Summarize the changed files, the pending-state regression coverage, exact test
commands and results, any E2E fixture limitations, and synchronized task/plan
status updates. Keep the plugin API and install pipeline unchanged.

## Results

- Updated `install-plugin-dialog.tsx` to render the existing installing
  translation with an animated `IconLoader2` and `aria-busy` while the shared
  install pipeline is pending; disabled behavior and test IDs remain unchanged.
- Added the URL pending-state component regression and desktop/mobile held-route
  E2E coverage. The shared held-install helper in
  `apps/web/e2e/helpers/plugin-install.ts` is used by both E2E specs. The tests
  verify recovery to the idle Install action and the existing inline error state
  after a controlled failure.
- Preserved the existing successful URL/upload install pipeline, store updates,
  bundle loading, dialog close behavior, and marketplace install controls.
- Verification passed: production build, focused page suite (17 tests),
  Chromium E2E (1 test), Pixel 5 E2E (1 test), typecheck, targeted ESLint,
  `git diff --check`, interactive browser validation, and managed desktop/mobile
  screenshot capture. After PR review, the shared-helper fix was rechecked with
  the focused Chromium and Pixel 5 E2E tests plus targeted ESLint.
