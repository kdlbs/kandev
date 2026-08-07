---
id: "05-plugin-hook-e2e-regressions"
title: "Plugin hook E2E regressions"
status: completed
wave: 3
depends_on:
  - "01-authoritative-plugin-lifecycle"
  - "02-fail-closed-user-state-uninstall"
  - "03-responsive-task-menu-context"
  - "04-bounded-mobile-plugin-panels"
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 05: Plugin hook E2E regressions

## Acceptance

- Desktop E2E proves a plugin panel survives reload/update but closes on explicit
  disable, and desktop task-menu actions observe `presentation: "desktop"`.
- Mobile E2E opens a panel through the grouped picker, proves touch/overflow geometry,
  observes `presentation: "mobile"` from a kanban action, and falls back to Chat after
  definitive removal.
- The real packaged fixture and authenticated `host.storage` path remain the evidence;
  tests do not replace them with page-injected registry mocks.

## Verification

```bash
make build-backend build-web build-e2e-plugin-package
cd apps/web && pnpm e2e --project=chromium tests/plugins/plugin-task-panel.spec.ts
cd apps/web && pnpm e2e --project=mobile-chrome tests/plugins/mobile-plugin-task-panel.spec.ts
```

## Files likely touched

- `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`
- `apps/backend/cmd/plugin-fixture/fixture_package_test.go` if fixture assertions change
- `apps/web/e2e/tests/plugins/plugin-task-panel.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-plugin-task-panel.spec.ts`
- `apps/web/e2e/pages/session-page.ts` only for reusable picker selectors

## Dependencies

Tasks 01–04.

## Parallelism

`sequential`; E2E consumes all behavior tasks and shares the fixture across desktop and
mobile scenarios.

## Inputs

- Every plugin-hook scenario added to `docs/specs/plugins/spec.md`.
- Existing real fixture install helpers and plugin-user-state E2E patterns.

## Risks

E2E runs against built assets. Always rebuild backend, web, and the fixture package in
the same command sequence or the assertions may exercise stale code.

## Output contract

Report fixture changes, exact build/E2E commands and counts, screenshots/artifacts on
failure, teardown evidence, and synchronize task/plan status/results.

## Results

- Rebuilt the real fixture with `rtk make -C apps/backend e2e-plugin-package`.
- `rtk pnpm e2e:run --host --project chromium tests/plugins/plugin-task-panel.spec.ts`
  — 6 tests passed, including desktop reload/disable and `presentation: "desktop"`.
- `rtk pnpm e2e:run --host --project mobile-chrome tests/plugins/mobile-plugin-task-panel.spec.ts`
  — 2 tests passed, including grouped picker geometry, definitive-removal Chat
  fallback, no horizontal overflow, and `presentation: "mobile"`.
- The tests use the packaged fixture and authenticated `host.storage` path; no
  page-injected registry mocks were added.
