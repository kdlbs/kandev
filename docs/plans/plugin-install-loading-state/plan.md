---
spec: docs/specs/plugins/requirements/plugins.md
created: 2026-08-08
status: complete
---

# Implementation Plan: Plugin install loading state

## Overview

The install action already owns a single `installBusy` state for URL and upload
installs, and the dialog already disables its submit button while that state is
true. The regression is presentational: the dialog always renders the idle
Install label and no loading indicator, so a user cannot tell whether the
disabled action is still working. The fix keeps the existing install pipeline
and dialog composition, rendering the shared busy state as an animated spinner,
an installing label, and an accessible busy state.

## Confirmed root cause

`usePluginActions.runInstall` sets `installBusy` before invoking either install
API and clears it in `finally`, but `InstallPluginDialog` only consumes `busy`
for `disabled={...}`. Its primary button always renders `t("plugins:install")`
and has no icon or `aria-busy` state. The existing page test passes 16 tests but
does not hold the install promise pending, so it cannot detect this missing
feedback.

## Frontend

### Install dialog

- `apps/web/components/settings/plugins/install-plugin-dialog.tsx`
  - Reuse the existing `busy` prop for both URL and upload tabs.
  - Render `IconLoader2` with `animate-spin` while busy and keep it hidden when
    idle.
  - Use the existing `plugins:installing` translation while busy, otherwise keep
    `plugins:install`.
  - Expose `aria-busy` while the pipeline is pending and preserve the stable
    tab-specific test IDs and disabled behavior.

No backend, API, store, plugin contract, persistence, or marketplace changes are
needed. The scope is the manual install dialog shown in the reported screenshot;
marketplace actions already expose their own in-progress label and are unchanged.

## Mobile parity contract

- **Desktop outcome / mobile entry:** both viewports submit the same URL/upload
  install through Settings > Plugins; mobile enters through the existing Plugins
  settings route and install dialog.
- **Nearest shipped exemplar:** the existing Radix install dialog and
  `apps/web/e2e/tests/plugins/mobile-plugin-nav.spec.ts` plugin-settings flow.
  They provide the dialog presentation, footer action, and mobile reachability.
- **Hierarchy and primary action:** the dialog remains the focused surface; the
  footer Install action remains the primary action and shows the same spinner and
  installing label on phone and desktop.
- **Presentation / scroll / safe area:** no composition changes; keep the current
  Dialog, its existing viewport behavior, and existing scroll ownership. No new
  fixed control or safe-area handling is introduced.
- **Shared state:** `installBusy`, error handling, and submit callbacks remain
  shared; no mobile-specific business logic is added.
- **Mobile proof:** extend the existing mobile plugin install flow with a held
  install response assertion. That flow already owns the disposable plugin
  fixture and cleanup, so it can prove the same primary action without creating a
  new mobile surface.

## Tests

- **What:** While a URL or upload install promise is pending, the dialog’s primary
  action is disabled, has `aria-busy="true"`, shows an animated spinner, and uses
  the installing label; after rejection the spinner disappears and the existing
  error/retry state remains available.
  **File:** `apps/web/app/settings/plugins/page.test.tsx`.
  **How:** hold a mocked install promise with an explicit resolver, assert the
  rendered button state before resolving/rejecting, then assert the settled state.
- **What:** Existing URL and upload success paths still close the dialog, update
  the store, and load the installed bundle.
  **File:** `apps/web/app/settings/plugins/page.test.tsx`.
  **How:** run the existing focused page suite unchanged alongside the new
  pending-state regression.

## E2E Tests

- **Scenario:** GIVEN an upload in Settings > Plugins, WHEN the install request
  is deliberately held before its response reaches the browser, THEN the
  desktop Install button is disabled and visibly spinning; after a controlled
  failure is released, the action recovers and the inline error remains visible.
  **Files:** `apps/web/e2e/helpers/plugin-install.ts` and
  `apps/web/e2e/tests/plugins/plugins.spec.ts`.
- **Scenario:** The same pending upload state remains reachable and visually
  understandable on the configured Pixel 5 project.
  **File:** `apps/web/e2e/tests/plugins/mobile-plugin-nav.spec.ts`, using the
  shared install-response helper.
- **Focused commands:**
  - `cd apps && pnpm install --frozen-lockfile` (only if this worktree lacks dependencies)
  - `cd apps && pnpm --filter @kandev/web test -- --run app/settings/plugins/page.test.tsx`
  - `cd apps/web && pnpm e2e:run --project chromium e2e/tests/plugins/plugins.spec.ts -- --grep "install.*spinner|spinner.*install"`
  - `cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/plugins/mobile-plugin-nav.spec.ts -- --grep "install.*spinner|spinner.*install"`
  - `cd apps/web && pnpm run typecheck`

## Verification Results

- `make build-backend build-web` — passed; Vite production assets rebuilt.
- `cd apps && pnpm --filter @kandev/web test -- --run app/settings/plugins/page.test.tsx` — passed, 17 tests.
- `cd apps/web && pnpm e2e:run --host --no-build e2e/tests/plugins/plugins.spec.ts -- --grep "install.*spinner|spinner.*install"` — passed, 1 Chromium test.
- `cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/plugins/mobile-plugin-nav.spec.ts -- --grep "install.*spinner|spinner.*install"` — passed, 1 Pixel 5 test.
- `cd apps/web && pnpm run typecheck` — passed.
- Targeted ESLint for the changed component, page test, and desktop/mobile plugin specs — passed with zero warnings.
- `git diff --check` — passed.
- Interactive dev-browser validation confirmed `disabled`, `aria-busy="true"`, the animated spinner, and `Installing…` before the held response was released.
- Managed screenshot capture passed and generated two fresh, compressed, secret-free assets in `.pr-assets/manifest.json` for desktop and Pixel 5 viewports; the disposable capture spec was removed afterward.
- CodeRabbit's valid duplication finding was fixed by moving the held-install
  route helper into `apps/web/e2e/helpers/plugin-install.ts`; both focused E2E
  regressions passed again afterward.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-install-loading-state](task-01-install-loading-state.md)

The dialog, page regression test, and focused desktop/mobile E2E evidence share
the same install state and should be implemented and verified as one vertical
slice. The marketplace’s separate install/update buttons are not changed by this
fix.
