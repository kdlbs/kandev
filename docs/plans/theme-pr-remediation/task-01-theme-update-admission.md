---
id: "01-theme-update-admission"
title: "Constrain theme update admission"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-THEMES-001
  - REQ-PLUGINS-THEMES-003
acceptance_criteria:
  - AC-PLUGINS-THEMES-001.2
  - AC-PLUGINS-THEMES-003.1
  - AC-PLUGINS-THEMES-003.3
system_design:
  - ../../specs/plugins/system-design/theme-packages.md
---

# Task 01: Constrain Theme Update Admission

## Summary

Make automatic and Installed-tab theme updates validate the selected release
before admission. Keep the existing theme installation and reviewed-import
contracts; do not route an update from a data-only source into runtime execution.

## In scope

- Extract a context-based bounded downloader from `themes_handlers.go` into
  service-owned theme release code, preserving timeout, redirect and size caps.
- Bind kind, ID, version and digest before installation. Classify using both
  installed profile and listing kind; reject a theme-to-plugin listing drift.
- Change the manual update callback to carry the complete catalog entry. Resolve
  the installed record and use existing theme prepare/commit APIs for themes.
- Preserve the ordinary plugin Browse install callback and existing toasts,
  busy-state, refresh and mark-updated behavior.
- Add real-store service regressions and manual-update hook/browser evidence.

## Out of scope

New endpoints, general plugin installer redesign, new permissions and UI layout.

## Acceptance

1. Wrong or missing expected identity, wrong digest, and executable archives
   (including another package ID) fail without starting a runtime, loading a
   bundle or changing the old theme's record, inventory or files.
2. A valid update publishes the expected version and inventory without a runtime
   call; installed theme/listing-kind disagreement fails closed.
3. Manual and automatic entry points have distinct regression coverage. Desktop
   and phone users complete the existing Installed-tab update flow successfully.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/plugins -run 'TestThemeMarketplaceUpdate|TestThemeReview|TestThemeLifecycle' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/plugins -run 'TestThemeMarketplaceUpdate|TestThemeReview|TestThemeLifecycle' -count=1)
(cd apps/web && pnpm exec vitest run components/settings/plugins/use-plugin-update-action.test.tsx components/settings/plugins/use-plugin-actions.test.tsx hooks/domains/plugins/use-plugin-updates.test.ts lib/api/domains/themes-api.test.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/plugins/theme-marketplace.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/plugins/mobile-theme-marketplace-update.spec.ts)
```

Write the executable case in `TestThemeMarketplaceUpdateAdmission` first and confirm the
existing production path reaches the fake runtime before correction. Add a
valid-update positive control with identical admission prerequisites.

## Files likely touched

- `apps/backend/internal/plugins/{autoupdate.go,themes_handlers.go}`
- `apps/backend/internal/plugins/{service_theme_updates.go,service_theme_updates_test.go}` (new)
- `apps/backend/internal/plugins/{themes_handlers_test.go,autoupdate_test.go}`
- `apps/web/components/settings/plugins/{plugins-settings.tsx,use-plugin-update-action.ts,use-plugin-update-action.test.tsx,use-plugin-actions.ts,use-plugin-actions.test.tsx}`
- `apps/web/lib/types/plugins.ts`
- `apps/web/e2e/tests/plugins/{theme-marketplace.spec.ts,mobile-theme-marketplace-update.spec.ts,theme-marketplace-update-flow.ts,theme-package-fixture.ts}`

## Dependencies

None.

## Risks

Do not trust a marketplace label over the installed profile. Do not pass Gin
objects or handler DTOs into the shared application helper. Keep existing
executable plugin behavior and theme lifecycle locking intact.

## Parallelism

`sequential`

## Inputs

The package design's Preparation, Lifecycle and Marketplace sections; the
accepted data-only theme ADR; existing auto-update and reviewed-import tests.

## Results

- Red: the initial service regression started `unexpected-runtime`; wrong ID,
  version, digest and kind cases were also accepted. The manual-update callback
  test proved that only the URL reached the installer.
- Green: automatic admission validates the bounded archive's exact digest, theme
  profile, ID and version in isolation before installation. Installed/listing
  kind mismatches fail closed. Manual updates retain the complete entry and use
  the existing prepare/commit APIs; ordinary plugin updates still load their UI.
- Service rejection cases preserve the prior record, files and inventory and
  invoke no runtime. Additional prepare-route cases leave no committable review
  for mismatched identity/digest or executable data. Missing identity fails
  before download.
- Targeted service/review/lifecycle tests passed with `-race`; the full plugin,
  pack and verifier suites passed. The four focused frontend files passed
  (46 tests). Desktop catalog install/update and phone update checks passed.
- The final combined verification is recorded in Task 04.
