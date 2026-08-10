---
spec: docs/specs/dev-preview-title-prefixes/spec.md
created: 2026-08-09
status: complete
---

# Implementation Plan: Environment-specific browser tab title prefixes

## Overview

Reuse the merged PR #2459 title-prefix contract and supply environment-specific
defaults at the launch boundaries. The development profile will provide `Dev`,
the PR preview startup script will provide `Preview`, and the CLI supervisor
will preserve the prefix across restarts. Focused Go, CLI, browser, and docs
checks will cover the launcher contracts and the visible browser result.

## Backend and launchers

### Development profile

- `apps/backend/internal/profiles/profiles.yaml` (the target of the root
  `profiles.yaml` symlink): add `KANDEV_WEB_TITLE_PREFIX` with an empty `prod`
  and `e2e` value and `Dev` as the `dev` value.
- `apps/backend/internal/profiles/profiles_test.go`: assert the dev profile
  supplies `Dev` while the existing profile precedence remains intact.

### PR preview launcher

- `apps/backend/cmd/preview/sprite_ops.go`: export
  `KANDEV_WEB_TITLE_PREFIX=Preview` in the generated preview startup script.
- `apps/backend/cmd/preview/sprite_ops_test.go`: pin the generated script to
  the `Preview` prefix alongside its existing startup assertions.

### Supervisor environment contract

- `apps/cli/src/supervisor/manifest.ts`: add `KANDEV_WEB_TITLE_PREFIX` to the
  restart-manifest allowlist so any configured prefix survives a supervised
  backend restart.
- `apps/cli/src/supervisor/manifest.test.ts`: verify the title-prefix variable
  is retained while arbitrary variables remain excluded.

## Frontend and browser behavior

No new frontend implementation is required. The existing PR #2459 path already
renders the configured prefix in the server shell and applies it for the
`/api/v1/app-state` boot path.

Add `apps/web/e2e/tests/system/title-prefix.spec.ts` to restart the fixture
backend with the development profile and with an explicit preview prefix, then
assert both visible browser titles through the UI. The test restores the worker
environment afterward.

### Mobile parity contract

The affected surface is browser metadata (`document.title`), not layout,
navigation, touch input, scrolling, or viewport-bound UI. The desktop and
mobile outcomes are identical, so no mobile-specific interaction test is
needed; the E2E test and existing title helper contract cover the same
viewport-independent behavior.

## Public documentation

- `docs/public/configuration.md`: explain that `make dev` defaults the title
  prefix to `Dev` and PR previews use `Preview`, while explicit environment
  values still override defaults.
- `docs/configuration.md`: keep the internal configuration reference aligned.

## Tests

- **What:** the development profile resolves the title prefix to `Dev` and
  leaves production/e2e defaults blank.
  **File:** `apps/backend/internal/profiles/profiles_test.go`.
  **How:** profile application unit test with isolated environment variables.
- **What:** the preview startup script exports `Preview`.
  **File:** `apps/backend/cmd/preview/sprite_ops_test.go`.
  **How:** generated-script contract assertion.
- **What:** the supervisor retains the title-prefix environment variable.
  **File:** `apps/cli/src/supervisor/manifest.test.ts`.
  **How:** allowlist unit test.
- **What:** a configured runtime prefix is visible in the browser title.
  **File:** `apps/web/e2e/tests/system/title-prefix.spec.ts`.
  **How:** worker backend restart plus Playwright UI assertion.

## E2E Tests

- **Scenarios:** GIVEN a fixture backend restarted with the development profile,
  WHEN the browser opens Kandev, THEN the page title is `Dev Kandev`; GIVEN a
  fixture backend restarted with `KANDEV_WEB_TITLE_PREFIX=Preview`, THEN the
  page title is `Preview Kandev`.
- **File:** `apps/web/e2e/tests/system/title-prefix.spec.ts`.
- **What to verify:** the user-visible browser tab title after a fresh page
  navigation.

## Verification Results

- `cd apps/backend && go test -tags fts5 ./internal/profiles ./cmd/preview` —
  passed, 30 tests in 2 packages.
- `cd apps && pnpm --filter kandev test -- --run src/dev.test.ts src/supervisor/manifest.test.ts` —
  passed, 2 files and 6 tests.
- `cd apps/web && pnpm e2e:run --project chromium tests/system/title-prefix.spec.ts` —
  passed, 2 Playwright tests (`Dev Kandev` and `Preview Kandev`).
- `node --test scripts/validate-public-docs.test.mjs` — passed, 58 tests.
- `node scripts/validate-public-docs.mjs` — passed, 41 published pages.
- `pnpm exec prettier --check ...` — passed for the changed TypeScript files.
- `git diff --check` — passed.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential; the launch contracts and their tests are one cohesive
change):

- [x] [task-01-launch-prefixes](task-01-launch-prefixes.md)

## Open Questions

None.
