---
spec: docs/specs/platform/requirements/dev-preview-title-prefixes.md
created: 2026-08-10
status: complete
---

# Implementation Plan: Keep Debug Start on the Production Profile

## Overview

`make start-debug` sets the legacy pprof debug variable. The profile loader
currently treats that variable as a development selector, so it applies the
`Dev` title prefix. Make `KANDEV_DEBUG_DEV_MODE` the only development selector,
keep pprof as debug behavior, and preserve `Dev Kandev` for `make dev`. The
debug launcher supplies `Debug` as the default title prefix, producing
`Debug Kandev` without selecting the development profile.

The work proceeds from the backend profile contract to the launcher checks and
then to the browser regression test and public documentation.

The confirmed root cause is in `profiles.DetectEnvironment`: its dev branch
checks `KANDEV_DEBUG_PPROF_ENABLED` even though the release debug launcher sets
that variable without selecting the development profile.

## Backend and launchers

### Profile selection

- `apps/backend/internal/profiles/profiles.go`: make
  `KANDEV_DEBUG_DEV_MODE=true` the only non-e2e signal for the `dev` profile.
  Keep `KANDEV_DEBUG_PPROF_ENABLED` available to the debug configuration and
  pprof routes.
- `apps/backend/internal/profiles/profiles_test.go`: add a pprof-only
  regression case that resolves to `prod`, keeps the title prefix out of the
  profile defaults, and keeps production feature defaults.

### Debug title default

- `apps/backend/internal/launcher/env.go`: when the native debug launcher is
  used, supply `KANDEV_WEB_TITLE_PREFIX=Debug` only when the caller has not
  already supplied a prefix. Keep the explicit environment override contract.
- `apps/backend/internal/launcher/start_test.go`: verify the debug launcher
  supplies `Debug` by default and preserves an explicit prefix.
- `apps/backend/internal/launcher/supervisor.go`: preserve the title prefix in
  the restart manifest so a supervised restart keeps `Debug Kandev`.
- `apps/backend/internal/launcher/supervisor_test.go`: cover the allowlisted
  title prefix in the restart manifest.

### Make targets

- `apps/backend/Makefile`: make the direct `dev` target export the canonical
  `KANDEV_DEBUG_DEV_MODE=true` selector. Make the direct `start-debug` target
  default to the `Debug` prefix while preserving an explicitly supplied
  `KANDEV_WEB_TITLE_PREFIX`. The debug target continues to export pprof and
  debug logging without the dev selector.
- `scripts/check-make-shells`: update the expected `dev` command for Unix,
  native Windows, and Git Bash/MSYS, plus the direct `start-debug` title
  environment.

### Durable decision record

- `docs/decisions/0007-runtime-feature-flags.md`: reconcile the profile
  selector table with the corrected contract.
- `docs/decisions/2026-08-10-debug-launcher-profile-selection.md`: preserve the
  reason for separating diagnostic behavior from environment identity.
- `docs/decisions/INDEX.md`: index the new decision.

## Frontend

No frontend production code changes are required. The existing server-shell
and SPA title composition already uses `KANDEV_WEB_TITLE_PREFIX` correctly.

The existing title-prefix browser test will cover the debug-start result. This
is browser metadata, not a layout or interaction surface. Desktop and phone
viewports therefore have the same expected title, and no mobile-specific
interaction test is required.

## Public documentation

- `docs/public/configuration.md`: state that `make start-debug` keeps the
  production profile and uses the `Debug Kandev` title while enabling
  diagnostics.
- `docs/public/cli.md`: explain that `--debug` does not select the development
  profile; `make dev` is the development-profile launcher.
- `docs/configuration.md`: keep the internal configuration reference aligned.

## Tests

- **What:** pprof-only startup selects `prod` and does not apply `Dev`.
  **File:** `apps/backend/internal/profiles/profiles_test.go`.
  **How:** isolated environment unit test through `ApplyProfile`.
- **What:** the native debug launcher supplies the `Debug` prefix by default
  and preserves an explicit prefix.
  **File:** `apps/backend/internal/launcher/start_test.go`.
  **How:** inspect the environment assembled for debug and non-debug starts.
- **What:** supervised restarts preserve the debug title prefix.
  **File:** `apps/backend/internal/launcher/supervisor_test.go`.
  **How:** assert the debug prefix survives the restart manifest allowlist.
- **What:** direct `make dev` emits the canonical dev selector across shells.
  **File:** `scripts/check-make-shells`.
  **How:** run the existing deterministic `make -n` shell matrix.
- **What:** the debug-start launcher uses the distinct debug title.
  **File:** `apps/web/e2e/tests/system/title-prefix.spec.ts`.
  **How:** restart the fixture with pprof enabled and both profile selectors
  disabled plus the launcher's `Debug` prefix, navigate to `/`, and assert
  `Debug Kandev`.
- **What:** development and preview title behavior remains unchanged.
  **File:** `apps/web/e2e/tests/system/title-prefix.spec.ts`.
  **How:** retain the existing `Dev Kandev` and `Preview Kandev` assertions.

## E2E Tests

- **Scenario:** GIVEN a backend restarted with pprof/debug behavior and no
  development or e2e selector, WHEN the browser opens Kandev, THEN the title is
  `Debug Kandev`.
- **Scenario:** GIVEN a backend restarted with the development selector, WHEN
  the browser opens Kandev, THEN the title is `Dev Kandev`.
- **Scenario:** GIVEN a backend with an explicit `Preview` prefix, WHEN the
  browser opens Kandev, THEN the title is `Preview Kandev`.
- **File:** `apps/web/e2e/tests/system/title-prefix.spec.ts`.
- **Mobile note:** `document.title` is viewport-independent. The existing
  browser metadata assertion covers the same user outcome on a phone, so no
  `mobile-*.spec.ts` file is needed.

## Verification Results

- `make test-backend` — passed (full backend test suite).
- `make lint-backend` — passed (0 issues).
- `make build-backend` — passed (backend binaries built).
- `cd apps/backend && make check-make-shells` — passed for Unix, native
  Windows, and Git Bash/MSYS dispatch, including explicit prefixes with spaces.
- `cd apps/web && pnpm e2e:run --project chromium tests/system/title-prefix.spec.ts`
  — passed (3 tests).
- `node --test scripts/validate-public-docs.test.mjs` — passed (58 tests).
- `node scripts/validate-public-docs.mjs` — passed (41 published docs pages).
- `git diff --check` — passed.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential):

- [x] [task-01-profile-selector-contract](task-01-profile-selector-contract.md)

Wave 2 (sequential, depends on Wave 1):

- [x] [task-02-title-regression-docs](task-02-title-regression-docs.md)

Parallel delegation is not authorized by this plan. The work stays in the
primary session and follows the dependency order above.

## Open Questions

None.
