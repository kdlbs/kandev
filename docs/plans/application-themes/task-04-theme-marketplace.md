---
id: "04-theme-marketplace"
title: "Marketplace discovery and import review"
status: done
wave: 4
depends_on:
  - "03-theme-rendering"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-THEMES-001
  - REQ-PLUGINS-THEMES-002
  - REQ-PLUGINS-THEMES-003
  - REQ-UI-APP-THEMES-002
acceptance_criteria:
  - AC-PLUGINS-THEMES-001.3
  - AC-PLUGINS-THEMES-001.4
  - AC-PLUGINS-THEMES-002.1
  - AC-PLUGINS-THEMES-002.2
  - AC-PLUGINS-THEMES-002.3
  - AC-PLUGINS-THEMES-002.4
  - AC-PLUGINS-THEMES-003.1
  - AC-PLUGINS-THEMES-003.2
  - AC-PLUGINS-THEMES-003.4
  - AC-PLUGINS-THEMES-003.5
  - AC-UI-APP-THEMES-002.4
  - AC-UI-APP-THEMES-002.5
system_design:
  - ../../specs/plugins/system-design/theme-packages.md
  - ../../specs/ui/system-design/application-themes.md
---

# Task 04: Marketplace discovery and import review

## Summary

Expose theme discovery and reviewed import through the existing marketplace.
Make community release curation validate data-only artifacts using the same host
format, while administrators can also import standalone VS Code files.

## In scope

Add theme kind/category across registry schema/build/check-releases, Go catalog,
web API/query types, source merging/filtering and installed/update states. Reuse
artifact checksum/ID/version validation and support old plugin/canvas catalogs.
Build Themes list/detail and native/JSON import review with missing-base choice,
scoped samples, compatibility/contrast report, permissions, prepare/commit retry,
expiry/cancel and Go to Appearance after install. Add desktop/phone flows.

## Out of scope

New marketplace infrastructure, automatic external publishing, production theme
listings, selecting a newly installed theme automatically, widening host.theme,
and implementing native plugin code in this monorepo.

## Acceptance

1. Registry CI and host reject a runtime package labelled theme, missing release
   license, incompatible identity/version and mismatched digest; valid native
   theme releases are discoverable without changing existing source precedence.
2. Real admin import/review/confirm uses the exact prepared input and displays
   limitations; cancel/errors/non-admin actions do not mutate installed state or
   the saved appearance. JSONC and native archives share normalization/validation.
3. Desktop and phone marketplace/review follow UI-02/UI-03 with long-report
   scrolling, keyboard focus, contained layouts and safe >=44px phone actions.

## ASCII UI preview

UI-02 and UI-03 from [the full preview](plan.md#ascii-ui-preview):

```text
Themes [Search] [Import theme] -> Listing -> Review install
Desktop: bounded review dialog, scrolling report, fixed Cancel / Install themes
Phone:   Back/header, one scrolling report, safe-area Cancel / Install themes
```

Review includes metadata/variants, sample colors, warnings, base choice when
required, and explicit confirmation. Install never auto-applies. Structural
contracts and AC mapping are in UI-02/UI-03; labels are illustrative localized copy.

## Verification

Run from repository root after Task 03 install/build preparation:

```bash
(cd apps/backend && go test -trimpath ./internal/plugins/marketplace ./internal/plugins -run 'Theme|Marketplace')
node --test plugin-registry/build-index.test.mjs plugin-registry/check-releases.test.mjs plugin-registry/workflow-contract.test.mjs
(cd apps/web && pnpm exec vitest run components/settings/plugins hooks/domains/plugins/use-marketplace.test.ts lib/api/domains/marketplace-api.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium -- tests/plugins/theme-marketplace.spec.ts tests/plugins/theme-import.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- tests/plugins/mobile-theme-marketplace.spec.ts tests/plugins/mobile-theme-import.spec.ts)
git diff --check
```

Use Node 24. If source changes after desktop build, remove --no-build on phone.
Add theme-marketplace.test.tsx/theme-import-review.test.tsx in the settings/plugins
suite; new E2E use test-base and actual disposable catalog/archive fixtures.
Run narrow changed-file Go/TS lint and record exact paths. Reuse configured local
worker/shard budgets, causal waits and existing admin/member fixture patterns.

## Files likely touched

- `plugin-registry/schema.json`, build-index.mjs, check-releases.mjs and tests.
- `apps/backend/internal/plugins/marketplace/{types,catalog}.go`,
  marketplace_handlers.go and tests; verifier kind output from Task 01.
- `apps/web/lib/api/domains/marketplace-api.ts`, type/query/store/hook tests.
- `apps/web/components/settings/plugins/plugins-settings.tsx` and new
  theme-marketplace/detail/import-review components and tests.
- `apps/web/e2e/tests/plugins/{theme-marketplace,theme-import,mobile-theme-marketplace,mobile-theme-import}.spec.ts` (new), scoped fixture helpers.
- All supported locale catalogs, pseudo/generated Traditional Chinese as needed.

## Dependencies

Task 03 supplies safe scoped sample rendering; Task 02 exact review APIs;
Task 01 validator/pack/verifier. No production source or registry publishing.
Read /e2e and /mobile-parity before implementing.

## Risks

Catalog category masquerading as artifact kind, old-host schema compatibility,
review becoming a second installer, and treating a phone dialog as squeezed desktop UI.

## Parallelism

`sequential`

## Inputs

Both designs, UI-02/UI-03, existing canvas-marketplace and plugins-settings,
registry build/check tests, and admin member E2E precedents.

## Results

Implemented the Themes catalog kind, registry artifact/license/digest validation,
update discovery, admin-only prepare/review/commit, VS Code JSON/JSONC conversion,
and responsive import review. Installation preserves the user's selection.

Registry tests passed (46). Backend plugin/packer/verifier suites passed. The
production-build marketplace E2E installed a native package from a local catalog
through the actual review flow. Shared import controls are exercised on desktop
and phone. Planned scenario names above are design targets; the delivered test
files and actual executions are recorded in [delivery evidence](delivery.md).
