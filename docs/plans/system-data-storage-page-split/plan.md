---
created: 2026-09-03
status: implemented
requirements:
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-001
system_design:
  - ../../specs/system-page/system-design/system-data-storage-pages.md
legacy_specs:
  - ../../specs/system-page/requirements/system-page.md
  - ../../specs/system-page/system-design/system-page-01.md
---

# Implementation Plan: System Data and Storage Page Split

## Overview

This change creates separate `Data & Logs` and `Storage` pages. The first work
order changes routes, navigation, discovery, copy, and browser coverage. The
second work order updates public operations documentation and screenshots.

## Scope

### In scope

- Keep Database, Backups, and Logs on `/settings/system/data-storage`.
- Render storage maintenance on `/settings/system/storage`.
- Add a direct Storage entry to desktop and phone settings navigation.
- Reassign storage discovery targets to the Storage page.
- Preserve legacy redirects and storage save protection.
- Update all affected locales, tests, operations text, and screenshots.

### Out of scope

- Backend, API, database, or storage policy changes.
- New settings navigation levels or responsive primitives.
- Separate Database, Backups, or Logs pages.
- A rename of the existing `Data & Logs` path.

## Technical approach

### Route and component composition

Rename the combined content component to match its remaining Data and Logs
scope. Remove `StorageMaintenanceSettings` and its separator from that
component.

Replace the Storage redirect in `apps/web/src/settings-routes.tsx` with a
`SystemRouteShell` that renders `StorageMaintenanceSettings`. Reuse the existing
Storage title and description keys.

### Navigation and discovery

Add `SYSTEM_STORAGE_SETTINGS_HREF` to the System discovery catalog. Add a
Storage page definition and reparent every storage section to it. Keep existing
storage target IDs stable.

Add the Storage row to `SETTINGS_MENU_SECTIONS`. Add the translated breadcrumb
mapping for the `storage` path segment. Update route and catalog unit tests.

### Localization

Change `dataStoragePageDescription` so that it names database statistics,
backups, and server logs only. Update every locale. Use `pnpm run i18n:zh-hant`
for the Traditional Chinese catalogs.

### Compatibility

Keep the Database, Backups, and Logs redirects unchanged. Keep the current
`Data & Logs` URL and discovery page identity unchanged.

### Public documentation

Update `docs/public/operations.md` to point storage tasks to the Storage page.
Recapture the four affected screenshots with isolated disposable data. The
screenshots must show the Storage route and must not expose developer data.

## Tests

- `AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.1` through `.7`: route, menu,
  breadcrumb, discovery, and copy unit tests.
- `AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.9`: existing storage policy and role
  gating tests after route migration.

## E2E tests

- `AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.1` through `.5`: desktop System
  navigation and the Database, Backups, Logs, and Storage page specifications.
- `AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.7` through `.9`: mobile System
  navigation, storage maintenance, and member gating specifications.
- `AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-001.9`: authenticated and container-backed
  storage maintenance specifications.

## Work orders

- [x] [Task 01: Split the System pages](task-01-split-system-pages.md)
- [x] [Task 02: Update storage operations documentation](task-02-update-storage-operations-docs.md)

## Verification results

Task 01 is done. Focused unit tests passed with 107 tests, typecheck passed,
full web lint passed, the i18n check passed, and the desktop, authenticated, and
phone Playwright suites passed with 18, 2, and 10 tests respectively. The
container-backed storage suite could not start because Docker is unavailable in
the environment.

Task 02 is done. Public documentation tests and validation passed, and the four
Storage screenshots were recaptured from an isolated Playwright fixture with
fictional paths and no developer data. A landing-repository capture target was
not configured, so the documented screenshot fallback used the isolated native
Playwright capture.

The PR review follow-up added an explicit pseudo-locale assertion for the
Storage title and clarified the immediate command wording in the system design.
The focused remediation test passed.

## Risks

- Tests that use `/settings/system/data-storage` for storage can pass against
  the wrong route unless every reference is classified by feature.
- Auth tests currently assert backup and storage controls on one page. They
  must exercise both routes in the same role context.
- Existing operations screenshots show the `Data & Logs` breadcrumb. They must
  be recaptured after the route split.
- Discovery parent changes can break exact-target navigation if hrefs and
  parent identities do not change together.
