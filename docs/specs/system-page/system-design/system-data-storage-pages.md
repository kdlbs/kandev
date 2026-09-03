---
status: draft
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-001
---

# System Data and Storage Pages System Design

## Purpose and boundaries

The system-page system owns these routes and their operational content. This
design changes frontend composition and navigation only. Existing backend APIs,
permissions, jobs, and persistence remain unchanged.

This design supersedes the route allocation for Database, Backups, Logs, and
Storage in `system-page-01.md`.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-001` | [Route composition](#route-composition), [Navigation and discovery](#navigation-and-discovery), [Mobile design contract](#mobile-design-contract), [Compatibility](#compatibility) |

## Route composition

`apps/web/src/settings-routes.tsx` will own two static route renderers.

The `/settings/system/data-storage` renderer will use `SystemRouteShell` with
the existing `Data & Logs` title. Its content component will render these
sections in order:

1. Database.
2. Backups.
3. Logs.

The `/settings/system/storage` renderer will use `SystemRouteShell` with the
existing Storage title and description. It will render
`StorageMaintenanceSettings` directly. This removes the duplicate Storage
section heading from the new page.

The route split will not duplicate domain hooks or action handlers.
`StorageMaintenanceSettings` will remain the single owner of storage data,
draft state, and action state.

## Navigation and discovery

`apps/web/lib/settings-discovery/catalog/system.ts` will export a stable href
for the Storage page. The System menu will add a direct Storage row after
`Data & Logs`.

The existing `system-data-storage` discovery page identity and href will remain
stable. Database, Backups, and Logs will remain its children. A new
`system-storage` page identity will own all existing storage targets.

The breadcrumb label map will resolve the `storage` segment through the same
translation key as the menu and page title. Catalog order will place Storage
after `Data & Logs` and before Feature Toggles.

## Copy and localization

The `Data & Logs` page description will no longer mention disk cleanup. The
Storage route will reuse the existing localized Storage title and description.

All changed copy will remain complete in English, pseudo, Portuguese, Simplified
Chinese, and both Traditional Chinese catalogs. Traditional Chinese updates
will use the repository conversion command.

## Save lifecycle

The Storage page will mount the existing `system:storage-policy` save
contributor. The route-scoped settings coordinator will continue to show the
shared save action and navigation guard for dirty storage policy settings.

The `Data & Logs` page will not mount the storage contributor. Database,
backup, and log commands will keep their existing immediate action behavior.

## Mobile design contract

The desktop outcome and the phone outcome are the same. Both surfaces provide
direct destinations for system data and storage maintenance.

The existing Settings index and full-height Settings sheet are the nearest
mobile examples. They provide the route list, touch targets, and direct
navigation behavior. The change does not add another overlay.

Each destination remains an inline settings page with the existing page scroll
owner. The split reduces content depth and preserves safe-area behavior. It
does not change touch controls inside either page.

Desktop and phone share route definitions, discovery data, permissions, state,
and action handlers. Playwright coverage will open both destinations from the
correct settings navigation surface. Phone coverage will also check horizontal
containment and storage save behavior.

## Compatibility

The canonical `Data & Logs` path remains `/settings/system/data-storage`.
Existing `/settings/system/database`, `/settings/system/backups`, and
`/settings/system/logs` paths will continue to redirect there.

`/settings/system/storage` will stop redirecting and will become a rendered
page. Saved last-page behavior will accept both canonical routes because both
remain in the static route table.

Loading and error states remain local to the component that owns each API.
Changing one route does not change API availability or admin checks.

## Verification

Unit tests will cover route rendering, navigation labels, breadcrumb labels,
discovery ownership, and localized copy. Playwright tests will cover desktop,
phone, authenticated member, and container-backed storage paths.

## Related decisions

- [Separate System Data and Storage Pages](../../../decisions/2026-09-03-separate-system-data-storage-pages.md)
- [Centralize Navigation and Namespace Plugin Destinations](../../../decisions/2026-08-04-navigation-manifest-boundaries.md)
- [Install-wide storage maintenance](../../../decisions/0045-install-wide-storage-maintenance.md)
- [Settings route save coordinator](../../../decisions/0046-settings-route-save-coordinator.md)
