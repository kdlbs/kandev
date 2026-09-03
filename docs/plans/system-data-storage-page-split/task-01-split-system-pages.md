---
id: "01-split-system-pages"
title: "Split the System pages"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.1
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.2
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.3
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.4
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.5
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.6
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.7
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.8
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.9
system_design:
  - ../../specs/system-page/system-design/system-data-storage-pages.md
---

# Task 01: Split the System Pages

## Summary

Create direct `Data & Logs` and `Storage` routes. Update navigation, discovery,
localization, unit tests, and Playwright coverage as one vertical change.

## In scope

- Remove storage maintenance from the Data and Logs content component.
- Render storage maintenance directly on `/settings/system/storage`.
- Add the Storage route to the System menu, breadcrumbs, and discovery catalog.
- Update the Data and Logs description in every locale.
- Move every storage-focused Playwright flow to the Storage route.
- Make authenticated role tests exercise both owning pages.
- Preserve legacy redirects and exact discovery targets.

## Out of scope

- Backend or API changes.
- Storage policy or cleanup behavior changes.
- Public operations text and screenshots.

## Acceptance

- Each route renders only its specified operational sections.
- Desktop and phone navigation open both routes directly.
- Storage save, permission, action, and containment tests pass on the Storage
  route.

## Verification

```bash
pnpm exec vitest run components/settings/system/system-route-copy.test.ts components/settings/system/system-invisible-copy.test.tsx components/app-sidebar/sections/settings/settings-nav-copy.test.ts components/app-sidebar/sections/settings/settings-tree.test.ts lib/settings-discovery/catalog.test.ts src/settings-routes.test.ts
pnpm run i18n:check
pnpm e2e:run --project chromium tests/system/sidebar-navigation.spec.ts tests/system/database-page.spec.ts tests/system/backups-page.spec.ts tests/system/logs-page.spec.ts tests/system/storage-maintenance.spec.ts
pnpm e2e:run --project auth tests/auth/system-data-storage-member-gating.spec.ts
pnpm e2e:run --project mobile-chrome tests/system/mobile-database-page.spec.ts tests/system/mobile-logs-bundle.spec.ts tests/system/mobile-storage-maintenance.spec.ts tests/auth/mobile-system-data-storage-member-gating.spec.ts
pnpm e2e:run --project containers tests/docker/storage-maintenance.spec.ts
```

Run these commands from `apps/web`.

## Files likely touched

- `apps/web/components/settings/system/data-logs-settings.tsx`
- `apps/web/components/settings/system/system-route-copy.test.ts`
- `apps/web/components/settings/system/system-invisible-copy.test.tsx`
- `apps/web/components/settings/settings-breadcrumb-labels.ts`
- `apps/web/components/app-sidebar/sections/settings/settings-menu-sections.ts`
- `apps/web/components/app-sidebar/sections/settings/settings-nav-copy.test.ts`
- `apps/web/components/app-sidebar/sections/settings/settings-tree.test.ts`
- `apps/web/lib/settings-discovery/catalog/system.ts`
- `apps/web/lib/settings-discovery/catalog.test.ts`
- `apps/web/src/settings-routes.tsx`
- `apps/web/src/settings-routes.test.ts`
- `apps/web/src/locales/*/system.json`
- `apps/web/e2e/tests/system/*.spec.ts`
- `apps/web/e2e/tests/auth/*data-storage-member-gating*.ts`
- `apps/web/e2e/tests/docker/storage-maintenance.spec.ts`
- `apps/web/e2e/tests/task/unarchive-storage-recovery.spec.ts`

## Dependencies

None.

## Risks

- A broad route replacement can move database or log tests to Storage by
  mistake. Classify each reference before editing it.
- The auth tests use manual browser contexts. Both route visits must retain the
  same authenticated context.
- The Storage route must keep the route-scoped save contributor mounted until
  the navigation guard resolves.

## Parallelism

`sequential`

## Inputs

- `REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-001` and all acceptance criteria.
- `docs/specs/system-page/system-design/system-data-storage-pages.md`.
- ADRs 0045, 0046, and 2026-08-04-navigation-manifest-boundaries.
- Existing System route, discovery, settings menu, and Playwright patterns.

## Results

Implemented in commit `407e3df65`. The route composition, System navigation,
breadcrumbs, discovery ownership, localized copy, unit tests, and desktop,
authenticated, phone, and container-backed Playwright references were updated.

Verification passed:

- Focused Vitest command: 107 tests passed.
- `pnpm run typecheck` passed.
- Full `pnpm run lint` passed.
- `pnpm run i18n:check` passed.
- Desktop Playwright command: 18 tests passed.
- Authenticated role command: 2 tests passed.
- Phone Playwright command: 10 tests passed.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check` passed.

The exact container Playwright command was attempted, but Docker is unavailable
in this environment, so that suite could not run.
