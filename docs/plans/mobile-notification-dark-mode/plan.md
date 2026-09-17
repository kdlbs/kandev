---
created: 2026-09-09
status: done
requirements:
  - REQ-UI-TOAST-THEME-001
system_design:
  - ../../specs/ui/system-design/toast-theme.md
legacy_specs: []
---

# Implementation Plan: Mobile Notification Dark Mode

## Overview

Synchronize the shared Sonner wrapper with the application's initial resolved
theme so plugin success notifications use dark colors after a dark page load.
One sequential work order owns the regression, correction, and focused
desktop/mobile verification.

## Confirmed root cause

`Toaster` reads the document class during its initial render. On a cold page
load, the HTML shell has no theme class, so that read yields light.
`AppThemeProvider` then applies dark in its layout effect. The Toaster's
passive effect subscribes afterward and does not read the current value again.
Its state stays light until a later root-class mutation.

The app shell enables `richColors`. Sonner selects those semantic palettes
using `data-sonner-theme`, so the wrapper's application `--normal-*` CSS
variables do not correct success colors. The same shared defect can affect
desktop and the authentication surface.

## Scope

### In scope

- Document [REQ-UI-TOAST-THEME-001](../../specs/ui/requirements/toast-theme.md),
  which fills a missing reusable UI contract.
- Reconcile the document theme when the shared observer subscribes.
- Cover dark startup, explicit preference precedence, and later theme changes.
- Prove a real plugin success notification renders correctly on mobile and
  desktop.

### Out of scope

- Plugin lifecycle, operating-system notifications, new palettes, and toast
  layout changes.
- Replacing the existing toast APIs or changing theme persistence.

## Technical approach

In `apps/packages/ui/src/sonner.tsx`, preserve the root-class observer and
disconnect behavior, but synchronize immediately after subscribing. Reuse the
same document-theme reader for initial and subsequent synchronization.
Preserve caller-provided props. Keep the theme provider's layout effect;
terminal initialization already depends on that ordering.

## Tests

Add `apps/web/components/theme/sonner-theme.test.tsx` with the real
`AppThemeProvider`, shared Toaster, and a real notification. Cover:

- The four cases in `uses $expected at cold load with theme=$theme and
systemDark=$systemDark` cover AC .1 and .2.
- `preserves an existing notification through preview and discard` and
  `updates existing and subsequent notifications when the system theme changes`
  cover AC .3, including notification identity, content, and actions.
- Observer cleanup and explicit wrapper `theme` prop compatibility.

Retain the existing `components/theme/app-theme.test.tsx` checks, especially
the layout-effect ordering regression.

## E2E tests

Add `tests/settings/toast-theme.spec.ts` for Chromium and
`tests/settings/mobile-toast-theme.spec.ts` for mobile Chrome. Share scenario
helpers in `e2e/helpers/toast-theme.ts` and reuse the packaged plugin install
helpers in `tests/plugins/plugin-test-helpers.ts`.

On a fresh navigation, set saved dark against a light OS, system against a
dark OS, and saved light against a dark OS. Install the isolated fixture via
Settings > Plugins, assert the success notification, and compare its rendered
background, text, and border to the selected Sonner palette. Do not inject the
document's theme class or toggle the theme before the startup assertion.

The mobile spec uses the configured Pixel 5 project, touch activation, a
viewport-contained notification, and a screenshot for visual inspection.
Run the existing mobile theme-toggle spec as preservation coverage. Restore
fixture state with the existing uninstall helper in `afterEach`.

## Work orders

- [x] [Task 01: Synchronize toast theme at mount](task-01-synchronize-toast-theme.md)

## Verification results

- Workspace dependencies installed with `pnpm install --frozen-lockfile`.
- Temporary component reproduction rendered the application root as dark and
  Sonner as light for saved dark and system-dark startup. A later root-class
  change updates Sonner, confirming the missing initial reconciliation.
- Permanent component RED: two dark-start failures and nine passing controls
  or provider tests. The expected failure was root `dark` with Sonner `light`.
- Production-build browser RED: desktop had two dark-start failures and one
  passing light case; mobile had the same two failures and three passing
  controls, including both existing mobile theme-toggle scenarios.
- GREEN: 11 component/provider tests, three desktop browser tests, and five
  mobile browser tests passed with retries disabled. The work order records
  the exact commands and the workspace-path discovery workaround.
- Desktop and mobile dark screenshots were inspected. Toast background, text,
  border, viewport containment, and absence of horizontal document overflow
  passed. Evidence is retained for this session under
  `/tmp/kandev-toast-evidence.Eb9wFU/` (`desktop-after.png`,
  `mobile-after.png`, and `mobile-before.png`).
- Web typecheck, targeted ESLint, targeted Prettier, and `git diff --check`
  passed. Specification linter tests passed all 30 tests during planning;
  full specification lint also passed.
- PR review added and passed an explicit shared-UI ESLint invocation and
  clarified the historical browser commands, including the temporary
  configuration needed to reproduce this checkout's filename-filter workaround.
- The temporary test configuration was removed. The managed runner completed
  teardown of its isolated test backends and browsers.
- Public docs: no change needed. This correction changes notification colors;
  navigation, copy, configuration, and operator instructions are unchanged.

## Risks

- Development StrictMode effect replay can hide the bug; preserve the
  production-style mount regression and use built Vite assets for E2E.
- A document already carrying the desired class will miss the regression.
- Real Sonner notifications render asynchronously. Wait for the notification
  before reading `data-sonner-theme` or computed colors.
- Disabling rich colors would mask the bug while changing notification
  semantics; keep them enabled.
