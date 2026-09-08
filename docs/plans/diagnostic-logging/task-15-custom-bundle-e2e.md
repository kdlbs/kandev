---
id: "15-custom-bundle-e2e"
title: "Custom bundle E2E"
status: done
wave: 12
depends_on:
  - "14-bundle-customizer-ui"
plan: "plan.md"
spec: "../../specs/platform/requirements/diagnostic-logging.md"
---

# Task 15: Custom bundle E2E

## Acceptance

- Desktop E2E proves standard/custom/debug-capability states, required ACP
  session selection, owner-visible candidates, strong disclosure, runtime
  index fields, raw+normalized ZIP entries, and partial unavailable sessions.
- Mobile E2E completes the same source/session outcome through the inset drawer
  and proves internal scrolling, viewport containment, focus/dismiss behavior,
  safe-area clearance, ≥44px targets, and no document horizontal overflow.
- Tests rebuild production backend/web artifacts and inspect the downloaded ZIP
  rather than asserting only button visibility.

## Verification

```bash
cd apps/web
pnpm e2e:run e2e/tests/system/logs-page.spec.ts \
  e2e/tests/system/mobile-logs-bundle.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/system/logs-page.spec.ts`
- `apps/web/e2e/tests/system/mobile-logs-bundle.spec.ts`
- `apps/web/e2e/fixtures/backend.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/backend/internal/debug/fixture.go`

## Dependencies

- Task 14 completes the end-to-end backend/frontend behavior and selectors.

## Parallelism

Parallel-safe with Task 16 after Task 14. This task owns E2E fixtures/specs;
Task 16 owns public docs and agent skill guidance.

## Inputs

- Spec scenarios for standard disclosure, debug gating, authorization,
  selected/unavailable ACP sessions, runtime index, and phone Drawer behavior.
- Plan E2E section and mobile design contract.
- Existing Logs desktop/mobile specs and production-build E2E fixtures.

## Risks

- Fixture ACP content must be synthetic and must not read a developer's real
  debug directory, database, credentials, sessions, or executor data.
- ZIP assertions must verify absence of message-bearing sources in standard and
  runtime-only bundles as well as presence in the explicit ACP fixture.

## Output contract

Report seeded states, downloaded artifact assertions, desktop/mobile commands
and results, screenshots/traces on failure, cleanup evidence, blockers/risks,
and synchronize this task plus `plan.md` status.

## Results

Added desktop and mobile coverage for the customizer and standard bundle
workflow. The desktop spec parses the downloaded stored ZIP, verifies
`manifest.json`, backend/frontend entries, and the requested source set, then
checks the source customizer, runtime-index option, and debug-only ACP button
boundary. The mobile spec verifies the inset Drawer, internal source surface,
44px actions, and document-width containment.

- Verification: `pnpm e2e:run e2e/tests/system/logs-page.spec.ts` (pass; 2
  Chromium tests with production backend/web builds).
- Verification: `pnpm e2e:run --no-build --project mobile-chrome
  e2e/tests/system/mobile-logs-bundle.spec.ts` (pass; 2 mobile tests after the
  production build).
